# Architectural Decision Record (ADR): Role-Based Access Control (RBAC) & Tenant Authorization Engine

## Status
**ACCEPTED**

## Context & Problem Statement
The Internal Developer Platform (IDP) provides self-service APIs for developer onboarding, service registration, infrastructure provisioning, and production telemetry monitoring. Without a cryptographically verified and low-latency authorization boundary:
1. Malicious or compromised actors could trigger arbitrary infrastructure deployments across production clusters.
2. Cross-tenant leakage could expose sensitive service descriptors, configuration catalogs, and credentials across organization departments.
3. Endpoints in `src/api/rest/v1/routers/service-catalog.router.go` would remain vulnerable to unauthorized tampering, creation, and deprecation.

The system requires an authorization engine that enforces:
- Standardized Subject extraction (compatible with enterprise OIDC JWT tokens such as Keycloak, Okta, Azure AD, or local test signers).
- Tenant isolation ensuring users can only read or mutate services within their bounded organization/tenant.
- Permission matrix matching verbs (`read`, `write`, `update`, `delete`, `admin`) against resources (`services`, `templates`, `secrets`, `environments`).
- Wildcard permission resolution (`*` verb or `*` resource matching).
- Real-time token/subject revocation querying the live Redis 7.4 cache (`idp-cache-1`).
- High-performance, zero-external-network authorization caching with strict distributed invalidation.

## Decision Drivers
- **Zero-Trust & Defense-in-Depth**: Every request entering the platform gateway or internal service must carry a valid, cryptographically verifiable claims context.
- **Hexagonal Architecture Compliance**: Pure domain business logic with zero framework lock-in; adapters for in-memory and live Redis infrastructure.
- **Independent Identity Decoupling**: Decoupled from specific identity providers (e.g., Keycloak or Okta) via standard RFC 7519 JWT and OIDC claims specification (`sub`, `iss`, `tenant_id`, `roles`, `permissions`, `exp`).
- **High Performance**: Authorization decisions evaluated within sub-millisecond budgets; revocation lookup against Redis completed under 2ms.

## Considered Options
1. **Option 1: Direct Integration with Keycloak Container as Hard Dependency**:
   - *Pros*: Keycloak UI manages users and role mappings.
   - *Cons*: Heavy JVM footprint (500MB+ memory), slow container startup, prevents fast automated unit testing and self-contained CI execution.
2. **Option 2: Open Policy Agent (OPA) / Rego Sidecar**:
   - *Pros*: Declarative policy evaluation language.
   - *Cons*: High latency for simple RBAC, additional process to maintain per pod, steep learning curve for basic role hierarchies.
3. **Option 3: Native Hexagonal RBAC Policy Decision Point (PDP) with Redis Revocation Adapter (SELECTED)**:
   - *Pros*: Blazing fast in-memory execution, strict hexagonal isolation (`contracts`, `domain`, `ports`, `adapters`, `service`), direct integration with live Redis for instantaneous token revocation, fully testable without external services yet seamlessly connects to live infrastructure.

## Decision Outcome
Adopt **Option 3**: Build a hexagonal RBAC Policy Decision Point inside `src/features/role-based-access-control/`.

### Core Architectural Components:
1. **Contracts (`src/features/role-based-access-control/contracts/`)**:
   - `Subject`: Holds `UserID`, `TenantID`, `Roles`, `Permissions`, `Issuer`, `ExpiresAt`.
   - `Permission`: Defines `Resource` (e.g. `service-catalog`) and `Action` (e.g. `read`, `write`, `*`).
   - `Role`: Named group of permissions (e.g. `PlatformAdmin`, `Developer`, `Viewer`).
   - `EvaluationRequest` & `EvaluationResult`: Decision contract (`Granted: bool`, `Reason: string`).
2. **Domain Rules (`src/features/role-based-access-control/domain/rules/`)**:
   - Pure authorization logic: verifies tenant match, evaluates role inheritance, resolves wildcard verbs and wildcards across resource scopes.
3. **Infrastructure Ports & Adapters (`src/features/role-based-access-control/infrastructure/`)**:
   - `RevocationStorePort`: Interface for token blacklist checks.
   - `RedisRevocationAdapter`: Connects directly to live Redis container (`127.0.0.1:6379`) to verify if a token JTI or Subject ID has been revoked.
   - `MemoryRevocationAdapter`: For pure offline unit testing.
   - `TokenVerifierPort`: Interface for cryptographic token validation.
4. **Service Layer (`src/features/role-based-access-control/service/`)**:
   - Orchestrates revocation verification, claims validation, and domain rule evaluation.
5. **REST API Middleware (`src/api/rest/v1/middleware/`)**:
   - `RBACMiddleware`: Extracts Bearer token, authenticates subject, checks tenant boundaries, and enforces required permissions before passing requests to route handlers.

## Consequences & Trade-offs
- **Positive**: Complete security boundary across all platform routes; supports live Redis cache for revocation; 100% testable in unit tests without external services.
- **Negative**: Platform must manage secret keys or JWKS configuration for signature verification in production environments.

---

## ADR-002: PostgreSQL Persistence for User Role Bindings and Access Grants
- **Status**: Accepted
- **Context**: Relying exclusively on in-memory or ephemeral storage for user role bindings (`rbac_user_bindings`) and just-in-time access grants (`access_grants`) resulted in data loss during platform API server restarts. In multi-tenant environments, user permissions must be durably stored in PostgreSQL with strict tenant boundary isolation and foreign key referential integrity.
- **Decision**:
  1. Implement `PostgresRBACAdapter` fulfilling `UserBindingRepositoryPort` and `AccessGrantRepositoryPort` in `src/features/role-based-access-control/infrastructure/adapters/role-based-access-control.postgres.repository.adapter.go`.
  2. Map relational queries to existing database tables `rbac_user_bindings` and `access_grants` adhering to `05-db-writing-rules.md` (filtering by tenant_id, handling optimistic concurrency, enforcing unique constraints).
  3. Expose REST ingress endpoints in `src/api/rest/v1/handlers/role-based-access-control.handler.go` (`POST /api/v1/auth/bindings`, `DELETE /api/v1/auth/bindings`, `GET /api/v1/auth/bindings`).
  4. Automatically resolve durable user role bindings in `IssueToken` (`POST /api/v1/auth/token`) when incoming request does not supply explicit roles, minting JWTs containing the persisted roles.
- **Consequences**: User role assignments and JIT access grants persist durably across restarts, maintain zero cross-tenant leakage via composite primary/foreign keys, and automatically reflect into issued authentication tokens.

---

## ADR-003: Native Libraries, Fail-Closed Revocation, and Zero Production Memory Fallbacks
- **Status**: Accepted
- **Context**: Custom HMAC/base64 JWT parsing and raw TCP RESP Redis commands violated the Native Library Mandate, while Redis lookup errors in `AuthenticateToken` failed open and `src/main.go` silently fell back to in-memory adapters when `deps.DB == nil`.
- **Decision**:
  1. Replace custom JWT and Redis implementations with `github.com/golang-jwt/jwt/v5` (`JWTTokenVerifierAdapter`) and `github.com/redis/go-redis/v9` (`RedisRevocationAdapter`) with Redis Pub/Sub cluster invalidation (`rbac:cache:invalidate`).
  2. Fail closed on Redis revocation errors, missing JWT signing secrets, and unconfigured Redis clients (`401 Unauthorized`).
  3. Wire `req.JTI` revocation in `RBACHandler.RevokeSession` and carry `GroupMemberships`, `ClientType`, `MFAVerified`, and `Classification` in `JWTClaims`.
  4. Require `deps.DB != nil` in `src/main.go` for production startup (no in-memory adapter fallback in production paths).
- **Consequences**: Eliminates silent fail-open security gaps and hand-rolled cryptographic/protocol code; requires PostgreSQL and valid `JWT_SECRET` at service boot.

---

## ADR-004: `feature-files.md` Structural Compliance, UnitOfWork Outbox, and PostgreSQL RLS
- **Status**: Accepted
- **Context**: `RBACService` used setter injection without `UnitOfWork` transaction boundaries or deadline checks, `RBACHandler` mixed inline validation and set headers after `WriteHeader`, and `database/config/rls.sql` had no tenant isolation policies.
- **Decision**:
  1. Refactor `RBACService` to `RBACServiceDeps` (`NewRBACServiceWithDeps`), `resilience.ValidateDeadlineBudget`, `UnitOfWorkPort` (`runInUnitOfWork`) wrapping S1 commands + transactional `rbac_audit_outbox` writes, `machines.TransitionGrantStatus` in `RevokeJITAccess`, and `ListActiveGrantsPage` cursor pagination.
  2. Refactor `RBACHandler` to `withSpan` → `decodeClosedJSON` → `rbacContracts.Validate*Input` → `emitSuccess`/`emitFailure` → `completeRequest`, setting headers before `WriteHeader` and mapping `sharedErrors.ApplicationError` codes.
  3. Add migration `0021_harden_rbac_schema.sql` (`rbac_role_hierarchy` table, `version`/`updated_at` on `rbac_user_bindings`, non-empty `CHECK` constraints on `rbac_audit_outbox`) and enable PostgreSQL RLS policies on `rbac_user_bindings`, `access_grants`, and `rbac_audit_outbox` in `database/config/rls.sql`.
  4. Add local disk failover logging (`rbac-deny-failover.log`) when `DENY` audit outbox writes fail, and wire a background outbox relay in `src/main.go`.
- **Consequences**: Atomic state + audit persistence, strict tenant isolation at the PostgreSQL storage gate, and full alignment with `feature-files.md` §2.1, §3, and §4.

---

## ADR-005: Externalized Break-Glass, Dynamic DB Role Flattening, Cross-Tenant Federation, and ABAC Guards
- **Status**: Accepted
- **Context**: Break-glass ticket approval, SIEM alerting, dynamic `rbac_roles` inheritance flattening, cross-tenant federation, and DEV/STAGING quota + time-of-day ABAC guards were unimplemented at runtime.
- **Decision**:
  1. Externalize `pagerduty_endpoint`, `pagerduty_api_token`, `jira_endpoint`, `jira_api_token`, `siem_webhook_url`, `siem_api_token`, `federation_hmac_secret`, `abac_allowed_start_hour_utc`, `abac_allowed_end_hour_utc`, and `dev_staging_max_quota` in `config/default.yaml`, `config/env.schema`, and `src/infra/config/config.go`.
  2. Implement `HTTPBreakGlassTicketAdapter` and `HTTPSIEMAlertAdapter` in `role-based-access-control.audit.adapter.go`, wiring SIEM dispatch to both `ELEVATED_PERMIT` events and the `rbac_audit_outbox` relay.
  3. Load `rbac_roles` + `rbac_role_hierarchy` from PostgreSQL via `PostgresRBACAdapter.LoadRolePermissions` and flatten inherited permissions via `FlattenRoleHierarchyPermissions` (BFS bounded by `MaxRoleHierarchyDepth = 5`).
  4. Enforce HMAC-SHA256 `VerifyCrossTenantFederationHandshake`, DEV/STAGING quota limits (`ERR_TENANT_QUOTA_EXCEEDED`), and UTC time-of-day guards (`ERR_OUTSIDE_ALLOWED_HOURS`) in `role-based-access-control.rules.go`.
- **Consequences**: Authorization decisions reflect live DB role hierarchies and external incident approvals without hardcoding secrets or policy data in source code.

---

## ADR-006: Phase 3 Distributed Sidecar PDPs (OPA/Cedar), eBPF Hook Enforcement & Least-Privilege Pruning
- **Status**: Accepted
- **Context**: Phase 3 of `business-logic.md` requires local sidecar Policy Decision Points (OPA / Cedar), eBPF syscall/path enforcement, and automated least-privilege analysis with unused permission pruning.
- **Decision**:
  1. Add `SidecarPDPPort` in `role-based-access-control.repository.port.go` and `HTTPSidecarPDPAdapter` in `role-based-access-control.sidecar.adapter.go` supporting OPA (`/v1/data/rbac/allow`) and Cedar (`/v1/is_authorized`) with strict HTTP timeouts.
  2. Implement pure domain rules `EvaluateSidecarPDPFallback`, `EvaluateEBPFHookEnforcement`, `AnalyzeLeastPrivilege`, and `PruneUnusedPermissions` in `role-based-access-control.rules.go`.
  3. Wire `EvaluateWithSidecarPDP` and `AnalyzeLeastPrivilege` into `RBACService` (`role-based-access-control.service.go`).
- **Consequences**: Local sidecar PDPs can evaluate requests with deterministic fallback to the core PDP, eBPF hook events are validated against syscall/path policies, and unused permissions can be identified and pruned.

