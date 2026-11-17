The template you pasted contains things your platform can't copy as-is. It hard-codes thresholds nobody decided (0.5% errors, 250 ms p99, 100 requests, 15 s drain). It presents "real-life incidents" with no source. And it falls back to a degraded rollout when the mesh is missing, which contradicts your decision that the mesh is required.

I disagree with mirroring it literally because invented numbers become someone's production SLO by accident. Here's what I'd do instead:

- Every threshold is a named configurable value (`cfg.*`) with a floor, a default and a ceiling owned by a named person.
- Every "incident" is a failure mechanism tagged to the sources already verified in this thread (V-01 to V-34).
- Every fallback in your platform is a rejection, never a silent downgrade.

The risk in copying the template is a document that reads as production-grade while carrying assumptions nobody signed off.

Same structure and depth as your template, applied to `kubernetes-workload-provisioning`:

---

# Kubernetes Workload Provisioning — Business Logic Decision Trees

Tags: `[C V-xx]` verified earlier in this thread · `[L]` strong inference, verify in your environment · `cfg.*` configurable value (floor/default/ceiling, owner required) · `BLOCKER` = B-1 to B-4 from v4.

## 1. Architectural Overview & Request Dispatch Decision Tree

```
WORKLOAD PROVISIONING ENGINE (REQUEST DISPATCH & LIFECYCLE DECISION TREE)
│
├── 1. REQUEST INGESTION & IDEMPOTENCY TIER [P0]
│   └── Scenario: Team submits a request to the Cluster, Cloud Resource, or Workload API
│       └── Trigger: Inbound API request
│           ├── Check: Caller authenticated via IdP; member of an authorized group?      [C V-28]
│           │   ├── NO ──> [REJECT: 401 / 403 — never reveal whether target exists]
│           │   └── YES ──> Check: Idempotency key present?
│           │       ├── NO ──> [REJECT: 400 — key mandatory on all mutating calls]   [L]
│           │       └── YES ──> Check: Key seen before?
│           │           ├── SAME KEY + SAME PAYLOAD HASH ──> [RETURN: original result / status]
│           │           ├── SAME KEY + DIFFERENT PAYLOAD ──> [REJECT: 422 — key reuse]   [L]
│           │           ├── SAME KEY + ORIGINAL STILL RUNNING ──> [REJECT: 409 — in flight] [L]
│           │           └── NEW KEY ──> Check: Schema valid for this API + workload class?
│           │               ├── NO ──> [REJECT: 422 — field-level errors returned]
│           │               └── YES ──> Write intent to ledger ──> State: REQUESTED
│
├── 2. OWNERSHIP, NAMING & POLICY RESOLUTION TIER [P0]
│   └── State: REQUESTED
│       ├── Check: Owner group exists in IdP AND cost center valid in catalog?
│       │   ├── NO ──> [REJECT: 422 — unowned requests never proceed]
│       │   └── YES ──> continue
│       ├── Check: Generated names valid DNS labels (≤63) after normalization?          [C V-11]
│       │   ├── NO ──> [REJECT: 422 — before any cloud or cluster call]
│       │   └── YES ──> Check: Normalized name unique across all tenants?
│       │       ├── NO ──> [REJECT: 409 — collision]
│       │       └── YES ──> continue
│       ├── Check: Request violates a platform floor (cfg.* below floor)?
│       │   ├── YES ──> [REJECT: 422 — names the floor and its owner]
│       │   └── NO ──> State: VALIDATED
│       └── Check: Approval required for this role + change type?                      [YOU]
│           ├── YES ──> State: PENDING_APPROVAL (approval bound to spec hash)            [L]
│           │   ├── Spec edited while pending ──> [INVALIDATE approval; re-request]
│           │   ├── Rejected ──> State: REJECTED (terminal, reason recorded)
│           │   └── Approved ──> State: APPROVED
│           └── NO ──> State: APPROVED (auto, approver = policy ID, audited)
│
├── 3. CROSS-API DEPENDENCY & ORDERING TIER [P0]
│   └── State: APPROVED
│       ├── Check: Target cluster exists AND passes hardened baseline?                 [C V-28]
│       │   ├── NO, Cluster API request pending ──> [WAIT: dependency edge in ledger]
│       │   ├── NO, cluster non-compliant ──> [BLOCK: 409 — target not READY]
│       │   └── YES ──> continue
│       ├── Check: Required cloud resources (IAM binding, DNS, LB, disk) READY?
│       │   ├── NO ──> [WAIT or FAIL per dependency timeout cfg.dependency.timeout]
│       │   └── YES ──> continue
│       └── Order enforced: cluster ──> cloud resources ──> namespace ──> workload
│           Teardown enforced in exact reverse order                                    [L]
│
├── 4. CLUSTER PROVISIONING TIER — Cluster API [P0]
│   └── State: PROVISIONING (cluster)
│       ├── Action: Create GKE cluster to baseline: private nodes, private endpoint, Shielded
│       │   nodes, Workload Identity Federation, release channel, Google Groups RBAC,
│       │   application-layer secrets encryption, Binary Authorization, Backup agent
│       │                                                       [C V-28, V-29, V-33, V-34]
│       ├── Action: Install platform components via Argo CD (mesh, Rollouts, policies, agents)
│       ├── Check: Every baseline control verified by posture scan?
│       │   ├── NO ──> State: FAILED_RETRYABLE ──> retry ≤ cfg.retry.max ──> FAILED_NEEDS_HUMAN
│       │   └── YES ──> State: READY (cluster)
│       └── Never: hand a cluster to tenants before READY
│
├── 5. CLOUD RESOURCE PROVISIONING TIER — Cloud Resource API [P0]
│   └── State: PROVISIONING (cloud resource)
│       ├── Action: Record intent in ledger BEFORE the cloud call                       [L]
│       ├── Action: Create resource with request-ID, owner, cost-center labels
│       ├── Check: Cloud quota available?
│       │   ├── NO ──> [FAIL: 409 quota — surfaced to requester, no partial resource]
│       │   └── YES ──> continue
│       ├── Check: IAM grant targets a workload identity (never a key, never a person)? [C V-28]
│       │   ├── NO ──> [REJECT: 422]
│       │   └── YES ──> Record outcome in ledger ──> State: READY (resource)
│
├── 6. NAMESPACE & WORKLOAD PROVISIONING TIER — Workload API [P0]
│   └── State: PROVISIONING (workload)
│       ├── Step 1: Namespace per app per env                                         [YOU]
│       ├── Step 2: Quota + LimitRange (before anything else)                         [C V-03]
│       ├── Step 3: Pod Security labels with pinned version                           [C V-04]
│       ├── Step 4: Default-deny NetworkPolicy + mesh injection + mTLS STRICT          [C V-30]
│       ├── Step 5: RBAC bound to IdP groups; workload service account               [C V-28]
│       ├── Step 6: Argo CD Application created (only writer)                          [YOU]
│       ├── Step 7: Rollout object (Argo Rollouts) — replicas owned by autoscaler if any [C V-12]
│       └── Check: Available replicas == desired within cfg.ready.timeout?
│           ├── NO, ReplicaFailure (quota) ──> [FAIL: surfaced as quota error]      [C V-03]
│           ├── NO, ImagePull / admission ──> [FAIL: surfaced with policy reason]
│           └── YES ──> State: READY ──> [CONFIRM: 200 with observed state, not apply result]
│
├── 7. DELIVERY & CANARY TIER [P0 / P1]
│   └── Scenario: New image digest promoted to prod
│       ├── Check: Digest attested and allowed by Binary Authorization?              [C V-29]
│       │   ├── NO ──> [REJECT at admission — no rollout starts]
│       │   └── YES ──> continue
│       ├── Check: Migration metadata marked non-expandable (drop/rename)?           [L]
│       │   ├── YES ──> [HARD BLOCK: canary prohibited without expand/contract]
│       │   └── NO ──> continue
│       ├── Check: Mesh present and healthy in target cluster?                        [YOU]
│       │   ├── NO ──> [REJECT: 409 — no fallback; mesh is a platform requirement]
│       │   └── YES ──> State: CANARY_ACTIVE                                          [C V-31]
│       ├── For each step in cfg.canary.steps:
│       │   ├── Action: Argo Rollouts sets mesh weight (sole owner of weights)        [C V-31]
│       │   ├── Action: Analysis window cfg.canary.window
│       │   │   ├── Check: Sample size ≥ cfg.canary.min_sample?
│       │   │   │   ├── NO ──> synthetic probe traffic, or INCONCLUSIVE ──> treat as FAIL
│       │   │   │   └── YES ──> evaluate cfg.canary.metrics vs stable baseline
│       │   │   ├── ANY METRIC FAILED or MISSING ──> [ABORT: weight 0 to canary, then scale down]
│       │   │   └── ALL PASS ──> next step
│       └── Final: promote ──> stable = new digest ──> State: PROMOTED
│           Rollback outside an active canary = Git revert through Argo CD             [L]
│
└── 8. DECOMMISSION TIER [P0]
    └── Trigger: Approved retire request
        ├── Step 1: Traffic removed (mesh routes, DNS) ──> drain cfg.decom.drain
        ├── Step 2: Workload + Rollout deleted via Git
        ├── Step 3: Check: Data retention decision recorded per volume?               [C V-07]
        │   ├── NO ──> [BLOCK: no storage deletion]
        │   └── YES ──> snapshot | delete | retain-with-owner per decision
        ├── Step 4: Cloud resources (IAM, LB, DNS) removed in reverse order          [C V-08]
        ├── Step 5: Namespace deleted
        │   └── Check: Terminating > cfg.decom.ns_timeout? ──> runbook (finalizers, APIService) [C V-05]
        ├── Step 6: Locked backups retained until lock and regime retention expire  [C V-33, B-1]
        └── State: RETIRED ──> audit record closed
```

---

## 2. Invariants, Rationale & Enforcement Trees

### Invariant P0-01: READY Means Observed Availability, Never "Apply Succeeded"
- **Failure mechanism:** A Deployment is accepted, but pods are rejected by quota. The rejection only appears in a ReplicaFailure condition, so a service reports "deployed" while zero pods run. [C V-03]
- **Rationale:** An API accepting an object is not the same as a working service. The tenant's contract is about availability.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: OBSERVED READINESS (P0-01)
Trigger: Workload applied by Argo CD
├── Action: Watch Rollout status + ReplicaSet conditions
├── Check: available == desired within cfg.ready.timeout?
│   ├── YES ──> State: READY
│   └── NO ──> Classify: quota | image | admission | scheduling | crash
│       └── [FAIL with classified reason; never report READY]
```

### Invariant P0-02: One Writer Per Object, One Owner Per Field
- **Failure mechanism:** Two writers set the same field, such as Git and the autoscaler on `replicas`, or Argo CD and Argo Rollouts on mesh weights. They overwrite each other continuously. [C V-12]
- **Rationale:** With your design, Argo CD owns desired state, Argo Rollouts owns traffic weights and the autoscaler owns replicas. Any field without a declared owner is a future flapping incident.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: FIELD OWNERSHIP (P0-02)
Trigger: Manifest change in Git OR runtime write
├── Check: Field listed in ownership map for this kind?
│   ├── NO ──> [CI FAIL: undeclared field ownership]
│   └── YES ──> Check: Writer == declared owner?
│       ├── YES ──> allow
│       └── NO ──> [REJECT at admission / ignore-diff in Argo CD for owned fields] [L]
```

### Invariant P0-03: Safe Canary Abort — Traffic Before Pods
- **Failure mechanism:** If canary pods are removed while the mesh still routes traffic to them, requests hit dead endpoints.
- **Rationale:** The weight must return to stable first, connections must drain, and only then do pods scale down. [L] Verify the abort delay and scale-down behaviour of your Argo Rollouts version and set it explicitly rather than relying on the default.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: SAFE CANARY ABORT (P0-03)
Trigger: Analysis failure OR manual abort
├── Action 1: Canary weight ──> 0 (Argo Rollouts, mesh route)
├── Action 2: Wait cfg.canary.abort_drain
├── Action 3: Check canary request rate == 0?
│   ├── NO ──> keep waiting up to cfg.canary.abort_max, then page
│   └── YES ──> Action 4: scale down canary ReplicaSet
```

### Invariant P0-04: Guardrails Fail Closed, Admission Never Blocks Itself
- **Failure mechanism:** A webhook with fail-closed policy goes down, so every matching create is rejected. With fail-open policy, unvetted workloads land silently. If the webhook's own namespace is matched, it can't restart itself. [C V-01, V-02]
- **Rationale:** For a government platform, a security bypass is worse than a deploy outage. But the outage must be survivable.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: ADMISSION SAFETY (P0-04)
Trigger: Policy added or changed
├── Check: Rule expressible as in-process CEL?
│   ├── YES ──> CEL policy (no network failure domain)                   [C V-02]
│   └── NO ──> webhook engine, failurePolicy Fail for prod scopes        [C V-01]
├── Check: Admission system + kube-system excluded from webhook scope?
│   ├── NO ──> [CI FAIL]
│   └── YES ──> continue
└── Check: Rollout mode warn ──> audit ──> enforce, ring by ring?
    ├── NO ──> [CI FAIL]
    └── YES ──> allow
```

### Invariant P0-05: Intent Before Side Effect, Across All Three APIs
- **Failure mechanism:** The provisioner crashes between creating a cloud resource and recording it. The resource then exists with no record, which leaks cost and access. Load balancers and volumes can outlive their Kubernetes objects. [C V-07, V-08]
- **Rationale:** The ledger is the only way to tear down what you created, and to prove it to an auditor.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: INTENT-FIRST LEDGER (P0-05)
Trigger: Any create / update / delete side effect
├── Action 1: Write intent {request ID, target, action} to ledger
├── Action 2: Perform side effect (idempotent by request ID)
├── Action 3: Write outcome to ledger
└── Background: two-way sweep (ledger ↔ cluster ↔ cloud) every cfg.gc.interval
    ├── Resource without ledger entry ──> flag owner, never auto-delete
    └── Ledger entry without resource ──> resume or mark FAILED
```

### Invariant P0-06: Secrets Encrypted With Your Key, Rotated Without Downtime
- **Failure mechanism:** GKE's default encryption doesn't use your key; application-layer encryption with your KMS key is off by default. [C V-34] Separately, consumers that read secrets as env vars or subPath mounts never see a rotation. [C V-15]
- **Rationale:** Government key custody, plus rotation that can't break consumers.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: SECRET LIFECYCLE (P0-06)
Trigger: Cluster creation OR secret rotation
├── Check: Application-layer encryption ON with key in separate project?   [C V-34]
│   ├── NO ──> [BLOCK cluster READY]
│   └── YES ──> continue
└── Rotation:
    ├── Action 1: Add new value; old value stays valid (dual-valid window cfg.rotation.overlap)
    ├── Action 2: Restart consumers listed in inventory (env-var / subPath)   [C V-15]
    ├── Check: All consumers report new version?
    │   ├── NO ──> hold old value valid; page owner
    │   └── YES ──> Action 3: revoke old value
```

### Invariant P0-07: Audit Evidence Is Complete and Tamper-Resistant
- **Failure mechanism:** Data Access audit logs are disabled by default. When enabled, they land in a bucket with 30-day default retention, while Admin Activity logs are fixed at 400 days. [C V-32]
- **Rationale:** You said all logs must be retained. That is only true if routing, retention and locks are enforced, not assumed.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: AUDIT COMPLETENESS (P0-07)
Trigger: Project / cluster creation; daily verification
├── Check: Data Access audit logs enabled for all in-scope services?       [C V-32]
├── Check: Logs routed to user-defined bucket, CMEK, retention locked?     [C V-32]
├── Check: Retention ≥ regime requirement?                                 [BLOCKER B-1]
├── Check: If requirement > 400 days ──> Admin Activity also exported?      [C V-32]
│   ├── ANY NO ──> [BLOCK READY / page platform on-call]
│   └── ALL YES ──> evidence pipeline healthy
```

### Invariant P0-08: Only Attested Images Run — At Deploy Time and After
- **Failure mechanism:** Tightening the image policy doesn't re-check running pods, and breakglass deploys bypass enforcement. Only continuous validation still reports them. [C V-29]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: IMAGE TRUST (P0-08)
├── Deploy time: Binary Authorization enforcement (attestation required)  [C V-29]
├── Runtime: continuous validation with matching platform policy          [C V-29]
└── Breakglass used?
    ├── YES ──> audit event ──> mandatory review within cfg.breakglass.review
    └── NO ──> normal path
```

### Invariant P0-09: No Data Deleted Without a Recorded Decision
- **Failure mechanism:** Default retention behaviour differs by object. StatefulSet claims are retained by default, a Delete reclaim policy removes the disk, and locked backups can't be deleted early. [C V-07, V-33]
- **Enforcement:** Decommission Step 3 in Section 1 blocks deletion until a retention decision is recorded for every volume.

### Invariant P0-10: mTLS STRICT Only After Every Client Is in the Mesh
- **Failure mechanism:** Switching a namespace to STRICT while some callers still lack sidecars rejects their traffic. [L]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: MTLS CUTOVER (P0-10)
├── New namespaces: STRICT from creation (no legacy callers exist)         [C V-30]
└── Existing namespaces:
    ├── PERMISSIVE ──> measure plaintext connections
    ├── Check: plaintext count == 0 for cfg.mtls.observe?
    │   ├── NO ──> list callers, onboard them; stay PERMISSIVE
    │   └── YES ──> STRICT
```

### Invariant P0-11: No Standing Production Access
- **Failure mechanism:** Access granted to individuals outlives role changes, and "read-only" roles holding nodes/proxy can execute in any pod. [C V-25, V-28]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: ACCESS (P0-11)
├── Access granted only through IdP groups                                 [C V-28]
├── Prod exec/debug only via JIT grant, ≤ cfg.access.max_session           [YOU]
├── Check: any binding to a user, or nodes/proxy for humans/monitoring?   [C V-25]
│   ├── YES ──> [REJECT at admission]
│   └── NO ──> allow
└── Grant expiry enforced by controller, not by humans
```

---

## 3. Edge Cases & Decision Trees

### Case 01: Idempotency Key Reused With a Different Payload
```
EDGE CASE 01: KEY REUSE
Trigger: Request with existing key
├── Check: Payload hash matches stored hash?
│   ├── YES ──> return stored result (no new side effects)
│   └── NO ──> [REJECT 422; log as possible client bug or replay]          [L]
```

### Case 02: Partial Cross-API Failure
- **Mechanism:** The cluster is READY and the IAM binding succeeds, but DNS creation fails, leaving the tenant half-provisioned. [L]
```
EDGE CASE 02: PARTIAL PROVISIONING
Trigger: Step N fails after steps 1..N-1 succeeded
├── Check: Error retryable (quota wait, transient API)?
│   ├── YES ──> retry with backoff ≤ cfg.retry.max (idempotent by request ID)
│   └── NO ──> COMPENSATING: undo steps N-1..1 in reverse, from ledger
│       ├── Compensation succeeds ──> FAILED (clean)
│       └── Compensation fails ──> FAILED_NEEDS_HUMAN + page platform on-call
```

### Case 03: Low-Traffic Canary
- **Mechanism:** Too few requests make error ratios statistically meaningless, which causes both false aborts and false passes. [L]
```
EDGE CASE 03: LOW-SAMPLE ANALYSIS
Trigger: Analysis window ends
├── Check: requests ≥ cfg.canary.min_sample?
│   ├── YES ──> normal metric evaluation
│   └── NO ──> Check: synthetic probes configured for this service?
│       ├── YES ──> evaluate on synthetic + organic
│       └── NO ──> INCONCLUSIVE ──> treat as FAIL (never auto-promote)
```

### Case 04: Non-Expandable Schema Change
```
EDGE CASE 04: BREAKING MIGRATION
Trigger: Release pre-flight
├── Check: Migration marked drop / rename / type-narrowing?
│   ├── YES ──> [HARD BLOCK: split into expand ──> migrate ──> contract releases]
│   └── NO ──> canary allowed (both versions safe on one schema)
```

### Case 05: Mesh Feature Unsupported in Managed Mode
- **Mechanism:** The managed Cloud Service Mesh doesn't support EnvoyFilter or WasmPlugin, and can't coexist with in-cluster Istio. [C V-30]
```
EDGE CASE 05: UNSUPPORTED MESH API
Trigger: Tenant manifest contains EnvoyFilter / WasmPlugin
├── [REJECT at admission with documented alternative]
└── Platform request to run in-cluster Istio ──> [REJECT: conflicts with managed mesh]
```

### Case 06: Secret Consumed as Env Var During Rotation
```
EDGE CASE 06: FROZEN SECRET CONSUMER
Trigger: Rotation started
├── Check: Consumer reads via env var or subPath?                          [C V-15]
│   ├── YES ──> rolling restart required before old value revoked
│   └── NO (volume) ──> wait for kubelet sync; verify app re-reads file    [L]
```

### Case 07: Certificate Renewal Failing Inside the Renewal Window
- **Mechanism:** Public certificate lifetimes are shrinking on a fixed schedule [C V-18], and renewals retry on backoff without failing loudly. [C V-17]
```
EDGE CASE 07: RENEWAL FAILURE
Trigger: Certificate not renewed at expected time
├── Check: days_to_expiry < cfg.cert.page_threshold?
│   ├── YES ──> page owner + platform on-call
│   └── NO ──> ticket; retry continues
└── Check: issuer reachable? DNS/HTTP challenge passing? CA quota?
```

### Case 08: Exec Breaks After Upgrade
- **Mechanism:** Exec authorization semantics changed between versions: create-only roles broke on newer kubectl, and get-only roles break on newer API servers. [C V-24]
```
EDGE CASE 08: ACCESS REGRESSION
Trigger: Cluster upgrade scheduled
├── Action: Run can-i matrix (every role × exec/debug/port-forward) on target version
├── Check: Matrix equals expected?
│   ├── NO ──> [BLOCK upgrade; fix roles first]
│   └── YES ──> proceed
```

### Case 09: Restore Into a Namespace That Still Exists
- **Mechanism:** Restore tools typically skip objects that already exist, and updating an existing claim object doesn't bring back its data. [C V-27, L for Backup for GKE specifics]
```
EDGE CASE 09: RESTORE CONFLICT
Trigger: Restore requested
├── Check: Target cluster exists (restore never creates it)?               [C V-33]
│   ├── NO ──> provision via Cluster API first
│   └── YES ──> Check: Target namespace already populated?
│       ├── YES ──> conflict policy explicit: fail | delete-and-restore     [L]
│       └── NO ──> restore
├── Post-restore: rotate credentials restored from backup                  [L]
└── Post-restore: block outbound to live dependencies until verified       [L]
```

### Case 10: Namespace Stuck Terminating
```
EDGE CASE 10: STUCK TEARDOWN
Trigger: Namespace Terminating > cfg.decom.ns_timeout
├── Check: Unavailable APIService?                                         [C V-05]
│   ├── YES ──> fix/remove APIService first
│   └── NO ──> Check: Custom resources with finalizers but operator gone?
│       ├── YES ──> reinstall operator to process finalizers (never strip blindly)
│       └── NO ──> PV/PVC protection: pods still referencing claims?
```

### Case 11: Argo CD Is Down During an Incident
```
EDGE CASE 11: WRITER UNAVAILABLE
Trigger: Fix needed, Argo CD unhealthy
├── Check: Fix is traffic-only (abort canary, shift weight)?
│   ├── YES ──> Argo Rollouts abort (separate controller)                  [L]
│   └── NO ──> break-glass write path: time-boxed, audited, two-person    [L]
└── After recovery: commit the change to Git BEFORE Argo CD resumes sync
    (otherwise the next sync reverts the fix)
```

### Case 12: Leaver Holds an Active JIT Grant
```
EDGE CASE 12: IDENTITY REVOKED MID-SESSION
Trigger: IdP group removal
├── Group-based RBAC ──> access ends on next authorization check           [C V-28]
├── Active exec sessions ──> terminate via session controller              [L]
└── Owned workloads ──> ownership transfer task to team lead before group deletion
```

---

## 4. Phase-Gated Delivery Roadmap Tree

The template's week numbers are omitted on purpose. Durations depend on team size and blockers B-1 to B-4, so each phase ends on an exit gate rather than a date.

```
WORKLOAD PROVISIONING ROADMAP
│
├── PHASE 0: FOUNDATIONS (P0)
│   ├── Cluster API: hardened GKE baseline + posture scan                 [C V-28]
│   ├── Audit pipeline: Data Access logs, locked buckets, CMEK            [C V-32]
│   ├── IdP groups ──> RBAC; no standing prod access
│   ├── Admission: CEL policies + one webhook engine, fail closed
│   ├── Intent-first ledger shared by all three APIs
│   ├── Application-layer secrets encryption with your KMS key            [C V-34]
│   └── EXIT GATE: B-1 and B-3 answered; baseline scan 100%; audit evidence retrievable
│
├── PHASE 1: WORKLOAD + CLOUD RESOURCE APIS (P0)
│   ├── Workload API: namespace per app/env, quotas, Pod Security, network policy
│   ├── Cloud Resource API: IAM via workload identity, DNS, LB, disks
│   ├── Argo CD as single writer; field-ownership map
│   ├── Binary Authorization enforce + continuous validation                [C V-29]
│   ├── Secrets rotation + certificate automation (public + private CA)
│   ├── Observability + 24/7 on-call (NOT deferred — compliance requires it)
│   └── EXIT GATE: P0-01..P0-08 enforced; partial-failure game day passes
│
├── PHASE 2: MESH + CANARY DELIVERY (P0/P1)
│   ├── Managed Cloud Service Mesh; mTLS STRICT per P0-10                 [C V-30]
│   ├── Argo Rollouts canary with mesh traffic shifting + analysis       [C V-31]
│   ├── Expand/contract migration gate
│   ├── JIT access with session ceiling
│   └── EXIT GATE: injected-error canary aborts safely (P0-03); zero plaintext in prod
│
├── PHASE 3: BACKUP, RESTORE & DR (P0)
│   ├── Backup for GKE plans: secrets + volumes, second region, locked    [C V-33]
│   ├── Pre-created DR cluster via Cluster API; restore plans defined
│   ├── Failover + failback runbooks with fencing
│   └── EXIT GATE: B-2 answered; restore + failover drills meet RTO/RPO
│
└── PHASE 4: OPTIONAL CAPABILITIES (P2 — only on request)
    ├── Header/cohort-based canary routing
    ├── Shadow traffic mirroring (side effects disabled) + response diffing
    └── GPU / batch workload classes
```

---
