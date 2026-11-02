# GitOps Reconciliation — Business Logic Decision Trees

## 1. Architectural Overview & Reconciliation Loop Decision Tree

```
GITOPS RECONCILIATION ENGINE (DESIRED STATE SYNC & RECONCILIATION DECISION TREE)
│
├── 1. REPOSITORY WATCH & WEBHOOK TRIGGER TIER [P0]
│   └── Real-World: Developer merges pull request to `main` branch in Git repository
│       └── Trigger: Inbound Git Webhook Event / Polling Timer (ArgoCD / Flux)
│           ├── Check: Is Webhook Payload HMAC Signature Valid?
│           │   ├── NO ──> [REJECT: HTTP 401 Unauthorized — Webhook Signature Tampered]
│           │   └── YES ──> Check: Did Commit change manifests for monitored environment path?
│           │       ├── NO ──> [IGNORE: Commit unrelated to target path; ignore]
│           │       └── YES ──> Action: Ingest Target Commit SHA (e.g. `a1b2c3d`)
│           │           └── State: RECONCILIATION_QUEUED
│           │
├── 2. REPOSITORY CLONE & MANIFEST GENERATION TIER [P0]
│   └── Real-World: Engine renders Kustomize overlays and Helm charts for target production cluster
│       └── Evaluation: Manifest Compilation
│           ├── Action: Clone Git Repo at exact Commit SHA in isolated sandbox container
│           ├── Action: Render manifests (`kustomize build` / `helm template`)
│           ├── Check: Did Template Rendering succeed without syntax/schema errors?
│           │   ├── NO ──> [HALT: Compilation Failed — Post Commit Status `failure` to GitHub PR/Commit]
│           │   └── YES ──> Extract Desired State Manifest Set (Objects: CRDs, Deployments, Services)
│           │
├── 3. DRIFT DETECTION & DIFF EVALUATION TIER [P0 / P1]
│   └── Real-World: Cluster operator manually scaled replicas to 10 via kubectl; Git specifies 3
│       └── Evaluation: Compare Desired State (Git) vs Live State (Kubernetes Cluster)
│           ├── Action: Fetch Live Kubernetes State for all resources matching app selector
│           ├── Action: Perform Three-Way Merge Diff (Last Applied vs Desired vs Live)
│           ├── Check: Is Drift Detected?
│           │   ├── NO ──> State: SYNCED (No changes needed) ──> [TERMINAL: Cluster matches Git]
│           │   └── YES ──> Identify Drift Types:
│           │       ├── DRIFT TYPE A: Modified Fields (e.g. Image tag changed, CPU limits changed)
│           │       ├── DRIFT TYPE B: Missing Resources (New resources committed to Git)
│           │       └── DRIFT TYPE C: Orphaned Live Resources (Resources removed from Git but alive on cluster)
│           │
├── 4. SYNC STRATEGY & ADMISSION GATING TIER [P0]
│   └── Real-World: Automated deployment window check before syncing changes to production
│       └── Evaluation: Sync Permissions & Safety
│           ├── Check: Is Auto-Sync enabled for this Environment?
│           │   ├── NO ──> State: OUT_OF_SYNC (Wait for Manual Developer Trigger via Portal)
│           │   └── YES ──> Check: Is Sync Window Open (Not during weekend / freeze window)?
│           │       ├── BLOCKED ──> State: SYNC_BLOCKED (Queue until sync window opens)
│           │       └── OPEN ──> Proceed to Apply Phase
│           │
└── 5. APPLIED EXECUTION & HEALTH RECONCILIATION TIER [P0]
    └── Real-World: Applying manifests in topological wave order (CRD -> Namespace -> ConfigMap -> Deployment)
        └── State: SYNC_IN_PROGRESS (Executing Sync Waves: Wave 0 to Wave 5)
            ├── Action: Apply Resources via Server-Side Apply (SSA) with Field-Manager `gitops-engine`
            ├── Check: Server-Side Apply Conflict Encountered?
            │   ├── YES ──> [RESOLVE: Force SSA ownership if Git is authoritative; else HALT]
            │   └── NO ──> Objects Applied
            ├── Action: Prune Orphaned Live Resources (if `prune: true` configured)
            │   └── Check: Is Resource marked with `prevent-deletion` annotation?
            │       ├── YES ──> Skip Deletion and Alert Operator
            │       └── NO ──> Prune Resource cleanly
            └── Health Check: Monitor Workload Health status
                ├── STATUS == HEALTHY ──> State: SYNC_SUCCESSFUL ──> [POST: GitHub Status `success`]
                └── STATUS == DEGRADED / DEADLINE EXCEEDED
                    ├── Action: Trigger Auto-Rollback to Previous Git Commit SHA (if configured)
                    └── State: SYNC_FAILED ──> [ALERT: Page Team via PagerDuty]
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Git is the Single Authoritative Source of Truth
- Real-Life Incident: An operator debugged a live cluster issue by manually editing a ConfigMap with `kubectl edit`; an automated GitOps sync 5 minutes later overwrote the change, resurrecting the outage.
- Rationale: Direct cluster mutations without Git commits are ephemeral and will be wiped. The GitOps engine must continuously reconcile live state back to Git desired state to prevent configuration drift.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: GITOPS SOVEREIGNTY (P0-01)
Trigger: Live State Drift Detected
├── Check: Is Auto-Heal enabled on Application?
│   ├── YES ──> Action: Overwrite Live State with Git Desired State immediately
│   └── NO ──> Action: Mark Application OUT_OF_SYNC and emit notification to owner
```

### Invariant P0-02: Deterministic Immutable Commit Pinning
- Real-Life Incident: A GitOps controller configured to track branch `main` cloned during a force-push rebase, compiling an invalid half-synced merge commit.
- Rationale: Sync executions must bind strictly to an immutable Git Commit SHA, never to mutable floating branch heads.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: IMMUTABLE COMMIT PIN (P0-02)
Trigger: Sync Execution Triggered
├── Action: Resolve Branch Head to Exact 40-character Git SHA (e.g. `e5a6f7b8...`)
├── Action: Checkout exact SHA
├── Enforce: Sync, Audit Log, and Health status bind exclusively to this SHA
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Server-Side Apply (SSA) Field Manager Conflict
- Real-Life Incident: Horizontal Pod Autoscaler (HPA) adjusted `spec.replicas`, while GitOps engine kept fighting HPA by resetting `spec.replicas` to 3 every 30 seconds.
- Decision Tree:
```
EDGE CASE 01: HPA VS GITOPS REPLICA FIGHT
Trigger: Field Management Conflict on `spec.replicas`
├── Check: Does Target Deployment have an associated HorizontalPodAutoscaler (HPA) resource?
│   ├── YES
│   │   ├── Action: Configure GitOps IgnoreDifferences for `spec.replicas`
│   │   └── State: Replicas delegated exclusively to HPA; GitOps reconciles remaining fields
│   └── NO ──> Git remains authoritative owner of `spec.replicas`
```

### Case 02: Infinite Sync Loop on Dynamic Status/Mutation Annotations
- Real-Life Incident: An admission controller modified an annotation with a dynamic timestamp on every write, causing GitOps engine to see perpetual drift and loop sync 10 times a second.
- Decision Tree:
```
EDGE CASE 02: DYNAMIC ANNOTATION LOOP
Trigger: Sync Succeeded but Resource immediately re-enters Drift state
├── Check: Did Drift originate from metadata annotations or status fields?
│   ├── YES ──> Action: Rate-limit sync retries (Exponential backoff: 5s, 15s, 60s, 300s)
│   └── NO ──> Investigate resource spec mutation
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
GITOPS RECONCILIATION ROADMAP
│
├── PHASE 0: ARGO CD / FLUX ENGINE CORE (P0 - WEEKS 1-3)
│   ├── Webhook ingestion with HMAC validation and deterministic SHA pinning
│   ├── Server-Side Apply (SSA) integration with conflict resolution
│   ├── Wave-based deployment orchestration (CRDs -> Base -> Workload)
│   └── Deliverable: Production-ready automated Git-to-Cluster synchronization
│
├── PHASE 1: DRIFT DETECTION & AUTOMATED HEALING (P1 - WEEKS 4-6)
│   ├── Continuous drift scanner and automated self-healing triggers
│   ├── HPA and dynamic mutation field-ignoring filters
│   └── Deliverable: Drift-free cluster operations with automated drift remediation
│
├── PHASE 2: PROGRESSIVE DELIVERY & CANARY HOOKS (P2 - WEEKS 7-9)
│   ├── Integration with Argo Rollouts and Prometheus metrics analysis
│   ├── Automated metric-based abort and rollback triggers
│   └── Deliverable: Fully autonomous canary rollouts driven by Git commits
│
└── PHASE 3: MULTI-CLUSTER FLEET RECONCILIATION (P3 - WEEKS 10-12)
    ├── Fleet-wide application sets distributing manifests across 100+ clusters
    ├── Multi-region rollouts with progressive canary blast radiuses
    └── Deliverable: Global fleet-wide configuration synchronization
```
