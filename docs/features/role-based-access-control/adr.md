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
