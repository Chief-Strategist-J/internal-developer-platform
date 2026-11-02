# Secrets Management — Business Logic Decision Trees

## 1. Architectural Overview & Secrets Lifecycle Decision Tree

```
SECRETS MANAGEMENT ENGINE (LIFECYCLE & INJECTION DECISION TREE)
│
├── 1. SECRET CREATION & ENCRYPTION AT REST TIER [P0]
│   └── Real-World: Developer provisions a new Postgres database secret via Developer Portal
│       └── Trigger: Inbound Secret Create / Put Request
│           ├── Check: Is Plaintext Payload within size limits (<= 64 KB)?
│           │   ├── NO ──> [REJECT: HTTP 413 Payload Too Large — Secrets cannot exceed 64 KB]
│           │   └── YES ──> Check: Tenant Key Encryption Key (KEK) available in Cloud KMS / Vault?
│           │       ├── NO ──> [REJECT: HTTP 503 Service Unavailable — KMS Key Ring Unreachable]
│           │       └── YES ──> Action: Generate Ephemeral DEK (Data Encryption Key) via AES-256-GCM
│           │           ├── Encrypt Plaintext with DEK + Associated Data (TenantID + SecretPath)
│           │           ├── Encrypt DEK with KMS KEK (Envelope Encryption)
│           │           └── Action: Persist ciphertext, encrypted DEK, IV, and tag in DB with version=1
│           │               └── State: SECRET_VERSION_COMMITTED
│           │
├── 2. WORKLOAD ACCESS & JUST-IN-TIME INJECTION TIER [P0]
│   └── Real-World: Kubernetes Pod boots up; Secret CSI driver requests dynamic injection
│       └── Trigger: Workload Secret Fetch Request (mTLS + Pod ServiceAccount Token)
│           ├── Check: Is Pod ServiceAccount authenticated and bound to Target Secret Path?
│           │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Workload Identity Not Authorized for Path]
│           │   └── YES ──> Check: Is Secret Type STATIC or DYNAMIC (Ephemeral Ephemeral Lease)?
│           │       ├── STATIC SECRET
│           │       │   ├── Action: Fetch KMS KEK to decrypt DEK -> Decrypt Ciphertext in Memory
│           │       │   └── Output: Stream Plaintext to Workload Memory / In-Memory tmpfs volume
│           │       └── DYNAMIC SECRET (e.g. Ephemeral Database Role)
│           │           ├── Action: Generate short-lived DB credential on upstream DB engine (TTL: 1 hr)
│           │           ├── Action: Register Lease ID with Lease Revocation Scheduler
│           │           └── Output: Deliver dynamic user/pass with renewal lease token
│           │
├── 3. AUTOMATED SECRET ROTATION TIER [P1]
│   └── Real-World: 90-day compliance rotation timer triggers for high-privilege Stripe API key
│       └── Trigger: Rotation Cron or Manual Rotation Request
│           ├── Action: Mark current version active_v1
│           ├── Action: Generate new secret value active_v2 via Target Service API
│           ├── Action: Enter DUAL_ACTIVE State (both v1 and v2 accepted by workloads)
│           │   └── Workloads reload v2 via rolling restart or dynamic file watch
│           ├── Check: Have all consuming workloads acknowledged consumption of v2?
│           │   ├── NO (Within Grace Period of 24h) ──> Maintain Dual Active
│           │   └── YES (or Grace Period Expired)
│           │       ├── Action: Deprecate v1 on upstream provider (Stripe API)
│           │       └── State: ROTATION_COMPLETED (v2 becomes sole active version)
│           │
├── 4. LEASE EXPIRATION & AUTO-REVOCATION TIER [P0]
│   └── Real-World: Development Pod terminates; temporary database credentials must be deleted
│       └── Trigger: Lease Heartbeat Expired or Workload Pod Evicted
│           ├── Check: Was Lease Renewed before expiration window?
│           │   ├── YES ──> Extend Lease TTL up to Max Allowed Lease Duration (e.g. 24 hrs)
│           │   └── NO ──> Action: Trigger Upstream Revocation Worker
│           │       ├── Action: Connect to Target DB engine and execute `DROP USER <ephemeral_user>`
│           │       └── Check: Did Upstream Revocation succeed?
│           │           ├── YES ──> [CONFIRM: Lease Annihilated; Emit Audit Event]
│           │           └── NO ──> [ALERT: Mark Lease ORPHANED; Queue in Dead-Letter-Queue for Ops]
│           │
└── 5. COMPROMISE DISCOVERY & EMERGENCY KILL-SWITCH TIER [P0]
    └── Real-World: Security finds secret leaked in public GitHub issue; SecOps triggers nuclear revocation
        └── Trigger: Emergency Secret Revocation Event
            ├── Action: Immediately invalidate in-memory secret caches across all cluster PDPs
            ├── Action: Revoke upstream credential on cloud provider / target database synchronously
            ├── Action: Flag all existing active leases as REVOKED
            ├── Action: Trigger rolling restart of all dependent workloads with fallback placeholder
            └── [CONFIRM: Incident Mitigated; Emit Critical SIEM Audit Record]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Plaintext to Persistent Storage
- Real-Life Incident: A developer added debug logging in an API handler that printed incoming secrets; database backups subsequently exposed thousands of plain API keys in cold storage.
- Rationale: Plaintext secrets must never touch persistent storage, disk swap spaces, or application log files. Plaintext exists strictly in ephemeral volatile memory during cryptographic operations.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ZERO PLAINTEXT PERSISTENCE (P0-01)
Trigger: Secret Serialization / Logging / DB Persistence
├── Check: Is Data payload flagged as Plaintext Secret?
│   ├── YES
│   │   ├── Check: Is target destination an Ephemeral MemBuffer (CSI tmpfs or volatile RAM)?
│   │   │   ├── YES ──> [PERMIT: Volatile memory write]
│   │   │   └── NO (Disk, DB, Log Stream, Metric Label) ──> [HARD REDACT: Replace with '[REDACTED_SECRET]']
│   └── NO ──> [PERMIT: Standard Serialization]
```

### Invariant P0-02: Cryptographic Envelope Encryption per Tenant
- Real-Life Incident: A single shared master encryption key was compromised, exposing every secret across all 50 enterprise customers hosted on the multi-tenant cluster.
- Rationale: Every tenant must have an isolated Key Encryption Key (KEK). Data Encryption Keys (DEKs) must be unique per secret version and encrypted with the tenant's specific KEK.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: TENANT ENVELOPE ENCRYPTION (P0-02)
Trigger: Secret Encryption Request
├── Extract: Tenant ID
├── Fetch: Tenant KEK from Hardware Security Module (HSM) / Cloud KMS
├── Check: Is Tenant KEK Active and strictly isolated to Tenant's IAM ARN?
│   ├── NO ──> [HALT: Cryptographic Boundary Error]
│   └── YES ──> Generate fresh random 256-bit DEK -> Encrypt Secret -> Encrypt DEK with KEK
```

### Invariant P1-03: Dual-Active State during Rotation
- Real-Life Incident: A secrets manager rotated an OAuth client secret instantly; hundreds of microservices still executing with the old secret threw HTTP 401s, bringing down checkout for 45 minutes.
- Rationale: Hard-cut rotations cause zero-downtime failures. The previous secret version must remain valid for a bounded grace period while workloads roll over.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: DUAL-ACTIVE ROTATION (P1-03)
Trigger: Secret Rotation Event
├── Action: Generate Version N+1
├── Check: Does Target Provider support Concurrent Active Keys?
│   ├── YES ──> Enable N and N+1 concurrently; deprecate N after Workload Confirmation Window
│   └── NO ──> Require Blue/Green Service Deployment switch before revoking N
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Cloud KMS Outage During Pod Autoscaling Burst
- Real-Life Incident: AWS KMS suffered regional rate-limiting during a Black Friday traffic surge; 200 new auto-scaled Pods could not decrypt their database credentials and crashed in CrashLoopBackOff.
- Decision Tree:
```
EDGE CASE 01: KMS RATE LIMIT RESILIENCE
Trigger: High-Volume Secret Decryption under KMS Throttling
├── Check: Is Local Secure Cache (AES-GCM encrypted in tmpfs) available for target secret version?
│   ├── YES ──> [SERVE FROM CACHE: Decrypt locally without KMS roundtrip]
│   └── NO
│       ├── Check: KMS API returns 429 Too Many Requests?
│       │   ├── YES ──> [EXPONENTIAL JITTER BACKOFF: Retry up to 3 times with decorrelated jitter]
│       │   └── NO ──> Return 503 and alert platform on-call
```

### Case 02: Secret Injection Mount Failure on Node Eviction
- Real-Life Incident: A Kubernetes worker node ran out of memory; the CSI secrets driver failed to unmount the tmpfs volume cleanly, causing secret leak warnings on node reboot.
- Decision Tree:
```
EDGE CASE 02: DIRTY NODE UNMOUNT
Trigger: Node Shutdown or CSI Driver Crash
├── Check: Are unmounted secret volumes lingering in `/var/lib/kubelet/pods/...`?
│   ├── YES ──> [CLEANUP DAEMON: Execute secure memory scrub `shred -u` on tmpfs files before node halt]
│   └── NO ──> Clean exit
```

### Case 03: Orphaned Dynamic Database Lease on Orchestrator Crash
- Real-Life Incident: The platform vault pod crashed while managing 500 short-lived MySQL user accounts; the accounts never expired and exhausted the MySQL connection pool.
- Decision Tree:
```
EDGE CASE 03: ORPHANED DYNAMIC LEASES
Trigger: Platform Recovery after Orchestrator Restart
├── Action: Load all active leases from persistent DB state
├── Check: Are any leases past `expires_at` timestamp?
│   ├── YES ──> [SWEEPER TASK: Connect to upstream targets and execute batch drop for expired users]
│   └── NO ──> Re-register renewal timers for unexpired leases
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
SECRETS MANAGEMENT ROADMAP
│
├── PHASE 0: ENVELOPE ENCRYPTION & STATIC SECRETS (P0 - WEEKS 1-3)
│   ├── KMS / HashiCorp Vault backend integration with AES-256-GCM envelope encryption
│   ├── Kubernetes Secrets Store CSI Driver integration (tmpfs mounts)
│   ├── Tenant-isolated key hierarchies and strict redaction filters
│   └── Deliverable: Production-ready encrypted secret storage and K8s Pod injection
│
├── PHASE 1: DYNAMIC SECRETS & SHORT-LIVED LEASES (P1 - WEEKS 4-6)
│   ├── Ephemeral database credential generation (Postgres, MySQL, Mongo)
│   ├── Automated lease renewal heartbeat daemon and sweeper
│   └── Deliverable: Zero static database credentials across staging and production
│
├── PHASE 2: AUTOMATED ROTATION WORKFLOWS (P2 - WEEKS 7-9)
│   ├── Dual-active rotation engine for API keys, certificates, and OAuth secrets
│   ├── Workload reload webhooks and rolling restart coordination
│   └── Deliverable: Scheduled zero-downtime 90-day automated key rotation
│
└── PHASE 3: HARDWARE SECURITY & DISASTER RECOVERY (P3 - WEEKS 10-12)
    ├── Multi-region KMS key replication with regional failover
    ├── HSM-backed root keys (FIPS 140-2 Level 3 compliance)
    └── Deliverable: Global disaster-recovery resilience with zero plaintext risk
```
