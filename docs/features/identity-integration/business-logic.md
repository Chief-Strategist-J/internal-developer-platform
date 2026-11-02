# Identity Integration — Business Logic Decision Trees

## 1. Architectural Overview & Identity Synchronization Decision Tree

```
IDENTITY INTEGRATION ENGINE (AUTHENTICATION & PROVISIONING DECISION TREE)
│
├── 1. INBOUND AUTHENTICATION & TOKEN INGESTION TIER [P0]
│   └── Real-World: Developer signs into Internal Developer Platform via Corporate Okta / Azure AD OIDC
│       └── Trigger: Ingress OIDC Callback / Authorization Code Flow
│           ├── Check: Is State & Nonce parameter valid and matching session cookie?
│           │   ├── NO ──> [REJECT: HTTP 400 Bad Request — CSRF State Mismatch / Replay Detected]
│           │   └── YES ──> Action: Exchange Authorization Code with IdP Token Endpoint
│           │       ├── Check: Token Exchange Successful & IdP TLS Certificate Verified?
│           │       │   ├── NO ──> [REJECT: HTTP 502 Bad Gateway — IdP Communication Failed]
│           │       │   └── YES ──> Ingest ID Token & Access Token
│           │
├── 2. JWT VALIDATION & CRYPTOGRAPHIC VERIFICATION TIER [P0]
│   └── Real-World: API gateway evaluates inbound Bearer token from CLI tool vs automated CI runner
│       └── Evaluation: Signature & Claims Verification
│           ├── Check: Is `iss` (Issuer) in Allowed Platform Identity Providers list?
│           │   ├── NO ──> [REJECT: HTTP 401 Unauthorized — Untrusted Issuer]
│           │   └── YES ──> Check: Is `kid` present in cached JWKS key set?
│           │       ├── NO ──> Action: Fetch fresh JWKS from IdP (rate-limited to 1 request/min)
│           │       │   ├── Key Found ──> Proceed
│           │       │   └── Key Still Missing ──> [REJECT: HTTP 401 Unauthorized — Invalid Key ID]
│           │       └── YES ──> Verify RSA/ECDSA Signature
│           │           ├── INVALID ──> [REJECT: HTTP 401 Unauthorized — Signature Verification Failed]
│           │           └── VALID ──> Check Expiration: `exp > NOW()` & `nbf <= NOW()`
│           │               ├── EXPIRED ──> [REJECT: HTTP 401 Unauthorized — Token Expired]
│           │               └── VALID ──> Subject Identity Authenticated
│           │
├── 3. JUST-IN-TIME (JIT) PROVISIONING & SCIM SYNC TIER [P1]
│   └── Real-World: New hire logs into IDP for the first time; account and tenant affiliations must materialize
│       └── Evaluation: Subject Lookup in Platform Identity Store
│           ├── Check: Does Subject (`sub` + `iss`) exist in Platform User Database?
│           │   ├── YES ──> Action: Update `last_login_at`, sync updated claims (email, display_name)
│           │   └── NO ──> Action: Trigger Just-In-Time (JIT) Provisioning
│           │       ├── Check: Is domain whitelisted for tenant auto-join?
│           │       │   ├── NO ──> [HOLD: User Created in PENDING_APPROVAL state; Notify Tenant Admin]
│           │       │   └── YES ──> Action: Atomically create User record, assign default `Developer` role
│           │       └── State: USER_PROVISIONED
│           │
├── 4. ENTERPRISE GROUP MAPPING & RBAC BINDING TIER [P1]
│   └── Real-World: Engineer moves from Core Infrastructure team to Payments team in corporate HR system
│       └── Evaluation: IdP Group Claim to Platform Role Reconciliation
│           ├── Extract: IdP `groups` or `roles` claim array
│           ├── Action: Map enterprise groups (e.g. `sg-devops-payments`) to internal IDP Team & Roles
│           ├── Check: Has user lost membership in a previously mapped privileged group?
│           │   ├── YES ──> Action: Revoke unmapped internal roles immediately (Group Sync Eviction)
│           │   └── NO ──> Retain current bindings
│           └── Action: Bind newly assigned team roles within tenant boundaries
│
├── 5. WORKLOAD IDENTITY FEDERATION & MESH AUTHENTICATION TIER [P0 / P2]
│   └── Real-World: GitHub Actions CI workflow exchanges short-lived OIDC token for AWS/K8s credentials
│       └── Trigger: Workload Requests Token Exchange (RFC 8693)
│           ├── Check: Is Caller a Workload Subject (Spiffe ID / GitHub OIDC)?
│           │   ├── YES
│           │   │   ├── Check: Does repository subject match configured trust policy (`repo:org/service:ref:refs/heads/main`)?
│           │   │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Untrusted Workload Claim]
│           │   │   │   └── YES ──> Issue short-lived Platform Workload Token (TTL: 15 mins)
│           │   └── NO ──> Proceed with standard interactive user session
│           │
└── 6. SESSION LIFECYCLE & BACK-CHANNEL LOGOUT TIER [P0]
    └── Real-World: Enterprise security officer suspends employee in IdP; all active sessions must die instantly
        └── Trigger: Back-Channel Logout Event (OpenID Connect Back-Channel Logout 1.0)
            ├── Check: Verify Logout Token Signature from IdP
            │   ├── INVALID ──> [DROP: HTTP 400 Bad Request — Invalid Logout Token]
            │   └── VALID ──> Action: Extract `sub` or `sid` (Session ID)
            │       ├── Action: Insert Subject/Session into Distributed Revocation Store (Redis TTL: max token lifetime)
            │       ├── Action: Sever active WebSockets and Terminate API Gateway session cookies
            │       └── [RETURN: HTTP 200 OK — Back-Channel Logout Acknowledged]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Cryptographic JWKS Key Pinning & Anti-Tamper
- Real-Life Incident: An attacker launched a DNS spoofing attack against an unpinned IdP metadata URL, presenting a malicious JWKS public key and forging valid admin JWTs.
- Rationale: Identity Provider discovery URLs and public keys must be cached securely, fetched strictly over TLS 1.3 with pinned root certificates, and throttled against denial-of-service cache busts.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: JWKS PINNING (P0-01)
Trigger: JWKS Refresh Initiated
├── Check: Is IdP Discovery Host matching strict TLS certificate pin?
│   ├── NO ──> [SECURITY HALT: TLS Pin Failure — Abort Refresh]
│   └── YES ──> Check: Has JWKS refresh been called more than 5 times in 60 seconds?
│       ├── YES ──> [THROTTLE: Rate-limit key refresh; serve stale cached keys if valid]
│       └── NO ──> [FETCH: Update In-Memory JWKS Cache atomically]
```

### Invariant P0-02: Subject Identifier Immutability
- Real-Life Incident: An IDP integration matched users by mutable `email`. When an engineer's surname changed, their email was updated, creating a duplicate account and severing access to their production services.
- Rationale: Users must be uniquely and immutably keyed by `(issuer, subject_id)`. Email addresses are attributes, never primary foreign keys.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: IMMUTABLE SUBJECT KEY (P0-02)
Trigger: User Ingestion or Authentication Lookup
├── Extract: `iss` (Issuer URL) AND `sub` (Subject Identifier)
├── Primary Key Lookup: `SELECT * FROM identity_users WHERE issuer = :iss AND subject_id = :sub`
├── Check: Does Record Exist?
│   ├── YES ──> Match Found: Bind existing internal `user_id` (UUIDv7)
│   └── NO ──> Create New User with immutable `(issuer, subject_id)` composite constraint
```

### Invariant P1-03: Ephemeral Workload Identity Lifespans
- Real-Life Incident: A developer generated a long-lived machine API token for a CI runner; the token was accidentally committed to a public Git repository, remaining active for 90 days.
- Rationale: Automated machine identities must never use static API keys. All machine auth must use OIDC workload federation with maximum token TTL capped at 15 minutes.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: WORKLOAD TOKEN TTL (P1-03)
Trigger: Workload Token Generation Request
├── Check: Is Token Type `WORKLOAD_FEDERATION`?
│   ├── YES
│   │   ├── Check: Requested TTL <= 900 seconds (15 minutes)?
│   │   │   ├── YES ──> Issue short-lived token with non-renewable claim
│   │   │   └── NO ──> [CLAMP: Force TTL to 900 seconds]
│   └── NO ──> Interactive User Token: Max TTL 3600 seconds with refresh lease
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: IdP Outage During Active Developer Shift
- Real-Life Incident: Okta experienced a global 4-hour outage; thousands of developers were locked out of deploying critical bug fixes to production.
- Decision Tree:
```
EDGE CASE 01: IDP OUTAGE RESILIENCE
Trigger: IdP Returns HTTP 5xx or Connection Timeout
├── Check: Does User possess an Active, Unexpired Platform Session JWT?
│   ├── YES ──> [CONTINUE: Authenticate via Local Platform Session — IdP roundtrip not required]
│   └── NO (User Session Expired or New Login)
│       ├── Check: Is Platform Break-Glass Local Identity Mode activated by 2 Platform Admins?
│       │   ├── YES ──> [EMERGENCY: Allow local break-glass password + Hardware YubiKey MFA]
│       │   └── NO ──> [FAIL CLOSED: Return HTTP 503 IdP Unavailable — Do not compromise security]
```

### Case 02: Replay Attack on Authorization Code Exchange
- Real-Life Incident: An eavesdropping proxy intercepted an OIDC authorization code and attempted to exchange it for an access token simultaneously with the legitimate user.
- Decision Tree:
```
EDGE CASE 02: CODE REPLAY DETECTION
Trigger: Token Exchange Request for Authorization Code
├── Action: Atomic Check-And-Delete code in Redis `GETDEL oidc_code:<code_hash>`
├── Check: Was code successfully retrieved and deleted?
│   ├── YES ──> [EXCHANGE: Code valid and consumed; return tokens]
│   └── NO (Code already consumed or absent)
│       └── [SECURITY REVOCATION: Reject Request + Invalidate all tokens issued from this code tree]
```

### Case 03: Conflicting Email Claims Across Federated Identity Providers
- Real-Life Incident: A user logged in via GitHub with `alice@company.com` and later via Corporate Okta with `alice@company.com`; the system collided on email and linked the accounts, allowing a hijacked GitHub account to compromise internal enterprise resources.
- Decision Tree:
```
EDGE CASE 03: CROSS-IDP IDENTITY COLLISION
Trigger: Login with Email matching existing user under different Issuer
├── Check: Does existing user have `issuer_A` while incoming login has `issuer_B`?
│   ├── YES
│   │   ├── Check: Has Enterprise Admin explicitly configured Cross-IdP Trust Link?
│   │   │   ├── YES ──> Require Step-Up Verification from primary IdP before linking
│   │   │   └── NO ──> [HARD ISOLATION: Create separate distinct user account; DO NOT automatically link]
│   └── NO ──> Same Issuer: Update claims normally
```

### Case 04: SCIM Group Deprovisioning Event Dropped
- Real-Life Incident: A SCIM webhook de-provisioning an employee failed due to an ingress timeout; the employee retained access for weeks after termination.
- Decision Tree:
```
EDGE CASE 04: SCIM WEBHOOK DELIVERY FAILURE
Trigger: SCIM Server returns HTTP 5xx or drops event
├── Check: Did IdP receive 200 OK from SCIM endpoint?
│   ├── NO
│   │   ├── Action: IdP retries with exponential backoff
│   │   └── Platform Reconciliation: Run nightly full-state SCIM reconciliation cron
│   │       ├── Diff: Compare IdP active users list with Platform User DB
│   │       └── Action: Automatically suspend any user absent from IdP roster
│   └── YES ──> Deprovisioning committed synchronously
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
IDENTITY INTEGRATION ROADMAP
│
├── PHASE 0: CORE OIDC AUTHENTICATION (P0 - WEEKS 1-3)
│   ├── Ingress OIDC Authorization Code Flow + PKCE implementation
│   ├── Robust cryptographic JWT verifier with cached JWKS provider
│   ├── Database models for users, sessions, and identity provider configs
│   └── Deliverable: Secure, authenticated developer sessions via Corporate IdP
│
├── PHASE 1: SCIM 2.0 & GROUP RECONCILIATION (P1 - WEEKS 4-6)
│   ├── SCIM 2.0 Ingress endpoints (`/Users`, `/Groups`)
│   ├── Enterprise group-to-platform team mapping engine
│   ├── Nightly drift detection reconciliation worker
│   └── Deliverable: Zero-touch user lifecycle automation from HR systems
│
├── PHASE 2: WORKLOAD IDENTITY FEDERATION (P2 - WEEKS 7-9)
│   ├── RFC 8693 token exchange for GitHub Actions, GitLab CI, and K8s ServiceAccounts
│   ├── SPIFFE/SPIRE mTLS integration for inter-service workload identities
│   ├── Short-lived credential issuer for cloud providers (AWS STS, GCP Workload)
│   └── Deliverable: Elimination of static API tokens across all CI/CD pipelines
│
└── PHASE 3: REAL-TIME THREAT REVOCATION & ZERO-TRUST SIGNALS (P3 - WEEKS 10-12)
    ├── Shared Signals and Events (SSE) / Continuous Access Evaluation Protocol (CAEP)
    ├── Real-time back-channel logout WebSocket killer
    └── Deliverable: Instantaneous enterprise-wide access termination (< 500ms)
```
