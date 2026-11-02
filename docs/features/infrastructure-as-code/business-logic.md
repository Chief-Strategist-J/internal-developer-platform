# Infrastructure as Code — Business Logic Decision Trees

## 1. Architectural Overview & IaC Lifecycle Decision Tree

```
INFRASTRUCTURE AS CODE (IAC) ENGINE (EXECUTION & GOVERNANCE DECISION TREE)
│
├── 1. WORKLOAD CLAIM INGESTION & PLAN TRIGGER TIER [P0]
│   └── Real-World: Developer submits Terraform module or Crossplane Resource Claim for AWS S3 Bucket
│       └── Trigger: Inbound IaC Change / Git Pull Request
│           ├── Check: Is IaC Manifest valid according to Module Schema v1?
│           │   ├── NO ──> [REJECT: HTTP 422 Unprocessable Entity — Module Schema Validation Failed]
│           │   └── YES ──> Check: Is Target Cloud Account & Provider Registered and Healthy?
│           │       ├── NO ──> [REJECT: HTTP 503 Service Unavailable — Cloud Account Credentials Invalid]
│           │       └── YES ──> Initialize Execution Sandbox
│           │
├── 2. PRE-FLIGHT STATIC ANALYSIS & SECURITY SCANNING TIER [P0]
│   └── Real-World: Scanning Terraform code for unencrypted buckets, open security groups (0.0.0.0/0), or public IPs
│       └── Evaluation: Shift-Left Security Scanners (Tfsec / Checkov / Trivy)
│           ├── Scan Rule 1: Public Ingress Rules (`0.0.0.0/0` on sensitive ports 22, 3306, 5432)
│           │   ├── MATCH ──> [HARD BLOCK: CRITICAL Security Violation — Public database/SSH port prohibited]
│           │   └── PASS ──> Continue
│           ├── Scan Rule 2: Unencrypted Storage at Rest (`encrypted = false`)
│           │   ├── MATCH ──> [HARD BLOCK: Encryption at Rest is mandatory for all storage resources]
│           │   └── PASS ──> Continue
│           ├── Scan Rule 3: Mandatory Cloud Resource Tags (`CostCenter`, `Owner`, `Environment`)
│           │   ├── MISSING ──> [BLOCK: Missing Required Governance Tags]
│           │   └── PASS ──> Pre-Flight Scans Passed
│           │
├── 3. SPECULATIVE PLAN & COST ESTIMATION TIER [P0 / P1]
│   └── Real-World: Executing `terraform plan` and computing monthly cost differential via Infracost
│       └── State: SPECULATIVE_PLAN_RUNNING
│           ├── Action: Run Speculative Execution against Cloud Provider API
│           ├── Action: Parse Resource Delta (To Create: N, To Update: N, To Destroy: N)
│           ├── Check: Does Plan include Destructive Actions (Resources to DESTROY > 0)?
│           │   ├── YES
│           │   │   ├── Check: Are any destroyed resources marked with `prevent_destroy` lifecycle flag?
│           │   │   │   ├── YES ──> [HARD REJECT: Plan attempts to destroy protected database/storage resource]
│           │   │   │   └── NO ──> Flag Plan as DESTRUCTIVE (Requires Senior Architect Approval)
│           │   └── NO ──> Additive change only
│           ├── Action: Calculate Cost Delta (via Infracost)
│           │   └── Check: Monthly Cost Increase > $500?
│           │       ├── YES ──> Require FinOps Sign-Off
│           │       └── NO ──> Cost Approved
│           └── State: PLAN_GENERATED (Post Plan & Cost Diff to PR Comments)
│
├── 4. STATE LOCKING & CONCURRENCY CONTROL TIER [P0]
│   └── Real-World: Two developers push infrastructure changes to the same VPC concurrently
│       └── Evaluation: Distributed State Lock (Postgres / DynamoDB State Lock)
│           ├── Action: Attempt to acquire State Lock `acquire_lock(workspace_id)`
│           ├── Check: Was State Lock successfully acquired?
│           │   ├── NO ──> [REJECT: HTTP 409 Conflict — Workspace Locked by Active Run ID]
│           │   └── YES ──> State Lock Leased (TTL: 30 minutes with heartbeat)
│           │
└── 5. APPLY EXECUTION & STATE COMMIT TIER [P0]
    └── Real-World: Applying approved Terraform plan to cloud environment
        └── State: APPLY_IN_PROGRESS
            ├── Action: Stream logs in real-time to Developer Portal and Audit Log
            ├── Check: Did Cloud Provider return Error during resource creation?
            │   ├── YES
            │   │   ├── Action: Release State Lock cleanly
            │   │   ├── Action: Save partial state file to prevent state corruption
            │   │   └── State: APPLY_FAILED ──> [ALERT: Page Team with Cloud Provider API Error]
            │   └── NO (All Resources Created)
            │       ├── Action: Persist new State File version with cryptographic hash verification
            │       ├── Action: Release State Lock
            │       └── State: APPLY_SUCCEEDED ──> [CONFIRM: Cloud Infrastructure Live]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Unverified State File Overwrites
- Real-Life Incident: A network hiccup during a state upload caused an empty 0-byte state file to overwrite the S3 bucket, causing Terraform to think all 400 production cloud resources had been deleted.
- Rationale: State files contain the mapping between code and real-world assets. Overwrites must use optimistic concurrency control with serial numbers, cryptographic checksums, and versioned S3 buckets.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: SAFE STATE COMMIT (P0-01)
Trigger: State File Write
├── Check: New State Serial > Current State Serial?
│   ├── NO ──> [REJECT: State Serial Stale — Concurrent Modification Detected]
│   └── YES ──> Check: Is New State Size > 0 bytes and JSON Valid?
│       ├── NO ──> [HALT: Corrupted or Empty State Payload Blocked]
│       └── YES ──> Write to S3 with Versioning + Object Lock
```

### Invariant P0-02: Prevent Accidental Data-Store Deletion
- Real-Life Incident: An engineer renamed an RDS database module block in Terraform; Terraform planned to destroy the production database and create a blank new one.
- Rationale: Data-bearing resources (RDS, DynamoDB, S3, EBS) must have hard lifecycle blocks that fail any plan attempting to destroy them without explicit out-of-band unlock.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: DESTRUCTION GUARD (P0-02)
Trigger: Plan Evaluation
├── Scan Plan: Any `Destroy` action on `aws_db_instance`, `aws_s3_bucket`, or `aws_ebs_volume`?
│   ├── YES ──> Check: Is Workspace explicitly configured in `ALLOW_DATA_DESTRUCTION` mode?
│   │   ├── NO ──> [HARD ABORT: Plan Rejected — Production Data Store Destruction Blocked]
│   │   └── YES ──> Require 2 Director Approvals
│   └── NO ──> Safe to proceed
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Orphaned State Lock after Runner Pod Crash
- Real-Life Incident: An AWS spot instance running an `apply` was terminated by AWS; the DynamoDB state lock remained held, blocking all further deployments to that workspace.
- Decision Tree:
```
EDGE CASE 01: ORPHANED STATE LOCK
Trigger: Workspace Locked for > 30 minutes without active heartbeat
├── Action: Probe Runner Process Health
├── Check: Is Runner Pod Dead or Non-Existent?
│   ├── YES
│   │   ├── Action: Verify last state commit status
│   │   └── Action: Force-release lock with audit warning `STATE_LOCK_AUTO_RELEASED_DEAD_RUNNER`
│   └── NO ──> Runner active; maintain lock
```

### Case 02: Cloud Resource Import Collision
- Real-Life Incident: A developer manually created an S3 bucket in the AWS console; an IaC apply later attempted to create a bucket with the same name, failing with `BucketAlreadyExists`.
- Decision Tree:
```
EDGE CASE 02: CLOUD RESOURCE COLLISION
Trigger: Cloud Provider API returns ResourceAlreadyExists
├── Check: Does Resource exist in cloud account matching tenant credentials?
│   ├── YES ──> [HALT: Unmanaged Resource Detected — Prompt Developer to Import into IaC or Rename]
│   └── NO ──> Resource belongs to another external account — [REJECT: Name collision]
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
INFRASTRUCTURE AS CODE ROADMAP
│
├── PHASE 0: SECURE RUNNER & STATE LOCKING (P0 - WEEKS 1-3)
│   ├── Isolated ephemeral runner sandboxes with short-lived cloud credentials
│   ├── Robust PostgreSQL / DynamoDB state locking with heartbeat leases
│   ├── Shift-left security linter integration (Tfsec / Trivy)
│   └── Deliverable: Deterministic, safe Terraform / OpenTofu execution pipeline
│
├── PHASE 1: SPECULATIVE PLANS & COST FORECASTING (P1 - WEEKS 4-6)
│   ├── Pull request comment bot posting plans and Infracost diffs
│   ├── Hard destruction prevention guards for databases and object storage
│   └── Deliverable: Transparent infrastructure review process with FinOps governance
│
├── PHASE 2: CROSSPLANE & KUBERNETES-NATIVE CLAIMS (P2 - WEEKS 7-9)
│   ├── Crossplane composition engine for self-service cloud claims
│   ├── GitOps reconciliation of cloud resources alongside Kubernetes workloads
│   └── Deliverable: Kubernetes-native declarative infrastructure for developers
│
└── PHASE 3: DRIFT RECONCILIATION & AUTO-IMPORT (P3 - WEEKS 10-12)
    ├── Continuous background drift detection against live cloud APIs
    ├── Automated PR generation to reconcile out-of-band console changes
    └── Deliverable: 100% cloud estate synchronized with code
```
