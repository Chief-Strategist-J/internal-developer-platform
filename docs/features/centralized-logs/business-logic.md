# Centralized Logs — Business Logic Decision Trees

## 1. Architectural Overview & Logging Pipeline Decision Tree

```
CENTRALIZED LOGS ENGINE (INGESTION, BUFFERING & INDEXING DECISION TREE)
│
├── 1. LOG CAPTURE & NODE-LEVEL BUFFERING TIER [P0]
│   └── Real-World: 1,000 Kubernetes Pods emit JSON log lines to `/dev/stdout` and `/dev/stderr`
│       └── Trigger: Container stdout write captured by CRI (containerd) -> Node Log Daemon (FluentBit / Vector)
│           ├── Action: Enrich with Kubernetes Metadata (Namespace, PodName, Container, App, Version, TenantID)
│           ├── Check: Is Log Line Valid JSON?
│           │   ├── YES ──> Parse structured fields (`level`, `message`, `trace_id`, `span_id`)
│           │   └── NO ──> Wrap in unstructured envelope: `{ "raw": log_line, "level": "UNKNOWN" }`
│           ├── Check: Local Node Ring Buffer Utilization > 80%?
│           │   ├── YES ──> [APPLY LOCAL BACKPRESSURE: Flush to disk queue to avoid dropping logs]
│           │   └── NO ──> Dispatch batch to Central Log Collector Gateway
│           │
├── 2. SANITIZATION & FIELD-LEVEL REDACTION TIER [P0]
│   └── Real-World: Developer accidentally logs user request payload containing credit card numbers
│       └── Evaluation: Data Loss Prevention (DLP) Stream Filter
│           ├── Action: Evaluate stream against high-performance regex filters (PAN, SSN, API Tokens, Passwords)
│           ├── Check: Sensitive Pattern Detected in Log Payload?
│           │   ├── YES ──> [MUTATE: Replace matched substring with `[REDACTED_SECRET]` + Emit DLP Audit Metric]
│           │   └── NO ──> Payload Clean
│           │
├── 3. STREAM BUFFERING & RATE-LIMITING TIER [P0 / P1]
│   └── Real-World: Runaway debug loop in a microservice attempts to emit 100,000 logs/second
│       └── Evaluation: Ingestion Rate Limiter & Kafka / Redis Buffer
│           ├── Check: Does Ingestion Rate for Tenant/Service exceed Quota (e.g. > 10 MB/sec)?
│           │   ├── YES
│           │   │   ├── Action: Sample / Rate-Limit (Drop DEBUG logs first, preserve WARN/ERROR)
│           │   │   └── Action: Emit Throttle Warning Log: `[LOG_QUOTA_EXCEEDED: 4500 lines dropped]`
│           │   └── NO ──> Ingest clean batch into Distributed Log Queue (Kafka / Vector Ingress)
│           │
├── 4. HOT INDEXING & REAL-TIME SEARCH TIER [P0]
│   └── Real-World: Writing logs to OpenSearch / Elasticsearch / Grafana Loki for developer debugging
│       └── Evaluation: Index Routing
│           ├── Action: Route to Tenant-Scoped Daily / Time-Partitioned Index (`logs-<tenant>-YYYY.MM.DD`)
│           ├── Action: Index searchable metadata fields (`service`, `trace_id`, `status_code`)
│           └── State: LOG_SEARCHABLE (Available in Portal Log Viewer within 3 seconds of emission)
│
└── 5. RETENTION & MULTI-TIER STORAGE LIFECYCLE TIER [P1]
    └── Real-World: 30-day compliance retention; tiering aged logs to cheap S3 compressed Parquet
        └── Trigger: Scheduled Storage Tiering Daemon (Nightly Cron)
            ├── Hot Storage (OpenSearch / Loki SSD): Retain for 7 Days (Instant interactive search)
            ├── Warm Storage (Compressed Object Storage): Retain for 30 Days (Async query / Athena)
            └── Cold Archive (AWS Glacier / GCS Coldline): Retain for 365 Days (Compliance audit)
                └── Action: Delete index from Hot Cluster cleanly after successful S3 sync
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Plaintext Credentials in Central Log Store
- Real-Life Incident: A backend engineer printed HTTP headers including `Authorization: Bearer <token>`; an internal attacker queried Elasticsearch and gathered 50 valid admin tokens.
- Rationale: Log ingestion pipelines must enforce automatic, irreversible redaction of authentication tokens, passwords, and PII before indexing.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: LOG REDACTION (P0-01)
Trigger: Ingestion Stream Processing
├── Pattern Match: Bearer tokens, JWT headers, private keys, Credit Card Luhn checksums
├── Check: Match Found?
│   ├── YES ──> [MUTATE: Overwrite target bytes with deterministic redaction tag]
│   └── NO ──> Pass through
```

### Invariant P0-02: Tenant-Isolated Log Index Boundaries
- Real-Life Incident: An OpenSearch query without index filtering leaked Tenant A's customer query logs to a developer working on Tenant B.
- Rationale: Multi-tenant logs must be partitioned into distinct indexes or tagged with cryptographic tenant labels enforced at query time.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: TENANT LOG ISOLATION (P0-02)
Trigger: Log Search Query Evaluation
├── Action: Inject mandatory query filter: `tenant_id == :requester_tenant_id`
├── Enforce: Global queries across tenants strictly restricted to Platform SecOps role
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Runaway Log Ingestion Exhausts Disk on Worker Nodes
- Real-Life Incident: A container threw an infinite stacktrace; the node's `/var/log` disk reached 100%, causing Kubernetes to mark the node `DiskPressure` and evict all pods.
- Decision Tree:
```
EDGE CASE 01: LOG DISK PRESSURE
Trigger: Node Root Disk Space < 15%
├── Action: Container Log Rate-Limiting via CRI
├── Action: Kubelet triggers log rotation and truncates oversized log files
└── Action: Throttles container stdout/stderr buffer to protect node stability
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
CENTRALIZED LOGS ROADMAP
│
├── PHASE 0: LOG COLLECTION & DLP REDACTION (P0 - WEEKS 1-3)
│   ├── DaemonSet deployment of FluentBit / Vector across all Kubernetes nodes
│   ├── High-performance regex DLP redaction filter for secrets and PII
│   ├── Structured JSON log parser with Kubernetes metadata enrichment
│   └── Deliverable: Secure, standardized log stream ingestion with zero secret leaks
│
├── PHASE 1: OPENSEARCH / GRAFANA LOKI INDEXING (P1 - WEEKS 4-6)
│   ├── Multi-tenant log index routing and tenant-isolated search API
│   ├── Integration with Developer Portal for in-context service log streaming
│   └── Deliverable: Sub-3-second log searchability for developer troubleshooting
│
├── PHASE 2: RATE-LIMITING & COST GOVERNANCE (P2 - WEEKS 7-9)
│   ├── Adaptive log rate-limiting per service to prevent log storms
│   ├── Dynamic log-level adjustment via runtime API (DEBUG on-demand for 15 mins)
│   └── Deliverable: 50% log storage cost reduction with runaway storm protection
│
└── PHASE 3: DISTRIBUTED LOG-TRACE CORRELATION & COLD TIERING (P3 - WEEKS 10-12)
    ├── Seamless 1-click jump between Log lines and OpenTelemetry Traces
    ├── Automated S3 Parquet archiving and Athena serverless query integration
    └── Deliverable: Enterprise-grade compliance archiving with long-term retention
```
