# Environment Management — Business Logic Decision Trees

## 1. Architectural Overview & Environment Lifecycle Decision Tree

```
ENVIRONMENT MANAGEMENT ENGINE (LIFECYCLE, PROMOTION & TEARDOWN DECISION TREE)
│
├── 1. ENVIRONMENT CREATION & INGESTION TIER [P0]
│   └── Real-World: Developer opens Pull Request #142; system creates preview environment `pr-142-billing`
│       └── Trigger: Inbound Environment Create Intent (Static Dev/Staging/Prod OR Ephemeral PR Preview)
│           ├── Check: Is Environment Type STATIC or EPHEMERAL?
│           │   ├── STATIC (e.g. `production`, `staging`)
│           │   │   ├── Check: Does Requester have `platform.environment.admin` permission?
│           │   │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Only Platform Admins create static envs]
│           │   │   │   └── YES ──> Action: Create Persistent Environment Context
│           │   └── EPHEMERAL (e.g. `pr-142`, `preview-feat-auth`)
│           │       ├── Check: Has Team / Tenant exceeded Maximum Active Ephemeral Environments (max 5)?
│           │       │   ├── YES ──> [REJECT: HTTP 422 Unprocessable Entity — Ephemeral Quota Exceeded; Teardown old envs]
│           │       │   └── NO ──> Action: Assign Time-To-Live (TTL default: 72 hours)
│           │           └── State: ENVIRONMENT_PROVISIONING
│           │
├── 2. INFRASTRUCTURE & NAMESPACE BOUNDING TIER [P0]
│   └── Real-World: Provisioning dedicated K8s namespace, network boundaries, and ephemeral database clone
│       └── Evaluation: Environment Isolation Level
│           ├── Action: Allocate Target Kubernetes Namespace (`env-<tenant>-<env_id>`)
│           ├── Action: Apply NetworkPolicy (Strict Ingress/Egress Isolation from other environments)
│           ├── Action: Provision Data Tier Strategy
│           │   ├── STATIC ENV ──> Connect to Dedicated Persistent Cloud Database
│           │   └── EPHEMERAL ENV ──> Provision Ephemeral Database Clone (Copy-on-Write storage snapshot)
│           └── State: ENVIRONMENT_READY
│
├── 3. PROMOTION PIPELINE & STAGE GATING TIER [P0 / P1]
│   └── Real-World: Promoting release `v2.4.0` from Staging to Production
│       └── Evaluation: Promotion Gates
│           ├── Check: Has artifact `v2.4.0` successfully passed automated integration tests in Staging?
│           │   ├── NO ──> [REJECT: HTTP 422 Unprocessable Entity — Artifact Unverified in Staging]
│           │   └── YES ──> Check: Are all Required Approvals present (e.g. 1 Tech Lead + 1 QA Lead)?
│           │       ├── NO ──> State: PENDING_APPROVAL (Emit approval notification to reviewers)
│           │       └── YES ──> Check: Is Target Environment in Change Freeze Window?
│           │           ├── FREEZE ──> [REJECT: HTTP 403 Forbidden — Production Change Freeze Active]
│           │           └── OPEN ──> Proceed to Rollout Orchestration
│           │
├── 4. CONFIGURATION & SECRET OVERLAY TIER [P0]
│   └── Real-World: Injecting environment-specific database URLs and API keys without rebuilding container image
│       └── Evaluation: Environment Configuration Merging
│           ├── Action: Ingest Base Configuration (shared across all environments)
│           ├── Action: Apply Environment Overlays (`dev.env`, `staging.env`, `prod.env`)
│           ├── Action: Ingest Secrets from Environment-Scoped Vault Path (`/secrets/tenants/<id>/<env>`)
│           └── State: CONFIG_ASSEMBLED
│
└── 5. EPHEMERAL TEARDOWN & REAPER TIER [P0]
    └── Real-World: PR #142 is merged or closed; ephemeral preview environment must be instantly destroyed
        └── Trigger: PR Close Webhook OR Expiration of 72h TTL Reaper Timer
            ├── Check: Is Environment Ephemeral?
            │   ├── NO ──> [PROTECTED: Static Environments cannot be auto-reaped]
            │   └── YES
            │       ├── Action: Delete GitOps Application and Kubernetes Namespace cleanly
            │       ├── Action: Delete Ephemeral Database Clones and Cloud Storage Buckets
            │       ├── Action: Deregister Temporary DNS record (`pr-142.preview.company.internal`)
            │       ├── Action: Release Ephemeral Quota back to Tenant pool
            │       └── [CONFIRM: Ephemeral Environment Completely Annihilated; Emit Audit Record]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Cross-Environment Network Isolation
- Real-Life Incident: A developer running a load test in a staging environment accidentally pointed a connection string at the production database cluster, corrupting production transaction logs.
- Rationale: Staging and Development environments must never have network routes or credentials capable of accessing Production data planes.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: NETWORK ISOLATION (P0-01)
Trigger: Environment Creation / NetworkPolicy Generation
├── Check: Target Environment == DEV or STAGING?
│   ├── YES ──> Inject Egress Firewall / Cilium NetworkPolicy:
│   │   ├── DENY Egress to Production CIDR Blocks (`10.200.0.0/16`)
│   │   └── DENY Egress to Production Database Endpoints
│   └── NO ──> Production Egress strictly whitelisted
```

### Invariant P0-02: Ephemeral Environments Must Have Enforced Hard TTL
- Real-Life Incident: A team created 100 preview environments over a year without deleting them; abandoned cloud databases cost the company $40,000 monthly.
- Rationale: Ephemeral environments must have an unmodifiable maximum time-to-live (max 7 days) and be automatically reaped by a background daemon.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: HARD EPHEMERAL TTL (P0-02)
Trigger: Ephemeral Environment Creation
├── Check: Requested TTL <= 168 hours (7 days)?
│   ├── YES ──> Set `reap_at = NOW() + requested_ttl`
│   └── NO ──> [CLAMP: Force `reap_at = NOW() + 168 hours`]
├── Background Reaper Cron: Every 10 minutes
│   └── `SELECT * FROM environments WHERE type='EPHEMERAL' AND reap_at <= NOW()`
│       └── Trigger Clean Teardown Workflow
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: PR Re-Opened After Ephemeral Environment Teardown
- Real-Life Incident: Developer closed a PR by mistake and immediately re-opened it; the teardown job and recreation job ran concurrently, deadlocking namespace creation.
- Decision Tree:
```
EDGE CASE 01: PR REOPEN RACE CONDITION
Trigger: PR Reopened Webhook while Teardown Job Active
├── Action: Distributed Lock `lock:env:<pr_id>`
├── Check: Is Teardown Worker currently deleting resources?
│   ├── YES ──> [QUEUE: Wait for Teardown to complete cleanly before triggering recreation]
│   └── NO ──> Provision fresh environment
```

### Case 02: Promotion Blocked by Unhealthy Staging Environment
- Real-Life Incident: Staging cluster had a broken third-party mock service causing tests to fail, blocking a critical security patch from being promoted to Production.
- Decision Tree:
```
EDGE CASE 02: EMERGENCY PROMOTION BYPASS
Trigger: Emergency Production Deploy with Failed Staging Pre-Checks
├── Check: Does Promotion Request contain Security Incident Hotfix Flag?
│   ├── YES
│   │   ├── Check: Approved by 2 Authorized Platform Directors via JIT Break-Glass?
│   │   │   ├── YES ──> [ALLOW WITH AUDIT: Bypass staging verification, deploy hotfix, log to SIEM]
│   │   │   └── NO ──> [REJECT: Dual approval required for bypass]
│   └── NO ──> [REJECT: Must pass staging verification]
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
ENVIRONMENT MANAGEMENT ROADMAP
│
├── PHASE 0: STATIC ENVIRONMENT GOVERNANCE & CONFIG OVERLAYS (P0 - WEEKS 1-3)
│   ├── Core schema for Environments, Stages, and Promotion Paths
│   ├── Dynamic Kustomize overlay generation per environment
│   ├── Strict network isolation NetworkPolicies between staging and production
│   └── Deliverable: Robust governance and promotion workflows for static environments
│
├── PHASE 1: EPHEMERAL PREVIEW ENVIRONMENTS (P1 - WEEKS 4-6)
│   ├── Pull-request triggered preview environment automation
│   ├── Automated temporary wildcard DNS & TLS certificate issuance
│   ├── Background reaper daemon for expired TTL cleanup
│   └── Deliverable: Automated preview environments for developers on every PR
│
├── PHASE 2: COPY-ON-WRITE DATABASE CLONING (P2 - WEEKS 7-9)
│   ├── Integration with cloud storage snapshots for instant database cloning
│   ├── Automated data anonymization and PII sanitization for preview environments
│   └── Deliverable: Realistic staging and preview testing with zero production data leaks
│
└── PHASE 3: PROGRESSIVE ENVIRONMENT MESH & TRAFFIC SPLITTING (P3 - WEEKS 10-12)
    ├── Virtual service environments using header-based traffic routing (Istio VirtualService)
    ├── Dynamic multi-tenant sandbox sharing
    └── Deliverable: 90% cloud cost reduction by replacing isolated clusters with virtual mesh envs
```
