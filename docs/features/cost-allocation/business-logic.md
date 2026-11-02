# Cost Allocation — Business Logic Decision Trees

## 1. Architectural Overview & FinOps Cost Engine Decision Tree

```
COST ALLOCATION ENGINE (TAGGING, BILLING ATTRIBUTION & OPTIMIZATION DECISION TREE)
│
├── 1. BILLING INGESTION & CLOUD INVOICE PARSING TIER [P0]
│   └── Real-World: Ingesting daily AWS Cost and Usage Report (CUR) / GCP Billing BigQuery export
│       └── Trigger: Nightly Cloud Billing Export Ingestion Cron
│           ├── Action: Ingest Raw Line Items (ResourceID, UsageType, Cost, Tags, Region, TenantID)
│           ├── Action: Normalize Currency to Platform Standard USD ($)
│           ├── Action: Store in High-Performance Analytics Warehouse (ClickHouse / Snowflake)
│           └── State: RAW_BILLING_INGESTED
│           │
├── 2. TAGGING GOVERNANCE & ALLOCATION ENGINE TIER [P0]
│   └── Real-World: Attributing shared multi-tenant EKS cluster costs to individual microservices
│       └── Evaluation: Cost Allocation Strategy
│           ├── Check: Is Resource a Dedicated Cloud Asset (RDS, S3, Dedicated EC2)?
│           │   ├── YES
│           │   │   ├── Check: Are Mandatory Tags (`team`, `service`, `env`, `cost-center`) present?
│           │   │   │   ├── YES ──> [DIRECT ATTRIBUTION: Allocate 100% of line item to owning team]
│           │   │   │   └── NO ──> [UNALLOCATED POOL: Mark as Untagged Waste — Flag team manager]
│           │   └── NO (Shared Multi-Tenant Kubernetes Cluster)
│           │       ├── Action: Calculate Granular Pod Resource Allocation (via OpenCost / Kubecost)
│           │       │   ├── Metric 1: CPU Request * Node CPU Hourly Rate
│           │       │   ├── Metric 2: Memory Request * Node Memory Hourly Rate
│           │       │   ├── Metric 3: PersistentVolumeClaim (EBS) Storage GB-Hours
│           │       │   └── Metric 4: Egress Network Bandwidth GB
│           │       └── Merge: Allocate Cluster Sub-Costs proportionally to each Pod's Namespace & App
│           └── State: COSTS_ATTRIBUTED
│
├── 3. BUDGET ENFORCEMENT & BURN-RATE ANOMALY TIER [P1]
│   └── Real-World: A runaway worker in dev burns $5,000 in 6 hours due to a missing recursion base case
│       └── Evaluation: Anomaly Detection Engine (Evaluated hourly)
│           ├── Action: Compare Current Hourly Burn Rate against Historical 30-day Moving Average
│           ├── Check: Is Burn Rate > 3x Expected Baseline AND Cost Delta > $200?
│           │   ├── YES
│           │   │   ├── Action: Dispatch FinOps Emergency Alert to Team Slack Channel
│           │   │   └── Check: Does Team have Auto-Kill / Quota Enforcement enabled in Dev?
│           │   │       ├── YES ──> [AUTO-THROTTLE: Scale runaway deployment to 0 replicas]
│           │   │       └── NO ──> Alert Engineers for Manual Intervention
│           │   └── NO ──> Normal Spending Pattern
│           ├── Check: Has Team exceeded 100% of Monthly Budget?
│           │   ├── YES ──> [ALERT: Notify Engineering Director + Require Budget Increase Request]
│           │   └── NO ──> Spend within limits
│           │
└── 4. RIGHT-SIZING & WASTE IDENTIFICATION TIER [P1]
    └── Real-World: Engine detects 40 idle development databases and 200 overprovisioned pods
        └── Evaluation: Optimization Opportunity Scanner (Weekly Cron)
            ├── Opportunity 1: Idle Cloud Assets (CPU < 2% and Network In < 1 KB/s for 7 days)
            │   └── Action: Recommend automated termination or scheduling (Off-hours shutdown)
            ├── Opportunity 2: Overprovisioned Workloads (Allocated 8 GiB RAM, Peak Usage 500 MiB)
            │   └── Action: Generate PR with right-sized CPU/Memory requests (OpenCost Recommendation)
            ├── Opportunity 3: Unattached Storage Disks (Orphaned EBS volumes / unreferenced snapshots)
            │   └── Action: Schedule for deletion after 14-day grace period
            └── State: SAVINGS_REPORT_GENERATED (Publish potential monthly savings to Portal)
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Proportional Allocation of Shared Kubernetes Compute
- Real-Life Incident: A company divided the monthly $50,000 Kubernetes cluster bill equally among 10 teams; a batch ML team consuming 80% of cluster GPUs paid the same as an admin CRUD service.
- Rationale: Shared clusters must allocate costs dynamically using actual CPU/Memory requests and real-time usage (OpenCost algorithm), ensuring fair, transparent billing.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: KUBECOST PROPORTIONALITY (P0-01)
Trigger: Cluster Bill Allocation
├── Action: Calculate Total Cluster Cost `C_total`
├── For each Pod: Compute Resource Weight `W_pod = (CPU_req * C_cpu) + (RAM_req * C_ram)`
├── Allocate Pod Cost: `Cost_pod = C_total * (W_pod / sum(W_all_pods))`
└── Rollup: Aggregate Pod Costs by `label.team` and `label.service`
```

### Invariant P1-02: Mandatory Cost Center Tags at Resource Creation
- Real-Life Incident: An untagged S3 bucket containing raw video files cost $12,000/month; nobody knew who created it, and teams spent 3 months arguing over who paid for it.
- Rationale: Every cloud resource or Kubernetes namespace must have an immutable `cost-center` tag applied at creation time.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MANDATORY COST TAG (P1-02)
Trigger: IaC / Kubernetes Namespace Admission
├── Check: Is `cost-center` or `owner` tag present and non-empty?
│   ├── NO ──> [HARD REJECT: Admission Denied — Mandatory Cost Center Tag Missing]
│   └── YES ──> Tag Verified
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Spot Instance Price Fluctuations During Batch Job
- Real-Life Incident: AWS Spot market prices surged 8x in an availability zone; a long-running batch job ended up costing more than On-Demand instances.
- Decision Tree:
```
EDGE CASE 01: SPOT PRICE INVERSION
Trigger: Real-Time Spot Price Update
├── Check: Does Spot Hourly Rate > On-Demand Hourly Rate?
│   ├── YES ──> [ALERT & MIGRATE: Karpenter terminates Spot nodes and re-provisions on On-Demand baseline]
│   └── NO ──> Maintain Spot Execution
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
COST ALLOCATION ROADMAP
│
├── PHASE 0: BILLING INGESTION & MANDATORY TAGGING (P0 - WEEKS 1-3)
│   ├── AWS CUR / GCP Billing pipeline ingestion into ClickHouse
│   ├── Platform admission policies enforcing `cost-center` and `owner` tags
│   └── Deliverable: 100% tag compliance on new infrastructure with centralized billing
│
├── PHASE 1: KUBERNETES SHREDDING WITH OPENCOST (P1 - WEEKS 4-6)
│   ├── OpenCost daemon deployment across all Kubernetes clusters
│   ├── Pod and namespace level granular CPU/Memory/Egress cost attribution
│   └── Deliverable: Transparent per-microservice cost reporting in Developer Portal
│
├── PHASE 2: ANOMALY DETECTION & REAL-TIME ALERTING (P2 - WEEKS 7-9)
│   ├── Hourly burn-rate anomaly detection algorithms
│   ├── Slack notifications for sudden spend surges (> 3x baseline)
│   └── Deliverable: Elimination of end-of-month cloud bill shocks
│
└── PHASE 3: AUTOMATED RIGHT-SIZING & AUTONOMOUS SAVINGS (P3 - WEEKS 10-12)
    ├── Automated PR generation with VPA-guided right-sizing recommendations
    ├── Automated shutdown schedules for development environments on weekends
    └── Deliverable: Measured 30% reduction in total organizational cloud spend
```
