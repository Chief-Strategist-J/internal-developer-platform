# Role-Based Access Control (RBAC) — Business Logic Decision Trees

## 1. Architectural Overview & Authorization Decision Tree

```
RBAC & ABAC ENGINE (EVALUATION DECISION TREE)
│
├── 1. REQUEST INGESTION & CONTEXT NORMALIZATION TIER [P0]
│   └── Real-World: Developer submits pipeline deploy trigger to production cluster via Platform API / CLI
│       └── Trigger: Ingress Request with Bearer Token / Workload Identity
│           ├── Check: Is Authentication Token Valid, Signature Intact, and Not Expired?
│           │   ├── NO ──> [REJECT: HTTP 401 Unauthorized — Invalid Token or Expired Signature]
│           │   └── YES ──> Extract Subject (Sub, TenantID, GroupMemberships, Iss, ClientType)
│           │       ├── Check: Is Subject Blacklisted / Revoked in Distributed Revocation Cache (Redis Sentinel)?
│           │       │   ├── YES ──> [REJECT: HTTP 401 Unauthorized — Subject Revoked / Session Terminated]
│           │       │   └── NO ──> Context Normalized: (Subject, Action, Resource, TargetEnvironment)
│           │
├── 2. SYSTEM EMERGENCY & BYPASS EVALUATION TIER [P0]
│   └── Real-World: Production outage incident; platform on-call engineer triggers emergency break-glass procedure
│       └── Evaluation: Break-Glass / Incident Elevation Flag Present?
│           ├── YES
│           │   ├── Check: Is Break-Glass Ticket ID valid, approved by PagerDuty/Jira, within active TTL (max 2 hrs)?
│           │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Invalid or Unapproved Break-Glass Ticket]
│           │   │   └── YES ──> Check: Multi-Party Authorization (two-person rule) satisfied if target is Tier-0 Prod?
│           │   │       ├── NO ──> [REJECT: HTTP 403 Forbidden — Requires Dual Authorization Approval]
│           │   │       └── YES ──> [ALLOW WITH AUDIT: Grant Temporary Elevated Scope + Emit High-Priority SIEM Alert]
│           │   └── State: ELEVATED_SESSION_ACTIVE
│           └── NO ──> Proceed to Standard RBAC Evaluation
│
├── 3. ROLE BINDING & TENANT HIERARCHY RESOLUTION TIER [P0 / P1]
│   └── Real-World: Platform engineer requests access to deploy service across shared multi-tenant cluster
│       └── Evaluation: Direct & Group Role Mappings
│           ├── Check: Does Subject belong to Global System Admin Role?
│           │   ├── YES ──> Check: Is Action explicitly barred by Guardrail Invariant (e.g. Read Customer PII / Delete Audit Logs)?
│           │   │   ├── YES ──> [HARD REJECT: HTTP 403 Forbidden — Guardrail Violation Prevents Root Action]
│           │   │   └── NO ──> [ALLOW: Global System Admin Permitted]
│           │   └── NO ──> Resolve Tenant & Namespace Bindings
│           │       ├── Check: Does Subject have RoleBinding in target Tenant/Project?
│           │       │   ├── NO ──> [REJECT: HTTP 403 Forbidden — No Binding for Target Tenant/Namespace]
│           │       │   └── YES ──> Flatten Permissions from Direct Roles + Inherited Team Roles
│           │
├── 4. PERMISSION MATRIX & ACTION MATCHING TIER [P0]
│   └── Real-World: CI/CD runner requests token to update Kubernetes Deployment vs Secret extraction
│       └── Evaluation: Action (Verbs: read, write, update, delete, scale, execute, manage-secrets)
│           ├── Check: Does permission set include wildcard `*` or exact verb match for resource?
│           │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Missing Action Permission]
│           │   └── YES ──> Action Validated
│           │
├── 5. ATTRIBUTE-BASED ACCESS CONTROL (ABAC) & ENVIRONMENTAL GUARDS TIER [P1]
│   └── Real-World: Junior dev authorized to deploy in dev/staging attempts staging credential to push to prod
│       └── Evaluation: Dynamic Contextual Attributes
│           ├── Attribute Check: Target Environment Sensitivity (Development vs Staging vs Production)
│           │   ├── DEV / STAGING ──> Permitted within Tenant Quota
│           │   └── PRODUCTION
│           │       ├── Check: Is change window open or deployment frozen (Freeze Period / Black Friday)?
│           │       │   ├── FROZEN ──> [REJECT: HTTP 403 Forbidden — Active Change Freeze Active]
│           │       │   └── OPEN
│           │       │       ├── Check: Is Subject's MFA Assurance Level high (WebAuthn / FIDO2 for Prod)?
│           │       │       │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Step-Up MFA Required for Production]
│           │       │       │   └── YES ──> Check: Resource Tag / Data Classification Match
│           │       │       │       ├── MISMATCH ──> [REJECT: HTTP 403 Forbidden — Attribute Policy Deny]
│           │       │       │       └── MATCH ──> ABAC Validated
│           │
└── 6. DECISION CACHING & AUDIT DISPATCH TIER [P0]
    └── Real-World: 10,000 requests/sec authorization decisions hit Policy Decision Point (PDP)
        └── Output Generation
            ├── Decision: PERMIT
            │   ├── Action: Write decision to short-lived local LRU cache (TTL: 30s with cluster invalidation bus)
            │   └── Action: Dispatch Async Audit Record (Subject, Action, Resource, Granted=True, Reason)
            │       └── [RETURN: HTTP 200 OK — Authorized]
            └── Decision: DENY
                ├── Action: Do NOT cache deny across dynamic attribute fluctuations
                └── Action: Dispatch Immediate Security Incident Outbox Event (Subject, Action, Resource, ViolationCode)
                    └── [RETURN: HTTP 403 Forbidden — Access Denied]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Default Deny (Zero-Trust Principle)
- Real-Life Incident: In a cloud platform, an engineer misconfigured a wildcard route `/api/v2/*` without explicit policy attachments, resulting in unauthenticated internet users accessing customer microservice telemetry.
- Rationale: Any request without an explicit, cryptographically verifiable positive grant must result in an immediate deny. Absence of policy cannot equal permissive pass-through.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: DEFAULT DENY (P0-01)
Trigger: Access Evaluation Initiated
├── Check: Are valid roles and policies found for Subject?
│   ├── NO ──> [DENY: Explicit Deny Code ERR_NO_APPLICABLE_POLICY]
│   └── YES ──> Check: Is there any explicit DENY rule matching context?
│       ├── YES ──> [DENY: Explicit Deny Overrides All Grants]
│       └── NO ──> Check: Is there at least one explicit PERMIT rule?
│           ├── NO ──> [DENY: Fallback Default Deny Enforced]
│           └── YES ──> [PERMIT: Continue Evaluation]
```

### Invariant P0-02: Strict Multi-Tenant Isolation
- Real-Life Incident: A misconfigured group binding allowed an engineer in Tenant A (Fintech) to execute `kubectl exec` into a namespace owned by Tenant B (Healthcare) sharing the same Kubernetes control plane.
- Rationale: Tenant IDs are cryptographic boundaries. No role, regardless of privilege within Tenant A, may resolve, mutate, or read resources in Tenant B without an explicit cross-tenant federated trust handshake.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MULTI-TENANT BOUNDARY (P0-02)
Trigger: Resource Request Evaluated
├── Extract: Subject Tenant Context (Token Claim: tenant_id)
├── Extract: Target Resource Tenant Context (Resource Metadata: tenant_id)
├── Check: Subject Tenant == Target Resource Tenant?
│   ├── YES ──> [PERMIT: Tenant Boundary Respected]
│   └── NO ──> Check: Cross-Tenant Federation Binding Exists & Cryptographically Signed?
│       ├── YES ──> [PERMIT: Cross-Tenant Delegation Permitted]
│       └── NO ──> [SECURITY HALT: Immediate 403 Forbidden + Alert SecOps]
```

### Invariant P0-03: Immutable Audit Logging for Denials & Break-Glass Access
- Real-Life Incident: A rogue insider attempted brute-force enumeration of API secrets; missing denial logs prevented incident response from discovering the breach attempt until data exfiltration occurred.
- Rationale: Every negative authorization decision and every elevated break-glass action must be recorded synchronously or in guaranteed outbox queues before the HTTP response is completed.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: AUDIT RECORDING (P0-03)
Trigger: Decision Finalized (DENY or ELEVATED_PERMIT)
├── Action: Construct Audit Event Object (UUID, Timestamp, Subject, Resource, Action, Context, Decision, TraceID)
├── Check: Is Outbox Write / Event Bus Dispatch Successful?
│   ├── YES ──> [RETURN: Send Decision to Client]
│   └── NO (Storage/Queue Partition) ──> Fail-Safe Handling
│       ├── Decision == DENY ──> [RETURN: HTTP 403 Forbidden (Log to Local Disk Failover Log)]
│       └── Decision == ELEVATED_PERMIT ──> [HALT: Deny Request — Cannot grant break-glass without audit guarantee]
```

### Invariant P1-04: Least-Privilege Role Inheritance Cycle Prevention
- Real-Life Incident: An admin created role inheritance `Developer -> Team-Lead -> Manager -> Developer`, causing the policy engine evaluator to enter an infinite loop, exhausting memory and taking down the platform auth server.
- Rationale: Role hierarchies must form a Directed Acyclic Graph (DAG). Any cycle must be detected and rejected at definition time.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ROLE HIERARCHY DAG (P1-04)
Trigger: Role Binding or Role Hierarchy Creation / Update
├── Action: Traverse Role Dependency Graph via Tarjan's / Kosaraju's Cycle Detection Algorithm
├── Check: Cycle Detected in Role Graph?
│   ├── YES ──> [REJECT: HTTP 422 Unprocessable Entity — Cyclic Dependency Detected]
│   └── NO ──> [ACCEPT: Commit Role Definition]
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Token Expiration Mid-Execution of Long-Running Job
- Real-Life Incident: A deployment pipeline running a 45-minute database migration job had its 15-minute JWT expire midway, aborting the schema rollout and leaving the DB in a locked state.
- Decision Tree:
```
EDGE CASE 01: TOKEN EXPIRY IN FLIGHT
Trigger: Long-Running Platform Operation in Progress
├── Check: Did token expire while task worker is actively executing an idempotent batch step?
│   ├── YES
│   │   ├── Check: Is task worker operating under cryptographically signed Task Execution Lease?
│   │   │   ├── YES ──> [ALLOW: Task Lease Valid until Lease Expiration TTL; proceed without re-checking user JWT]
│   │   │   └── NO ──> [SUSPEND: Pause task step, trigger refresh token exchange via platform agent]
│   │   │       ├── Refresh Succeeded ──> [RESUME: Continue step with renewed lease]
│   │   │       └── Refresh Failed ──> [ABORT: Graceful compensation and rollback]
│   └── NO ──> [NORMAL: Proceed with standard verification]
```

### Case 02: Revocation Race Condition (User Terminated While Webhook Queued)
- Real-Life Incident: An employee was terminated and their Okta account deactivated, but an asynchronous GitOps webhook triggered prior to termination ran 30 seconds later, deploying untracked code.
- Decision Tree:
```
EDGE CASE 02: ASYNCHRONOUS REVOCATION RACE
Trigger: Async Worker Pops Job from Platform Task Queue
├── Extract: Subject Public ID from Job Payload
├── Probe: Distributed Revocation Set (Redis Cluster + Memory Bloom Filter)
├── Check: Is Subject ID present in Revocation Register?
│   ├── YES ──> [DISCARD: Poison Job, Emit Security Warning, Terminate Execution]
│   └── NO ──> Check: Has Job queue latency exceeded Maximum Permissible Queue Drift (e.g. 5 minutes)?
│       ├── YES ──> [REVALIDATE: Make Synchronous OIDC Introspect Call to Identity Provider]
│       │   ├── Active == True ──> [EXECUTE: Job Dispatched]
│       │   └── Active == False ──> [DISCARD: Record Revocation and Evict Job]
│       └── NO ──> [EXECUTE: Job Dispatched]
```

### Case 03: Service-to-Service Impersonation Spoofing
- Real-Life Incident: Compromised microservice A appended an `X-Impersonate-User: admin` header to call internal platform API, bypassing standard perimeter ingress authentication.
- Decision Tree:
```
EDGE CASE 03: IMPERSONATION HEADER SPOOFING
Trigger: Request Received with Impersonation Header
├── Check: Does Call originate from Trusted Internal Network with mTLS Workload Certificate?
│   ├── NO ──> [HARD DROP: Strip Impersonation Header, Return 403 Forbidden]
│   └── YES
│       ├── Check: Does Calling Service Identity have explicit `iam.impersonate` grant?
│       │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Calling Service Cannot Impersonate]
│       │   └── YES ──> Check: Is Target Impersonated User an Exempt Protected Identity (e.g. Root Admin)?
│       │       ├── YES ──> [HARD REJECT: Protected Identities Cannot Be Impersonated]
│       │       └── NO ──> [ALLOW: Log Both Caller Identity AND Impersonated Subject in Audit Trail]
```

### Case 04: Confused Deputy Problem in Shared Webhook Handlers
- Real-Life Incident: Multi-tenant webhook dispatcher called external third-party endpoint using platform's internal system credentials instead of tenant-scoped secrets, leaking internal metadata.
- Decision Tree:
```
EDGE CASE 04: CONFUSED DEPUTY PREVENTION
Trigger: Platform Service invokes downstream resource on behalf of Tenant
├── Check: Is request signed with Shared Platform Service Account or Tenant-Specific Delegation Token?
│   ├── SHARED SERVICE ACCOUNT ──> [HALT: Reject Invocation — Anti-Pattern Detected]
│   └── TENANT DELEGATION TOKEN
│       ├── Check: Does Token scope strictly match Tenant Resource Scope?
│       │   ├── NO ──> [REJECT: Scope Oversubscription Detected]
│       │   └── YES ──> [FORWARD: Execute Request under Tenant Delegation Scope]
```

### Case 05: Policy Decision Point (PDP) Cache Staleness during Role Downgrade
- Real-Life Incident: A developer was removed from `Production-Deployer` role, but an in-memory PDP cache with 10-minute TTL permitted them to push emergency changes for 8 minutes after revocation.
- Decision Tree:
```
EDGE CASE 05: PDP CACHE STALENESS
Trigger: Role Revocation / Permission Downgrade Committed
├── Action: Publish Invalidation Event to Distributed Pub/Sub Bus (Redis / Kafka)
├── Check: Did all Policy Decision Points acknowledge cache eviction within 500ms?
│   ├── YES ──> [HEALTHY: Cache Coherent Across Cluster]
│   └── NO (Network Partition / Dropped PubSub Message)
│       ├── Fail-Safe: PDP local cache enforces hard staleness limit (max 30s for mutation actions)
│       └── Production Action Check: Production mutations ALWAYS bypass read cache and query authoritative DB
```

### Case 06: Cascading Permission Inheritance Explosion (Deep Role Tree)
- Real-Life Incident: Nesting 25 levels of sub-teams resulted in an exponential permission tree traversal taking 1.8 seconds per HTTP request, causing cascade timeouts across the platform API.
- Decision Tree:
```
EDGE CASE 06: PERMISSION GRAPH EXPANSION EXPLOSION
Trigger: Role Resolution Requested
├── Check: Does Depth of Role Inheritance Hierarchy exceed MAX_DEPTH (5 levels)?
│   ├── YES ──> [REJECT: Schema Invalidation — Role Tree Exceeds Maximum Nesting Depth]
│   └── NO ──> Check: Are flattened role permissions available in Subject Session Context?
│       ├── YES ──> [O(1) RESOLUTION: Evaluate direct bitmask/hashset permissions]
│       └── NO ──> Flatten Graph with memoization, cache flattened bitmask for session duration
```

### Case 07: Split-Brain Ingress Authorization during Regional Network Partition
- Real-Life Incident: US-East and US-West suffered a WAN link severance; local US-West PDP could not reach global policy store and defaulted to rejecting all valid developer traffic.
- Decision Tree:
```
EDGE CASE 07: SPLIT-BRAIN REGIONAL AVAILABILITY
Trigger: Local PDP cannot connect to Global Policy Authority
├── Check: Is Action classified as READ-ONLY / RUNTIME EXECUTION or PRIVILEGED ROLE MUTATION?
│   ├── ROLE MUTATION (Add Admin, Create Role) ──> [REJECT: HTTP 503 Unavailable — Cannot mutate during partition]
│   └── RUNTIME WORKLOAD / DEPLOY EXECUTION
│       ├── Check: Is Local Read-Replica policy snapshot younger than Stale Grace Window (2 hours)?
│       │   ├── YES ──> [DEGRADED ALLOW: Evaluate against local snapshot with warning header]
│       │   └── NO ──> [FAIL CLOSED: Snapshot expired, reject to prevent security posture breach]
```

### Case 08: Rapid-Fire Escalation Privilege Grabbing (Race Condition)
- Real-Life Incident: An attacker sent 50 parallel HTTP requests to claim an unassigned admin binding token, triggering a database race condition that allocated the role twice to separate accounts.
- Decision Tree:
```
EDGE CASE 08: CONCURRENT ROLE ALLOCATION RACE
Trigger: Subject requests claim of Exclusive Role Binding
├── Action: Execute Conditional Update `UPDATE role_binding SET subject_id = :id WHERE role_id = :role AND subject_id IS NULL`
├── Check: Rows affected == 1?
│   ├── YES ──> [COMMIT: Role Granted Exclusively]
│   └── NO (Rows affected == 0)
│       └── [REJECT: HTTP 409 Conflict — Role Binding Already Claimed]
```

### Case 09: Wildcard Permission Collision (`storage.*` vs `storage.read`)
- Real-Life Incident: A developer assigned `storage.read` was unintentionally granted bucket deletion because an upstream policy wildcard `storage.*` was applied at team level.
- Decision Tree:
```
EDGE CASE 09: WILDCARD PERMISSION COLLISION
Trigger: Permission Matching for Sensitive Destructive Action
├── Check: Does Resource or Action match a Wildcard Pattern?
│   ├── YES
│   │   ├── Check: Is Action marked as High-Risk Destructive (e.g. Delete, Drop, Terminate)?
│   │   │   ├── YES ──> Wildcard Exclusion Rule: High-Risk Destructive actions REQUIRE explicit non-wildcard verbs
│   │   │   │   ├── Explicit Verb Present ──> [PERMIT: Specific Authorization Confirmed]
│   │   │   │   └── Wildcard Only ──> [DENY: Destructive Action Blocked by Wildcard Guardrail]
│   │   │   └── NO ──> [PERMIT: Standard Wildcard Expansion]
│   └── NO ──> [PERMIT: Exact Verb Match]
```

### Case 10: Just-In-Time (JIT) Elevation Expiration During Active Terminal Session
- Real-Life Incident: An operator running `kubectl exec` via temporary JIT elevation kept the TCP socket open for 6 hours after their 30-minute approval window expired, bypassing access limits.
- Decision Tree:
```
EDGE CASE 10: ACTIVE STREAMING SOCKET TIMEOUT
Trigger: Continuous Streaming Connection (WebSocket / SSH / Kubectl Exec)
├── Background Heartbeat: Check Active Session Elevation Status every 60 seconds
├── Check: Has Session Grant or JIT Window Expired?
│   ├── YES ──> [TERMINATE: Send TCP FIN/RST Packet, Sever Connection, Flush TTY Buffer, Log Session Duration]
│   └── NO ──> [MAINTAIN: Keep Socket Active]
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
RBAC ENGINE IMPLEMENTATION ROADMAP
│
├── PHASE 0: FOUNDATIONAL ZERO-TRUST CORE (P0 - WEEKS 1-3)
│   ├── Enforce Default-Deny PDP evaluation engine
│   ├── Schema DDL: Strict PostgreSQL tables for roles, permissions, bindings with tenant isolation
│   ├── Integration with Platform API Gateway (Token validation, Subject extraction)
│   ├── Cryptographic audit outbox for all denials and mutations
│   └── Deliverable: Fully tested O(1) in-memory policy matcher with zero wildcard leaks
│
├── PHASE 1: TENANCY, DELEGATION & ABAC RULES (P1 - WEEKS 4-6)
│   ├── Dynamic ABAC attribute evaluator (Environment, Change Freeze, Time-of-Day)
│   ├── Cross-tenant federation bindings with mutual signatures
│   ├── Redis Sentinel distributed revocation set with sub-second propagation
│   ├── Cycle detection validator on role creation APIs
│   └── Deliverable: Complete multi-tenant isolation with dynamic environmental guards
│
├── PHASE 2: BREAK-GLASS & JIT ELEVATION WORKFLOWS (P2 - WEEKS 7-9)
│   ├── Dual-authorization approval workflows (Two-Person rule)
│   ├── Time-bounded JIT access tokens with auto-revocation triggers
│   ├── Active streaming socket reaper for expired kubectl/SSH sessions
│   ├── Ingress mTLS workload identity impersonation filters
│   └── Deliverable: Production-grade emergency break-glass pipeline with SOC2 compliance
│
└── PHASE 3: DISTRIBUTED PDP & HIGH-SCALE FEDERATION (P3 - WEEKS 10-12)
    ├── Local sidecar PDPs (Open Policy Agent / Cedar integration) with eBPF hooks
    ├── Split-brain resilience with regional snapshot validation
    ├── Automated least-privilege analysis & unused permission pruning bot
    └── Deliverable: Sub-millisecond global authorization across 100,000 requests/sec
```
