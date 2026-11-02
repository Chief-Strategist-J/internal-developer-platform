# Self-Service Provisioning — Business Logic Decision Trees

## 1. Architectural Overview & Provisioning Decision Tree

```
SELF-SERVICE PROVISIONING ENGINE (SERVICE & RESOURCE CREATION DECISION TREE)
│
├── 1. REQUEST INGESTION & TEMPLATE SELECTION TIER [P0]
│   └── Real-World: Developer selects 'Go Microservice with Redis & PostgreSQL' in Developer Portal
│       └── Trigger: Inbound Service Creation Request
│           ├── Check: Does Target Template ID exist and match active approved version?
│           │   ├── NO ──> [REJECT: HTTP 404 Not Found — Invalid or Deprecated Template]
│           │   └── YES ──> Check: Is Developer authenticated and member of Target Tenant & Team?
│           │       ├── NO ──> [REJECT: HTTP 403 Forbidden — Developer Not in Target Tenant/Team]
│           │       └── YES ──> Ingest Parameters & Manifest Schema
│           │
├── 2. PARAMETER VALIDATION & DRIFT PRE-FLIGHT TIER [P0]
│   └── Real-World: Developer submits service name `billing-engine` with memory request of `128Gi`
│       └── Evaluation: Parameter Constraints
│           ├── Check: Is Service Name RFC-1123 compliant (`[a-z0-9]([-a-z0-9]*[a-z0-9])?`)?
│           │   ├── NO ──> [REJECT: HTTP 422 Unprocessable Entity — Service Name Violates DNS RFC-1123]
│           │   └── YES ──> Check: Does Service Name collide with existing Catalog / DNS records?
│           │       ├── COLLISION ──> [REJECT: HTTP 409 Conflict — Service Name Already Claimed]
│           │       └── UNIQUE ──> Check: Resource Sizing against Template Allowed Ranges
│           │           ├── OUT OF BOUNDS ──> [REJECT: HTTP 422 Unprocessable Entity — CPU/Memory Outside Permitted Limits]
│           │           └── VALID ──> Parameter Payload Validated
│           │
├── 3. QUOTA, BUDGET & COMPLIANCE GUARDRAIL TIER [P0 / P1]
│   └── Real-World: Team requests provision of 3 new clusters; project exceeds monthly cloud budget
│       └── Evaluation: Tenant Quota & Cost Forecasting
│           ├── Check: Does Target Project have available CPU, Memory, and IP quota?
│           │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Tenant Quota Exhausted; Request Quota Increase]
│           │   └── YES ──> Action: Query Cost Engine for Projected Monthly Spend
│           │       ├── Check: Projected Spend > Team Budget Cap?
│           │       │   ├── YES ──> Action: Route to FinOps Approval Workflow (State: PENDING_FINOPS_APPROVAL)
│           │       │   └── NO ──> Proceed to Automated Provisioning
│           │
├── 4. ORCHESTRATION & COMPONENT PROVISIONING DAG TIER [P0]
│   └── Real-World: Engine creates GitHub repo, branches, CI/CD pipelines, DNS records, and ArgoCD app
│       └── State: PROVISIONING_DAG_RUNNING (Topological Step Execution)
│           ├── Step 1: Git Repository Scaffolding
│           │   ├── Action: Create Repo from Template + Push Initial Skeleton + Branch Protection Rules
│           │   └── Failure ──> [HALT: Trigger Compensation — Delete Repo and Abort]
│           ├── Step 2: CI/CD Pipeline Provisioning
│           │   ├── Action: Register OIDC Workload Identity with CI Runner + Inject Build Secrets
│           │   └── Failure ──> [COMPENSATE: Archive Repo and Abort]
│           ├── Step 3: Cloud Infrastructure Allocation (Terraform / Crossplane)
│           │   ├── Action: Emit Infrastructure Claim (RDS DB, S3 Bucket, IAM Role)
│           │   └── Failure ──> [COMPENSATE: Teardown Cloud Resources, Archive Repo]
│           └── Step 4: GitOps Application Registration
│               ├── Action: Commit ArgoCD / Flux Application CRD to Fleet Cluster
│               └── Failure ──> [COMPENSATE: Remove GitOps CRD, Teardown Cloud Infra]
│           └── State: ALL_STEPS_SUCCEEDED
│
└── 5. CATALOG REGISTRATION & NOTIFICATION TIER [P0]
    └── Real-World: Service is live; owner notified via Slack; service registered in Backstage catalog
        └── Output Actions
            ├── Action: Commit `catalog-info.yaml` to central Service Catalog index
            ├── Action: Dispatch Slack / Teams notification with repo link, dashboard URL, and starter guide
            └── [RETURN: HTTP 201 Created — Service Successfully Materialized]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: All Provisioning Side-Effects Must Be Compensatable (Saga Guarantee)
- Real-Life Incident: A provisioning script failed halfway through creating an AWS Aurora cluster; the Git repo was deleted, but the orphaned DB kept running undetected for 6 months, racking up thousands of dollars.
- Rationale: Provisioning spans heterogeneous external systems (GitHub, AWS, DNS, ArgoCD). The workflow must be an idempotent Saga state machine where any step failure triggers guaranteed backward compensation.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: COMPENSATABLE SAGA (P0-01)
Trigger: Provisioning Step Failure
├── Action: Halt Forward Execution immediately
├── Action: Read executed step log in reverse chronological order
├── For each executed step:
│   ├── Execute Compensation Action (e.g. Delete S3, Teardown RDS, Delete GitHub Repo)
│   ├── Check: Did Compensation Succeed?
│   │   ├── YES ──> Mark Step Compensated
│   │   └── NO ──> [ALERT: Mark ORPHANED_DIRTY — Page Platform On-Call with Resource IDs]
└── State: SAGA_COMPENSATED (Zero Lingering Side-Effects)
```

### Invariant P0-02: Tenant Quota Hard Isolation
- Real-Life Incident: A developer ran an automated load-testing generator script that provisioned 500 microservice sandboxes, consuming all available IP addresses in the shared VPC and bringing down production pod launches.
- Rationale: Self-service must have strict tenant and team resource ceiling caps enforced atomically before any external API is called.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ATOMIC QUOTA CHECK (P0-02)
Trigger: Provisioning Pre-Flight
├── BEGIN TRANSACTION
│   ├── Lock: `SELECT current_usage, quota_max FROM tenant_quotas WHERE tenant_id = :id FOR UPDATE`
│   ├── Check: `current_usage + requested_usage <= quota_max`?
│   │   ├── NO ──> [ROLLBACK & REJECT: Quota Exceeded]
│   │   └── YES ──> `UPDATE tenant_quotas SET current_usage = current_usage + requested_usage`
│   └── COMMIT
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: External Cloud API Rate Limiting Mid-Provisioning
- Real-Life Incident: 20 teams provisioned services concurrently after a hackathon kick-off; GitHub API rate-limited repo creation requests, leaving 15 services half-initialized.
- Decision Tree:
```
EDGE CASE 01: EXTERNAL API RATE-LIMITING
Trigger: Downstream Cloud/GitHub API returns HTTP 429
├── Check: Does Response contain `Retry-After` header?
│   ├── YES ──> Suspend Provisioning Step Lease until timestamp + random jitter (1-5s)
│   └── NO ──> Exponential backoff with Decorrelated Jitter (max 3 retries)
├── Check: Did Retries Succeed within Step Deadline (5 minutes)?
│   ├── YES ──> Resume Forward Provisioning DAG
│   └── NO ──> [GRACEFUL ABORT: Trigger Saga Compensation and alert user with retry token]
```

### Case 02: Duplicate Provisioning Intent (Double-Click Submission)
- Real-Life Incident: A developer double-clicked the "Create Service" button on a laggy connection, generating two separate GitHub repos and two competing ArgoCD apps for the same service.
- Decision Tree:
```
EDGE CASE 02: DUPLICATE PROVISIONING INTENT
Trigger: Inbound Create Service POST Request
├── Extract: `Idempotency-Key` or Hash(tenant_id + service_name)
├── Action: Redis Distributed Lock `SET lock:provision:<hash> NX EX 600`
├── Check: Was Lock Acquired?
│   ├── NO ──> [REJECT: HTTP 409 Conflict — Provisioning for this service is already in progress]
│   └── YES ──> Proceed to Atomic Reservation & Saga Execution
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
SELF-SERVICE PROVISIONING ROADMAP
│
├── PHASE 0: CORE TEMPLATE ENGINE & SAGA RUNNER (P0 - WEEKS 1-3)
│   ├── RFC-1123 validation, deduplication, and atomic tenant quota reservations
│   ├── Idempotent Saga orchestrator with automatic reverse compensation
│   ├── GitHub Repo & Branch Protection automation
│   └── Deliverable: Bulletproof end-to-end repository & CI provisioning with zero orphaned assets
│
├── PHASE 1: CLOUD INFRASTRUCTURE & GITOPS WIRING (P1 - WEEKS 4-6)
│   ├── Crossplane / Terraform Cloud provider integration (Databases, S3, IAM)
│   ├── ArgoCD / Flux Application CRD automated generation
│   ├── Integrated Backstage `catalog-info.yaml` publisher
│   └── Deliverable: Full-stack automated service generation in under 60 seconds
│
├── PHASE 2: FINOPS GUARDS & APPROVAL WORKFLOWS (P2 - WEEKS 7-9)
│   ├── Cost estimation API integration with automated budget threshold triggers
│   ├── Multi-tier approval workflows for non-standard sizing
│   └── Deliverable: Cost-governed self-service preventing unexpected cloud bills
│
└── PHASE 3: DRIFT RECONCILIATION & EPHEMERAL ENVIRONMENTS (P3 - WEEKS 10-12)
    ├── Ephemeral environment time-to-live (TTL) auto-destruction daemon
    ├── Template version upgrade bot (Automated PRs for template drifts)
    └── Deliverable: Fully autonomous service lifecycle management from creation to retirement
```
