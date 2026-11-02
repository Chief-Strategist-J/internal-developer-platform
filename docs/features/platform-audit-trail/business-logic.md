# Platform Audit Trail — Business Logic Decision Trees

## 1. Architectural Overview & Audit Stream Decision Tree

```
PLATFORM AUDIT TRAIL ENGINE (INGESTION, VERIFICATION & TAMPER-RESISTANCE DECISION TREE)
│
├── 1. EVENT EMISSION & STANDARDIZATION TIER [P0]
│   └── Real-World: Developer executes schema migration, modifies secret, or deletes namespace
│       └── Trigger: Ingress Platform Action Intercepted by Middleware / Outbox Listener
│           ├── Action: Extract Standard CloudEvents / CNCF Audit Schema Attributes:
│           │   ├── EventID: UUIDv7 (Monotonic, Time-Ordered)
│           │   ├── Timestamp: UTC Nanoseconds
│           │   ├── Actor: (SubjectID, SessionID, UserAgent, IngressIP, TenantID)
│           │   ├── Action: (Service, Method, Verb, APIVersion)
│           │   ├── TargetResource: (Type, ResourceID, TenantID, Environment)
│           │   └── Outcome: (Status, HTTPCode, ErrorCode, LatencyMs)
│           ├── Check: Does Event Payload contain Unredacted Plaintext Secrets / PII?
│           │   ├── YES ──> Action: Apply Field-Level Redaction Filter via Deterministic Masking
│           │   └── NO ──> Schema Normalized
│           │
├── 2. SYNCHRONOUS OUTBOX & WRITE PIPELINE TIER [P0]
│   └── Real-World: Critical security event must survive sudden process termination or crash
│       └── Evaluation: Audit Persistence Model
│           ├── Check: Is Event Classified as TIER-0 AUDIT (Auth Denial, Secret Access, Break-Glass, Deletion)?
│           │   ├── YES (Tier-0 Critical)
│           │   │   ├── Action: Write to Transactional Outbox table in SAME ACID transaction as state mutation
│           │   │   └── Check: DB Commit Succeeded?
│           │   │       ├── NO ──> Mutation Rolls Back; No Inconsistent State
│           │   │       └── YES ──> Guaranteed Event Delivery
│           │   └── NO (Tier-1 Informational Read / Query)
│           │       └── Action: Dispatch via Asynchronous Ring Buffer / Kafka Producer
│           │
├── 3. CRYPTOGRAPHIC PROVABLE INTEGRITY TIER [P0 / P1]
│   └── Real-World: Compliance auditor demands mathematical proof that audit logs have not been tampered with
│       └── Evaluation: Merkle Tree & Cryptographic Chain Construction
│           ├── Action: Compute Leaf Hash: `H_n = SHA256(H_{n-1} + EventPayload_n + Nonce)`
│           ├── Action: Append to Tenant-Scoped Tamper-Proof Cryptographic Hash Chain
│           ├── Action: Batch Leaves into Merkle Tree root every 60 seconds
│           ├── Action: Publish Root Hash to Write-Once-Read-Many (WORM) External Storage / Cloud Object Lock
│           └── State: EVENT_SEALED_IMMUTABLE
│
├── 4. RETENTION & MULTI-TIER COLD STORAGE TIER [P1]
│   └── Real-World: Regulatory compliance mandates 7-year audit retention with cold tiering
│       └── Trigger: Scheduled Storage Tiering Worker
│           ├── Hot Storage (PostgreSQL / Elasticsearch): 0 to 90 Days (Real-time search & SecOps SIEM)
│           ├── Warm Storage (Compressed S3 Standard): 91 to 365 Days (Athena / BigQuery analytics)
│           └── Cold Archive (AWS Glacier / GCS Archive with Object Lock / Legal Hold): 1 to 7 Years
│               └── Action: Enforce WORM Mode — deletion blocked at cloud storage hardware layer
│
└── 5. ANOMALY DETECTION & REAL-TIME SIEM DISPATCH TIER [P1 / P2]
    └── Real-World: Compromised credential triggers 50 API deletions across 5 minutes
        └── Evaluation: Stream Processing Pipeline (Flink / Kafka Streams)
            ├── Rule Check 1: Sudden Spike in 403 Denials (> 10 denials / minute for same IP/User)
            │   ├── MATCH ──> Action: Trigger Suspicious Activity Alert to SecOps Slack & PagerDuty
            ├── Rule Check 2: Geo-Velocity Anomaly (User signs in from Tokyo 10 mins after New York)
            │   ├── MATCH ──> Action: Terminate Active Session + Invalidate Revocation Cache
            └── Rule Check 3: Mass Resource Deletion (> 5 production resources in 60s)
                ├── MATCH ──> Action: Trigger Circuit Breaker to suspend automated deletions
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Guaranteed Delivery for Critical Security Actions (ACID Outbox)
- Real-Life Incident: A disgruntled engineer dropped a production database; the auditing microservice was temporarily offline, resulting in zero records of who initiated the teardown.
- Rationale: High-risk administrative actions must be atomic with their audit record. If the audit event cannot be committed to the outbox, the business operation must be aborted.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ACID AUDIT OUTBOX (P0-01)
Trigger: Destructive or Privileged Mutation Initiated
├── BEGIN TRANSACTION
│   ├── Execute Business Mutation (e.g. Delete Cluster / Update Secret)
│   ├── Insert Audit Record into `audit_outbox` table
│   └── COMMIT
├── Check: Did Commit Succeed?
│   ├── YES ──> Operation Finalized; Outbox Worker Relays to SIEM
│   └── NO ──> [ROLLBACK: Entire Operation Aborted — No unrecorded changes allowed]
```

### Invariant P0-02: Cryptographic Immutability (WORM & Hash Chaining)
- Real-Life Incident: A root attacker gained database access and executed `UPDATE audit_logs SET user_id = 'innocent'` to frame a colleague.
- Rationale: Audit logs must form an append-only, mathematically verifiable hash chain. Any retrofitted alteration breaks the Merkle tree verification and exposes tampering.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MERKLE HASH CHAIN (P0-02)
Trigger: Audit Event Insertion
├── Read: Previous Event Hash `H_{prev}`
├── Compute: `H_{curr} = SHA256(H_{prev} || EventData)`
├── Write: Persist Event with `H_{curr}` and `H_{prev}`
├── Lock: Database table triggers REJECT any `UPDATE` or `DELETE` on `audit_events`
└── Push: Periodic Merkle Root sent to Cloud WORM storage (S3 Object Lock Compliance Mode)
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Audit Ingestion Backpressure under DDoS / Event Flood
- Real-Life Incident: A massive distributed attack flooded the API with unauthenticated requests; the audit queue filled up, consuming all memory and crashing the platform API gateway.
- Decision Tree:
```
EDGE CASE 01: AUDIT EVENT BACKPRESSURE
Trigger: Audit Ingestion Buffer exceeds 80% Capacity
├── Check: Event Priority Classification
│   ├── TIER 0 (Authenticated Mutations, Admin Actions, Privileged Access)
│   │   └── [RESERVED BUFFER: Prioritize in dedicated un-throttled queue]
│   └── TIER 2 (Public Unauthenticated Ingress 404s / Rate Limit Hits)
│       └── [AGGREGATE & DROP: Sample high-volume noise; emit aggregated counter metric instead of discrete logs]
```

### Case 02: PII Redaction Failure Exposes Customer Cardholder Data
- Real-Life Incident: A microservice included raw credit card payloads in an HTTP error string; the audit log indexed the plain card numbers in an unencrypted search cluster.
- Decision Tree:
```
EDGE CASE 02: RUNTIME PII REDACTION
Trigger: Event Ingestion before Indexing
├── Action: Run regex scanner for sensitive patterns (PAN, SSN, API Tokens, Private Keys)
├── Check: Sensitive Pattern Detected?
│   ├── YES ──> [MASK: Replace with `[REDACTED_PII]` + Flag Event for Data Loss Prevention Alert]
│   └── NO ──> Ingest clean payload
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
PLATFORM AUDIT TRAIL ROADMAP
│
├── PHASE 0: TRANSACTIONAL OUTBOX & CORE SCHEMA (P0 - WEEKS 1-3)
│   ├── Standardized CloudEvents schema for all platform mutations
│   ├── PostgreSQL transactional outbox pattern implementation
│   ├── Database-level DDL guards preventing `UPDATE` and `DELETE` on audit tables
│   └── Deliverable: Zero unrecorded mutations with ACID delivery guarantee
│
├── PHASE 1: MERKLE CHAINING & IMMUTABLE STORAGE (P1 - WEEKS 4-6)
│   ├── Cryptographic SHA-256 hash chaining implementation
│   ├── Cloud WORM storage sync (AWS S3 Object Lock / GCP Bucket Lock)
│   ├── Field-level automated PII/Secret redaction pipeline
│   └── Deliverable: Tamper-proof, cryptographically provable audit record stream
│
├── PHASE 2: SEARCH, QUERY API & RETENTION TIERING (P2 - WEEKS 7-9)
│   ├── High-performance multi-tenant search API with audit indexing
│   ├── Automated storage tiering (Hot -> Warm -> Cold Glacier)
│   └── Deliverable: Fast, compliance-ready audit search across historical data
│
└── PHASE 3: REAL-TIME SIEM STREAMING & THREAT HEURISTICS (P3 - WEEKS 10-12)
    ├── Real-time Kafka / Webhook streaming to Datadog, Splunk, and Panther
    ├── Real-time velocity and brute-force anomaly detection triggers
    └── Deliverable: Instantaneous security threat alerting and automated containment
```
