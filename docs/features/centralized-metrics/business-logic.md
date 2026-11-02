# Centralized Metrics — Business Logic Decision Trees

## 1. Architectural Overview & Metrics Ingestion Decision Tree

```
CENTRALIZED METRICS ENGINE (SCRAPING, COMPACTION & ALERTING DECISION TREE)
│
├── 1. TARGET DISCOVERY & METRIC SCRAPING TIER [P0]
│   └── Real-World: Prometheus Operator discovers new pod with `prometheus.io/scrape: "true"`
│       └── Trigger: Scraping Interval (Every 15 seconds)
│           ├── Action: Discover Pod IP & Metric Port via Kubernetes API ServiceMonitor
│           ├── Action: Execute HTTP GET `/metrics` with 10-second timeout
│           ├── Check: Does Target respond with Valid OpenMetrics / Prometheus Text Format?
│           │   ├── NO (Timeout / HTTP 500 / Malformed Text)
│           │   │   ├── Action: Increment `up{service="app"} = 0`
│           │   │   └── State: TARGET_UNREACHABLE
│           │   └── YES
│           │       ├── Action: Increment `up{service="app"} = 1`
│           │       └── Ingest Metric Series (Counters, Gauges, Histograms)
│           │
├── 2. HIGH-CARDINALITY & SANITIZATION TIER [P0 / P1]
│   └── Real-World: Developer adds `user_id` or `email` as a metric label, creating 1,000,000 unique series
│       └── Evaluation: Cardinality Guard Filter
│           ├── Action: Count Unique Active Time Series per Metric Name
│           ├── Check: Does Metric contain High-Cardinality Labels (`user_id`, `request_id`, `ip_address`)?
│           │   ├── YES
│           │   │   ├── Action: Drop High-Cardinality Label immediately at scraper level
│           │   │   └── Action: Emit Cardinality Violation Warning to Service Owner
│           │   └── NO ──> Check: Has Service exceeded Maximum Series Quota (e.g. 10,000 series)?
│           │       ├── YES ──> [DROP NEW SERIES: Prevent Prometheus Time-Series DB (TSDB) out-of-memory crash]
│           │       └── NO ──> Series Ingested Cleanly
│           │
├── 3. TSDB STORAGE & DOWNSAMPLING TIER [P0 / P1]
│   └── Real-World: Storing billions of metric points in Thanos / Mimir / VictoriaMetrics
│       └── Evaluation: Storage Lifecycle & Compaction
│           ├── Raw Resolution (15s): Stored in In-Memory WAL + Local SSD for 48 Hours
│           ├── 5-Minute Downsampling: Compacted after 48 Hours -> Shipped to Object Storage (S3)
│           └── 1-Hour Downsampling: Retained for 13 Months (Year-over-Year trend capacity planning)
│           └── State: METRIC_PERSISTED
│
└── 4. ALERT EVALUATION & ESCALATION TIER [P0]
    └── Real-World: PromQL evaluates `rate(http_requests_total{status=~"5.."}[5m]) > 0.05`
        └── Trigger: Alert Rule Evaluation Loop (Every 30 seconds)
            ├── Action: Execute PromQL query against active TSDB engine
            ├── Check: Does Metric expression evaluate to TRUE?
            │   ├── NO ──> State: INACTIVE (Clear active alert if present)
            │   └── YES ──> State: PENDING (Wait for `for: 2m` duration to prevent flukes)
            │       ├── Check: Has Condition persisted for entire 2-minute pending window?
            │       │   ├── NO ──> Fluke resolved; return to INACTIVE
            │       │   └── YES ──> State: FIRING ──> Dispatch Alert to Alertmanager / PagerDuty / Slack
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Cardinality Explosion Prevention (TSDB Protection)
- Real-Life Incident: A microservice appended dynamic credit card numbers to metric labels; Prometheus RAM usage surged from 16 GB to 256 GB, crashing the cluster monitoring system during an outage.
- Rationale: High cardinality kills time-series databases. The metrics ingestion layer must drop unauthorized labels and clamp the maximum number of series per service.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: CARDINALITY LIMIT (P0-01)
Trigger: Metric Ingestion Scrape
├── Check: Total Active Series for Service > MaxAllowedSeries (10,000)?
│   ├── YES ──> [HARD CLAMP: Reject new unique metric series; retain existing series; emit warning]
│   └── NO ──> Ingest metric series
```

### Invariant P0-02: Alert Rule Determinism & Zero Flapping
- Real-Life Incident: A raw metric rule fired without a `for:` stabilization duration; temporary network blips woke up engineers 12 times a night with self-resolving false alarms.
- Rationale: Alerting rules must include a minimum pending stabilization period (`for: 2m` or `for: 5m`) to ensure alerts represent sustained failures.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ALERT STABILIZATION (P0-02)
Trigger: Alert Rule Registration
├── Check: Does Alert Rule define `for >= 2m`?
│   ├── NO ──> [REJECT: Alert Rule Lint Error — All alert rules must define stabilization window]
│   └── YES ──> Register Alert Rule
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Scrape Timeout on Memory-Constrained Pod
- Real-Life Incident: A Java pod undergoing major GC paused for 15 seconds; Prometheus scrape timed out, falsely reporting the service as dead (`up == 0`).
- Decision Tree:
```
EDGE CASE 01: SCRAPE TIMEOUT HANDLING
Trigger: Prometheus Scrape Times Out
├── Check: Did Service respond to previous 2 consecutive scrapes?
│   ├── YES ──> Mark single missed scrape without triggering immediate service dead alert
│   └── NO (3 consecutive timeouts) ──> Confirm Service Unreachable (`up = 0`)
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
CENTRALIZED METRICS ROADMAP
│
├── PHASE 0: PROMETHEUS SCRAPING & SERVICE MONITORS (P0 - WEEKS 1-3)
│   ├── Prometheus Operator deployment with standard ServiceMonitor CRDs
│   ├── Default infrastructure and Kubernetes cluster dashboards in Grafana
│   ├── High-cardinality label dropping filters
│   └── Deliverable: Robust multi-cluster metric collection and visualization
│
├── PHASE 1: LONG-TERM STORAGE WITH THANOS / MIMIR (P1 - WEEKS 4-6)
│   ├── Thanos Sidecar and Store Gateway deployment with S3 backend
│   ├── Automated 5-minute and 1-hour downsampling pipelines
│   └── Deliverable: Cost-effective 13-month metric retention with sub-second queries
│
├── PHASE 2: ALERTING HYGIENE & ALERTMANAGER ROUTING (P2 - WEEKS 7-9)
│   ├── Unified Alertmanager routing rules (Slack for warnings, PagerDuty for Sev-1)
│   ├── Automated metric rule linter in CI/CD (blocking non-deterministic rules)
│   └── Deliverable: Zero false-alarm noise with actionable on-call alerts
│
└── PHASE 3: ADAPTIVE SAMPLING & ANOMALY DETECTION (P3 - WEEKS 10-12)
    ├── Dynamic scrape frequency adjustment based on service criticality
    └── Deliverable: Optimized telemetry cost with zero visibility loss on tier-0 services
```
