# Deployment Strategies — Business Logic Decision Trees

## 1. Architectural Overview & Strategy Selection Decision Tree

```
DEPLOYMENT STRATEGIES ENGINE (STRATEGY DISPATCH & TRAFFIC PROGRESSION DECISION TREE)
│
├── 1. STRATEGY INGESTION & CAPABILITY RESOLUTION TIER [P0]
│   └── Real-World: Developer submits `rollout.yaml` specifying Canary rollout with Istio mesh
│       └── Trigger: Inbound Deployment Request
│           ├── Check: Is Requested Strategy (Canary, Blue/Green, RollingUpdate, Recreate) supported?
│           │   ├── NO ──> [REJECT: HTTP 422 Unprocessable Entity — Unsupported Strategy]
│           │   └── YES ──> Check: Does Target Cluster have required Ingress / Service Mesh provider?
│           │       ├── CANARY WITH TRAFFIC SPLIT (e.g. Istio / Linkerd / AWS ALB)
│           │       │   ├── Mesh Missing ──> [FALLBACK: Degrade to Pod-Ratio Canary or RollingUpdate]
│           │       │   └── Mesh Present ──> Strategy Validated: DYNAMIC_WEIGHT_CANARY
│           │       ├── BLUE / GREEN
│           │       │   ├── Check: Does Cluster have 2x Pod capacity for duplicate active pool?
│           │       │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Insufficient Quota for 2x Blue/Green Pool]
│           │       │   │   └── YES ──> Strategy Validated: BLUE_GREEN
│           │       └── ROLLING UPDATE / RECREATE ──> Strategy Validated: STANDARD_K8S
│           │
├── 2. CANARY TRAFFIC SHIFTING & PROGRESSION TIER [P0 / P1]
│   └── Real-World: Progressive rollout shifts 5% -> 20% -> 50% -> 100% traffic over 30 minutes
│       └── State: CANARY_ROLLOUT_ACTIVE
│           ├── Step 1: Deploy Canary Pod Replica Set (Version N+1)
│           ├── Step 2: Route 5% Traffic via VirtualService weight / Ingress annotation
│           ├── Step 3: Analysis Pause Window (Duration: 5 minutes)
│           │   ├── Action: Query Prometheus / Datadog for Canary Health Metrics:
│           │   │   ├── Metric A: HTTP 5xx Error Rate <= 0.5%?
│           │   │   ├── Metric B: P99 Latency <= 250ms?
│           │   │   └── Metric C: Custom Business Metrics (e.g. Payment Drop Rate == 0)?
│           │   ├── ANY METRIC VIOLATED ──> [HALT: Trigger Automated Rollback — Shift Traffic 100% back to Baseline]
│           │   └── ALL METRICS HEALTHY ──> Action: Step Traffic to 20%
│           ├── Step 4: Repeat Analysis for Step 20%, 50%
│           └── Step 5: Promote Canary to 100% Traffic -> Scale down Baseline Pods
│               └── State: CANARY_PROMOTED ──> [CONFIRM: Deployment Complete]
│
├── 3. BLUE / GREEN ENVIRONMENT SWITCHING TIER [P0]
│   └── Real-World: Deploying Green stack in parallel; cutover instant via Service selector update
│       └── State: BLUE_GREEN_ACTIVE
│           ├── Action: Deploy Green Pods (Version N+1) alongside Blue Pods (Version N)
│           ├── Action: Run Synthetic Smoke Tests directly against Green Service Endpoint
│           ├── Check: Did all Smoke Tests pass with 100% success?
│           │   ├── NO ──> [ABORT: Terminate Green Stack — Zero customer traffic affected]
│           │   └── YES ──> Action: Atomic Cutover (Update Kubernetes Service selector from Blue to Green)
│           ├── Action: Post-Cutover Soak Window (Duration: 15 minutes)
│           │   ├── Check: Elevated Alert Triggered during Soak?
│           │   │   ├── YES ──> [EMERGENCY CUTOVER: Instantly switch selector back to Blue]
│           │   │   └── NO ──> Gracefully terminate Blue Pods after soak completes
│           └── State: GREEN_ACTIVE ──> [CONFIRM: Cutover Finalized]
│
└── 4. SHADOW / MIRROR DEPLOYMENT TIER [P2]
    └── Real-World: High-risk payment overhaul; duplicating 100% live traffic to shadow instance without returning responses
        └── State: TRAFFIC_MIRRORING_ACTIVE
            ├── Action: Deploy Shadow Pods with outbound external side-effects mocked/disabled
            ├── Action: Configure Mesh VirtualService `mirror: shadow-service, mirrorPercentage: 100`
            ├── Action: Ingest mirrored responses into asynchronous Diff Engine
            └── Action: Compare Baseline vs Shadow responses (Status Codes, Payloads, Latencies)
                └── Output Diff Report to Team before greenlighting live deployment
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Traffic Loss During Canary Abort
- Real-Life Incident: A canary rollout detected errors and aborted; the controller deleted the canary pods before updating the traffic mesh, causing 10% of users to hit dead endpoints (HTTP 503).
- Rationale: Traffic shifting must precede workload scaling. When aborting, traffic must be reset to 100% baseline *before* canary pods are terminated.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: SAFE CANARY ABORT (P0-01)
Trigger: Metric Failure / Abort Command
├── Action 1: Update Service Mesh / Ingress Weight to 100% Baseline (0% Canary)
├── Action 2: Wait for Mesh In-Flight Connection Drain (min 15 seconds)
├── Action 3: Verify Zero Live Connections hitting Canary Endpoints
└── Action 4: Scale down and delete Canary ReplicaSet
```

### Invariant P0-02: Pre-Cutover Smoke Verification for Blue/Green
- Real-Life Incident: An operator triggered a Blue/Green cutover before the database migration had completed on the Green stack; live traffic was immediately routed to an uninitialized database.
- Rationale: A Blue/Green cutover must be gated by a cryptographically signed smoke-test certificate verifying both infrastructure and database compatibility.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: SMOKE TEST GATE (P0-02)
Trigger: Blue/Green Cutover Requested
├── Action: Execute Automated Pre-Cutover Test Suite against Green Private Ingress
├── Check: All Health Checks, Readiness Probes, and Smoke Tests 100% Passed?
│   ├── YES ──> Permit Selector Mutation
│   └── NO ──> [HARD ABORT: Refuse to update Service selector; leave Blue live]
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Low-Traffic Service Makes Statistical Canary Metrics Meaningless
- Real-Life Incident: An internal admin service handling only 2 requests/minute had an error on 1 request, triggering a 50% error rate alarm and aborting a healthy deployment.
- Decision Tree:
```
EDGE CASE 01: LOW TRAFFIC CANARY ANALYSIS
Trigger: Canary Metric Analysis Window Initiated
├── Action: Calculate Total Request Volume in Window
├── Check: Total Requests < Minimum Statistical Threshold (e.g. 100 requests)?
│   ├── YES ──> [SWITCH TO SYNTHETIC PROBING: Inject synthetic traffic to validate health instead of relying on organic traffic]
│   └── NO ──> Standard Organic Prometheus Metric Evaluation
```

### Case 02: Incompatible Database Schema Change in Canary
- Real-Life Incident: A canary version dropped a database column; the baseline version (handling 90% of traffic) crashed immediately because both versions queried the shared database.
- Decision Tree:
```
EDGE CASE 02: DATABASE BREAKING CHANGE CONFLICT
Trigger: Deployment Pre-Flight Check
├── Action: Inspect Schema Migration Metadata
├── Check: Is Schema Migration marked as NON-EXPANDABLE (e.g. Drop Column, Rename Table)?
│   ├── YES ──> [HARD BLOCK: Canary/Blue-Green Prohibited without Expand/Contract pattern]
│   └── NO ──> Safe for Concurrent Multi-Version Execution
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
DEPLOYMENT STRATEGIES ROADMAP
│
├── PHASE 0: ROLLING UPDATE & BLUE/GREEN CORE (P0 - WEEKS 1-3)
│   ├── Kubernetes Deployment native rolling update tuning (maxSurge, maxUnavailable)
│   ├── Automated Blue/Green stack orchestrator with Service selector cutover
│   ├── Pre-cutover synthetic smoke testing runner
│   └── Deliverable: Zero-downtime Blue/Green deployments across all services
│
├── PHASE 1: ARGO ROLLOUTS & PROGRESSIVE CANARIES (P1 - WEEKS 4-6)
│   ├── Argo Rollouts CRD controller integration
│   ├── Automated Prometheus MetricTemplate analysis (Error rate, P99 latency)
│   ├── Automated multi-step traffic weighting (5% -> 20% -> 50% -> 100%)
│   └── Deliverable: Fully automated canary analysis with auto-rollback
│
├── PHASE 2: SERVICE MESH FINE-GRAINED TRAFFIC ROUTING (P2 - WEEKS 7-9)
│   ├── Istio / Linkerd VirtualService integration for header-based routing
│   ├── User-cohort canary routing (Internal employees test canary first)
│   └── Deliverable: Precision traffic routing based on JWT claims and user IDs
│
└── PHASE 3: SHADOW MIRRORING & A/B EXPERIMENTATION (P3 - WEEKS 10-12)
    ├── Dark traffic mirroring with automated response payload diffing
    ├── Statistical significance engine for business KPI rollouts
    └── Deliverable: Risk-free shadow testing for high-value transactional systems
```
