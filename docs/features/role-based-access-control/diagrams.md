# RBAC & Authorization Architecture Diagrams and Usage Guide

## 1. High-Level Hexagonal Architecture

```mermaid
flowchart TD
    Client["Client / CLI / UI"] -->|"Bearer JWT + Headers"| Router["REST Router (/api/v1/auth/*, /api/v1/rbac/*)"]
    Router --> MW["RBACMiddleware (Typed 401/403)"]
    Router --> Handler["RBACHandler (withSpan -> decode -> Validate*Input -> emit)"]
    MW --> Service["RBACService (RBACServiceDeps + ValidateDeadlineBudget)"]
    Handler --> Service

    subgraph DomainCore["RBAC Domain Core"]
        Rules["Domain Rules (EvaluateAuthorization, FlattenRoleHierarchy, FederationHMAC, ABAC)"]
        Machine["State Machine (TransitionGrantStatus, TokenLifecycleMachine)"]
        Contracts["Contracts & Schema (Subject, EvaluationRequest, FromAPI/ToAPI)"]
        Service --> Rules
        Service --> Machine
        Rules --> Contracts
    end

    subgraph OutboundAdapters["Infrastructure Outbound Adapters"]
        JWT["JWTTokenVerifierAdapter (golang-jwt/jwt/v5)"]
        Redis["RedisRevocationAdapter (go-redis/v9 + PubSub)"]
        Postgres["PostgresRBACAdapter (UnitOfWork + RLS)"]
        Outbox["PostgresRBACAuditOutboxAdapter (rbac_audit_outbox)"]
        Ticket["HTTPBreakGlassTicketAdapter (PagerDuty / Jira)"]
        SIEM["HTTPSIEMAlertAdapter (SIEM Webhook)"]
    end

    Service --> JWT
    Service --> Redis
    Service --> Postgres
    Service --> Outbox
    Service --> Ticket
    Service --> SIEM

    Redis -->|"EXISTS / SETEX / PUBLISH"| LiveRedis[("Redis (idp-cache-1)")]
    Postgres -->|"SQL + app.current_tenant_id"| LivePG[("PostgreSQL (rbac_* + access_grants)")]
    Outbox -->|"INSERT + Relay Ticker (5s)"| LivePG
    Outbox -->|"Fallback on DENY"| FailoverLog["Local Disk Failover Log (rbac-deny-failover.log)"]
    Ticket -->|"GET /incidents or /issue"| ExtTicketing["PagerDuty / Jira API"]
    SIEM -->|"POST Security Event"| ExtSIEM["SIEM / SecOps Collector"]
```

---

## 2. 6-Tier Authorization, Break-Glass & Audit Sequence

```mermaid
sequenceDiagram
    autonumber
    actor Caller
    participant MW as RBACMiddleware / Handler
    participant Service as RBACService
    participant Redis as RedisRevocationAdapter
    participant Ticket as HTTPBreakGlassTicketAdapter
    participant PG as PostgresRBACAdapter
    participant Rules as Domain Rules
    participant Outbox as AuditOutbox / SIEM

    Caller->>MW: Request with Bearer JWT + x-tenant-id
    MW->>Service: AuthenticateToken(rawToken)
    Service->>Service: Verify JWT signature & expiry (golang-jwt/v5)
    alt Invalid or Expired JWT
        Service-->>MW: UnauthorizedError (401)
        MW-->>Caller: HTTP 401 Unauthorized
    end

    Service->>Redis: IsRevoked(subjectID, jti)
    alt Redis Unavailable (Fail Closed) or Revoked
        Redis-->>Service: Error or Revoked=true
        Service-->>MW: UnauthorizedError (401)
        MW-->>Caller: HTTP 401 Unauthorized
    end

    MW->>Service: AuthorizeWithContext(ctx, evalReq)
    Service->>Service: enrichEvaluationRequest(ctx, evalReq)
    opt BreakGlass Ticket Not Pre-Approved
        Service->>Ticket: VerifyTicketApproval(ticketID)
        Ticket-->>Service: Approved / Rejected
    end
    Service->>PG: LoadRolePermissions() + GetRoleHierarchyEdges()
    PG-->>Service: DB Role Definitions & DAG Edges
    Service->>Rules: FlattenRoleHierarchyPermissions(roles, groups, defs, edges)

    Service->>Rules: EvaluateAuthorization(req, now)
    Note over Rules: Tier 2 Break-Glass -> Root Guardrails -> Tenant/Federation HMAC -> Explicit Deny -> Role Perms -> Tier 5 ABAC (Hours, Quota, Freeze, MFA, Classification)

    alt Decision == DENY
        Service->>Outbox: EnqueueAuditEvent("DENY")
        opt Outbox Unavailable
            Service->>Service: Write rbac-deny-failover.log
        end
        Service-->>MW: ForbiddenError (403)
        MW-->>Caller: HTTP 403 Forbidden
    else Decision == ELEVATED_PERMIT
        Service->>Outbox: EnqueueAuditEvent("ELEVATED_PERMIT")
        alt Outbox Fails
            Service-->>MW: ERR_AUDIT_UNAVAILABLE (403 Fail-Closed)
        else Outbox Succeeds
            Service->>Outbox: EmitSecurityAlert (SIEM)
            Service-->>MW: Granted (ElevatedPermit=true)
        end
    else Decision == PERMIT
        Service->>Service: Cache in 30s LRU (bypassed for Prod mutations)
        Service->>Outbox: EnqueueAuditEvent("PERMIT")
        Service-->>MW: HTTP 200 OK
    end
```

---

## 3. S1 Command UnitOfWork & Outbox Relay Flow

```mermaid
flowchart LR
    Cmd["S1 Command (RequestJITAccess / RevokeJITAccess / BindUserRole / UnbindUserRole)"] --> Deadline["ValidateDeadlineBudget"]
    Deadline --> UoW["UnitOfWork.Run(txCtx)"]
    UoW -->|"BeginTx"| Logic["Validate Policy / TransitionGrantStatus (Machine)"]
    Logic --> WriteEntity["Persist Entity (access_grants / rbac_user_bindings)"]
    WriteEntity --> WriteOutbox["EnqueueAuditEvent in Same txCtx (rbac_audit_outbox)"]
    WriteOutbox -->|"CommitTx"| Done["Return Typed Result"]
    WriteOutbox -.->|"5s Background Relay in main.go"| Relay["ListPendingAuditEvents -> SIEM -> MarkAuditEventPublished"]
```

---

## 4. In-Depth Usage & Operations Guide

### 4.1 Configuration & Secrets (`config/default.yaml` & Environment Overrides)
All external endpoints and secrets are loaded via `src/infra/config/config.go` (`AppConfig.RBAC`). Leave tokens empty in `config/default.yaml` and inject them via environment variables in production:

| Environment Variable | Config Path | Purpose |
|---|---|---|
| `JWT_SECRET` | `auth.jwt_secret` | HS256 signing key (min 16 chars; adapter fails closed if empty) |
| `AUTH_ISSUER` | `auth.issuer` | Expected JWT `iss` claim |
| `REDIS_ADDR` | `redis.address` | Redis host:port for revocation & `rbac:cache:invalidate` Pub/Sub |
| `DATABASE_URL` | `database.url` | PostgreSQL DSN (required at boot; no memory fallback) |
| `PAGERDUTY_ENDPOINT` | `rbac.pagerduty_endpoint` | PagerDuty API base URL for `PD-*` break-glass tickets |
| `PAGERDUTY_API_TOKEN` | `rbac.pagerduty_api_token` | PagerDuty REST token (`Token token=...`) |
| `JIRA_ENDPOINT` | `rbac.jira_endpoint` | Jira API base URL for non-`PD-*` break-glass tickets |
| `JIRA_API_TOKEN` | `rbac.jira_api_token` | Jira Bearer API token |
| `SIEM_WEBHOOK_URL` | `rbac.siem_webhook_url` | SIEM HTTP collector URL for break-glass & outbox relay |
| `SIEM_API_TOKEN` | `rbac.siem_api_token` | SIEM Bearer token |
| `RBAC_FEDERATION_HMAC_SECRET` | `rbac.federation_hmac_secret` | Shared HMAC-SHA256 secret for cross-tenant federation handshakes |

Additional static defaults in `config/default.yaml` under `rbac:`:
- `abac_allowed_start_hour_utc: 0` and `abac_allowed_end_hour_utc: 24` (set e.g. `8` to `20` to enforce UTC business-hours access).
- `dev_staging_max_quota: 100` (enforced when `targetEnvironment` is `DEV` or `STAGING`).

### 4.2 Database Migrations & RLS Setup
Apply the RBAC migrations in order (`0002_create_rbac_tables.sql`, `0011_create_access_grants_table.sql`, `0020_create_rbac_audit_outbox.sql`, `0021_harden_rbac_schema.sql`), then apply `database/config/rls.sql`. Ensure your PostgreSQL transaction runner sets `app.current_tenant_id` per transaction so RLS policies on `rbac_user_bindings`, `access_grants`, and `rbac_audit_outbox` allow tenant-scoped queries.

### 4.3 API Usage Examples (`curl`)

#### 1. Issue a JWT with Extended Claims
```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{
    "subjectId": "usr_101",
    "tenantId": "tenant-alpha",
    "roles": ["Developer"],
    "groupMemberships": ["TeamLead"],
    "clientType": "cli",
    "mfaVerified": true,
    "classification": "confidential",
    "ttlSeconds": 3600
  }'
```

#### 2. Standard RBAC + ABAC Authorization Check
```bash
curl -X POST http://127.0.0.1:8080/api/v1/rbac/authorize \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -H "x-tenant-id: tenant-alpha" \
  -d '{
    "targetTenantId": "tenant-alpha",
    "resource": "service-catalog",
    "action": "write",
    "targetEnvironment": "PRODUCTION",
    "changeFreezeActive": false,
    "requireStepUpMfa": true,
    "resourceClassification": "confidential"
  }'
```

#### 3. Emergency Break-Glass Authorization (Tier-0 Prod)
If `ticketApproved` is `false`, `RBACService` verifies `ticketId` against `HTTPBreakGlassTicketAdapter` (PagerDuty for `PD-*` IDs, Jira otherwise), enforces `ttlSeconds <= 7200` and `dualApprovalGranted: true` for `tier0Prod: true`, persists `ELEVATED_PERMIT` to `rbac_audit_outbox`, and emits a SIEM alert.
```bash
curl -X POST http://127.0.0.1:8080/api/v1/rbac/authorize \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{
    "targetTenantId": "tenant-alpha",
    "resource": "infrastructure",
    "action": "delete",
    "targetEnvironment": "PRODUCTION",
    "breakGlass": {
      "ticketId": "PD-99124",
      "ticketApproved": false,
      "ttlSeconds": 1800,
      "tier0Prod": true,
      "dualApprovalGranted": true
    }
  }'
```

#### 4. Cross-Tenant Federation Handshake
Compute `signature = hex(HMAC_SHA256(RBAC_FEDERATION_HMAC_SECRET, "<sourceTenantId>|<targetTenantId>|<expiresAtUnix>"))`:
```bash
curl -X POST http://127.0.0.1:8080/api/v1/rbac/authorize \
  -H "Authorization: Bearer <JWT>" \
  -H "Content-Type: application/json" \
  -d '{
    "targetTenantId": "tenant-beta",
    "resource": "service-catalog",
    "action": "read",
    "crossTenantFederation": {
      "sourceTenantId": "tenant-alpha",
      "targetTenantId": "tenant-beta",
      "expiresAtUnix": 1893456000,
      "signature": "<hmac-sha256-hex>"
    }
  }'
```

#### 5. Bind User Role & Validate Role Hierarchy DAG
```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/bindings \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "Content-Type: application/json" \
  -d '{"subjectId": "usr_101", "tenantId": "tenant-alpha", "roleName": "Developer"}'

curl -X POST http://127.0.0.1:8080/api/v1/rbac/roles/validate-hierarchy \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "Content-Type: application/json" \
  -d '{"edges": {"PlatformAdmin": ["Developer"], "Developer": ["Viewer"]}}'
```

#### 6. JIT Access Grant Lifecycle & Session/JTI Revocation
```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/jit/grants \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "grant-001",
    "tenantId": "tenant-alpha",
    "subjectId": "usr_101",
    "targetNamespace": "prod-payments",
    "role": "Developer",
    "reason": "INC-4421 database failover investigation",
    "durationSeconds": 1800,
    "approverRole": "PlatformAdmin",
    "idempotencyKey": "idem-grant-001"
  }'

curl -X POST http://127.0.0.1:8080/api/v1/auth/jit/grants/grant-001/revoke \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "x-tenant-id: tenant-alpha"

curl -X POST http://127.0.0.1:8080/api/v1/auth/revoke \
  -H "Authorization: Bearer <ADMIN_JWT>" \
  -H "Content-Type: application/json" \
  -d '{"subjectId": "usr_101", "jti": "tok-jti-123", "reason": "offboarding"}'
```
