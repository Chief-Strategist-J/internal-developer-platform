# Autoscaling — Business Logic Decision Trees

## 1. Architectural Overview & Autoscaling Control Loop Decision Tree

```
AUTOSCALING ENGINE (HORIZONTAL, VERTICAL & CLUSTER SCALING DECISION TREE)
│
├── 1. SIGNAL INGESTION & METRIC EVALUATION TIER [P0]
│   └── Real-World: Traffic spikes 5x due to flash sale; HPA evaluates CPU, memory, and custom request rate
│       └── Trigger: Autoscaling Polling Interval (Every 15 seconds)
│           ├── Action: Ingest Resource Metrics (Metrics Server: CPU Utilization %, Memory Utilization %)
│           ├── Action: Ingest Custom / External Metrics (KEDA / Prometheus: Requests/sec, SQS Queue Depth)
│           ├── Calculation: Desired Replicas
│           │   └── `desired = ceil[ currentReplicas * ( currentMetricValue / targetMetricValue ) ]`
│           ├── Check: Does `desired != currentReplicas`?
│           │   ├── NO ──> State: STEADY (No scaling action needed)
│           │   └── YES ──> Action: Route to Directional Evaluator (Scale-Up vs Scale-Down)
│           │
├── 2. SCALE-UP EVALUATION & EXPEDITION TIER [P0]
│   └── Real-World: Desired replicas jumps from 10 to 45 to absorb sudden influx
│       └── Evaluation: Scale-Up Policy
│           ├── Check: Is `desired > maxReplicas` configured on HPA/KEDA?
│           │   ├── YES ──> [CLAMP: Set targetReplicas = maxReplicas + Emit Quota Alert]
│           │   └── NO ──> targetReplicas = desired
│           ├── Check: Scale-Up Stabilization Window (Default: 0s for instant burst handling)
│           │   └── Policy: Execute Fast-Scale (Double replicas or add +100% immediately)
│           ├── Action: Check Cluster Node Capacity
│           │   ├── Pods Fit on Existing Nodes ──> Mutate Deployment `spec.replicas = targetReplicas`
│           │   └── Pods Pending / Nodes Full ──> Trigger Cluster Autoscaler / Karpenter (Tier 4)
│           └── State: SCALED_UP
│
├── 3. SCALE-DOWN STABILIZATION & HYSTERESIS TIER [P0 / P1]
│   └── Real-World: Traffic drops after flash sale; preventing flapping/thrashing during temporary lull
│       └── Evaluation: Scale-Down Policy & Dampening
│           ├── Action: Apply Stabilization Window (Default: 300 seconds / 5 minutes)
│           ├── Check: Has Desired Replica count remained lower throughout entire 5-minute window?
│           │   ├── NO (Fluctuating) ──> [HOLD: Flapping detected — Retain current higher replica count]
│           │   └── YES ──> Calculate Maximum Permissible Downward Rate (e.g. max 10% per minute)
│           │       ├── Action: Graceful Scale Down (Clamp reduction to downward rate limit)
│           │       └── State: SCALED_DOWN
│           │
├── 4. CLUSTER & NODE-LEVEL PROVISIONING TIER (KARPENTER / CAS) [P0]
│   └── Real-World: 50 pods in Pending state waiting for compute instances
│       └── Trigger: Unscheduled Pods Detected (`PodCondition: Unschedulable`)
│           ├── Action: Analyze Pending Pod Resource Requests, NodeSelectors, and Taints
│           ├── Action: Select optimal Cloud Instance Type (e.g. `c6i.2xlarge` Spot vs On-Demand)
│           ├── Action: Provision Node via Cloud Provider API (Sub-60s Karpenter launch)
│           ├── Action: Wait for Node `Ready` -> Schedule Pending Pods
│           └── State: NODES_EXPANDED
│
└── 5. VERTICAL POD AUTOSCALING (VPA) & RIGHT-SIZING TIER [P1]
    └── Real-World: Developer configured 8 CPU cores for a pod that uses 150m; VPA recommends right-sizing
        └── Evaluation: Historical Resource Usage Trends (7-day window)
            ├── Action: Compute Recommendation (P95 CPU + 15% headroom, Peak Memory + 20% headroom)
            ├── Mode Check:
            │   ├── MODE == RECOMMENDATION_ONLY ──> Post suggestions to Developer Portal & Slack
            │   └── MODE == AUTO_IN_PLACE ──> Schedule Pod update during low-traffic maintenance window
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Hard Minimum Replicas for Production
- Real-Life Incident: A developer configured `minReplicas: 0` using KEDA; incoming web traffic suffered 45-second cold-start timeouts while waiting for new pods to boot.
- Rationale: User-facing synchronous services in Production must never scale to 0. A minimum replica floor (minReplicas >= 2 across distinct AZs) is mandatory.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MINIMUM PRODUCTION REPLICAS (P0-01)
Trigger: HPA / KEDA Spec Creation or Update
├── Check: Environment == PRODUCTION?
│   ├── YES ──> Check: `minReplicas >= 2`?
│   │   ├── NO ──> [MUTATE OR REJECT: Enforce `minReplicas = 2` — Zero-scale prohibited for Prod]
│   │   └── YES ──> Spec Valid
│   └── NO ──> Dev/Staging: Scale to 0 permitted for cost savings
```

### Invariant P0-02: Anti-Flapping Hysteresis Delay
- Real-Life Incident: Flapping traffic caused the autoscaler to scale pods from 5 to 50 and back to 5 every 3 minutes, consuming 100% of cluster CPU in container startup churn.
- Rationale: Scale-down must be dampened by a minimum 300-second stabilization window to prevent destructive flapping loops.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: HYSTERESIS STABILIZATION (P0-02)
Trigger: Scale-Down Intent Evaluated
├── Action: Scan desired replica calculation over past 300 seconds
├── Select: MAX(desired_replicas) in 300s window
├── Check: `currentReplicas > MAX(desired_replicas)`?
│   ├── YES ──> Safe to scale down to MAX
│   └── NO ──> Delay scale-down until window stabilizes
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Cloud Provider Spot Instance Interruption Notice
- Real-Life Incident: AWS reclaimed 20 Spot instances with 2-minute notice; cluster crashed because all replicas of a critical service were on Spot instances.
- Decision Tree:
```
EDGE CASE 01: SPOT TERMINATION NOTICE
Trigger: AWS / GCP Emits Spot Interruption Warning (2-Minute Warning)
├── Action: Taint and Cordon affected node immediately
├── Action: Karpenter provisions replacement On-Demand node with emergency priority
├── Action: Gracefully evict and reschedule pods before the 120-second termination deadline
└── Enforce: Production services must maintain at least 33% On-Demand baseline
```

### Case 02: Autoscaling Storm Exhausting VPC Subnet IPs
- Real-Life Incident: Rapid scaling launched 300 pods; the VPC subnet ran out of available private IP addresses, causing new pods to fail with `NetworkPlugin cni failed`.
- Decision Tree:
```
EDGE CASE 02: VPC IP EXHAUSTION
Trigger: Subnet Available IP Count < 10%
├── Action: Raise High-Priority Alert to Infrastructure Team
├── Action: Clamp HPA `maxReplicas` to available IP headroom
└── Action: Route overflow workloads to secondary cluster / VPC subnet
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
AUTOSCALING ROADMAP
│
├── PHASE 0: HPA CORE & RESOURCE METRIC TUNING (P0 - WEEKS 1-3)
│   ├── Metrics Server deployment and high-frequency Prometheus polling
│   ├── Default HPA templates with anti-flapping stabilization windows
│   ├── Production guardrails enforcing minimum 2 replicas
│   └── Deliverable: Stable, non-flapping horizontal autoscaling for CPU/Memory
│
├── PHASE 1: EVENT-DRIVEN AUTOSCALING WITH KEDA (P1 - WEEKS 4-6)
│   ├── KEDA integration for queue depth (Kafka, RabbitMQ, SQS) and HTTP traffic
│   ├── Custom ScaledObject abstractions for developer self-service
│   └── Deliverable: Rapid event-driven scale-outs triggered by real workload queues
│
├── PHASE 2: HIGH-SPEED NODE AUTOSCALING WITH KARPENTER (P2 - WEEKS 7-9)
│   ├── Karpenter deployment replacing legacy Cluster Autoscaler
│   ├── Sub-60s node provisioning with mixed Spot/On-Demand instance pools
│   └── Deliverable: Instantaneous compute capacity scaling with 40% cost reduction
│
└── PHASE 3: ML-POWERED PREDICTIVE AUTOSCALING (P3 - WEEKS 10-12)
    ├── Predictive time-series forecasting for recurring traffic patterns
    ├── Pre-warming replicas 15 minutes before scheduled traffic surges
    └── Deliverable: Zero latency spikes during predictable morning and flash-sale surges
```
