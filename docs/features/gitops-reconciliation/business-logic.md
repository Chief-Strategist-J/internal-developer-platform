Your two decisions collide here. "Kubernetes Secrets" plus "Argo CD as the only writer" means secret values must get into the cluster somehow, and Argo CD's own maintainers strongly advise against them passing through Argo CD. So this tree needs one new decision from you, D-SEC below, before it is production-safe.

The pasted template also has four problems I won't copy:

- **Auto-rollback to the previous SHA** can't happen while automated sync is on.
- **Webhook HMAC as the trust gate** is misplaced: a webhook is only a refresh hint, and signed commits are the actual trust boundary.
- **"Force SSA ownership if Git is authoritative"** would break the replica and traffic-weight ownership you need for HPA and Argo Rollouts.
- **Week-based phases and hard-coded retry intervals** are invented numbers, the same issue as the last template.

Same structure and depth, applied to your Argo CD layer. Tags carry over from the last answer; new ones are V-35 to V-40, with sources at the end.

---

# GitOps Reconciliation (Argo CD) — Business Logic Decision Trees

## 1. Architectural Overview & Reconciliation Decision Tree

```
GITOPS RECONCILIATION ENGINE — ARGO CD (DESIRED STATE SYNC DECISION TREE)
│
├── 1. CHANGE TRIGGER TIER [P0]
│   └── Scenario: Approved PR merged to the protected environment branch
│       └── Trigger: Git webhook (fast path) OR periodic reconciliation (safety net)
│           ├── Webhook received
│           │   ├── Check: Webhook secret configured and valid?                     [C V-39]
│           │   │   ├── NO ──> [REJECT event] — note: worst case was only a refresh
│           │   │   └── YES ──> refresh Applications watching this repo/path
│           │   └── Rule: a webhook NEVER grants trust; it only shortens detection time
│           ├── Periodic reconciliation (cfg.reconcile.interval + jitter)            [C V-39]
│           │   └── Catches missed webhooks AND live-cluster drift
│           └── State: REFRESH_REQUESTED
│
├── 2. SOURCE TRUST TIER [P0]
│   └── State: REFRESH_REQUESTED
│       ├── Check: Repo URL in the AppProject's allowed sourceRepos?                 [C V-40]
│       │   ├── NO ──> [REJECT: application spec invalid]
│       │   └── YES ──> continue
│       ├── Action: Resolve target revision to the exact commit SHA; record it
│       ├── Check: Prod project enforces commit signature verification?            [C V-37]
│       │   ├── NO ──> [CI/POLICY FAIL: prod projects must enforce]
│       │   └── YES ──> Check: Commit signed by a key allowed for this project?
│       │       ├── NO / UNSIGNED ──> [REFUSE SYNC: comparison error, page owner]   [C V-37]
│       │       └── YES ──> continue
│       └── Check: Commit reached branch through required review by role?          [YOU, L]
│           ├── NO ──> [Branch protection rejects merge — never reaches Argo CD]
│           └── YES ──> State: SOURCE_TRUSTED (SHA, signer, approver recorded)
│
├── 3. MANIFEST GENERATION TIER [P0]
│   └── State: SOURCE_TRUSTED
│       ├── Action: repo-server renders manifests (Kustomize / Helm) at the pinned SHA
│       ├── Check: Any secret value injected during generation (plugin, values file)? [C V-36]
│       │   ├── YES ──> [BLOCK: generated manifests are cached in plaintext]
│       │   └── NO ──> continue
│       ├── Check: Rendering succeeded and schemas valid?
│       │   ├── NO ──> [HALT: mark ComparisonError; commit status failure to Git]
│       │   └── YES ──> Desired state set extracted
│       └── Rule: no tenant-supplied config-management plugins on prod                [L]
│
├── 4. PROJECT & TENANCY POLICY TIER [P0]
│   └── Evaluation: Is every desired object allowed for this AppProject?            [C V-40]
│       ├── Check: Destination cluster + namespace in project destinations?
│       │   ├── NO ──> [REJECT]
│       │   └── YES ──> continue
│       ├── Check: Any cluster-scoped kind (CRD, ClusterRole, webhook config)?
│       │   ├── YES, tenant project ──> [REJECT: platform-only per your operator rule] [YOU]
│       │   └── YES, platform project ──> allow
│       ├── Check: Kind on project blacklist (ResourceQuota, LimitRange, NetworkPolicy)?
│       │   ├── YES, tenant project ──> [REJECT: guardrails owned by Workload API]
│       │   └── NO ──> continue
│       └── State: POLICY_PASSED
│
├── 5. DIFF & DRIFT TIER [P0 / P1]
│   └── Evaluation: Desired (Git @ SHA) vs Live (cluster)
│       ├── Action: Apply ignore rules for fields owned by other controllers:
│       │   ├── spec.replicas where an autoscaler exists                              [C V-38]
│       │   ├── mesh route weights written by Argo Rollouts                          [C V-31]
│       │   └── fields set by mutating admission / sidecar injection                 [L]
│       ├── Check: Drift after ignore rules?
│       │   ├── NO ──> State: SYNCED ──> [TERMINAL]
│       │   └── YES ──> Classify:
│       │       ├── TYPE A: modified fields (manual change or new commit)
│       │       ├── TYPE B: missing objects (new in Git)
│       │       ├── TYPE C: extraneous objects (removed from Git) ──> prune path, tier 7
│       │       └── TYPE D: objects in namespace not tracked by any Application
│       │           └── orphaned-resource warning to owner, never auto-deleted      [C V-40]
│
├── 6. SYNC GATING TIER [P0]
│   └── Evaluation: May this sync run now?
│       ├── Check: Auto-sync enabled for this environment?                           [C V-40]
│       │   ├── NO ──> State: OUT_OF_SYNC (manual sync by allowed role)
│       │   └── YES ──> continue
│       ├── Check: Sync window state (allow / deny windows)?                         [C V-40]
│       │   ├── DENY active or outside every ALLOW ──> State: SYNC_BLOCKED (queued)
│       │   │   └── Emergency: manual sync only if window permits it + break-glass audit
│       │   └── OPEN ──> continue
│       └── Check: Same SHA already failed a previous automated sync?               [C V-35]
│           ├── YES ──> no automatic retry ──> [ALERT owner: fix-forward commit needed]
│           └── NO ──> State: SYNC_IN_PROGRESS
│
├── 7. APPLY EXECUTION TIER [P0]
│   └── State: SYNC_IN_PROGRESS (sync waves)
│       ├── Wave order: namespaces + guardrails ──> CRDs ──> config ──> workloads ──> routes [L]
│       ├── Apply mode: server-side apply where field ownership matters or objects are large [C V-38]
│       ├── Check: Object already managed by another Application?
│       │   ├── YES ──> [FAIL sync: shared resource]                                  [C V-38]
│       │   └── NO ──> apply
│       ├── Check: Custom resource whose CRD is not yet present?
│       │   ├── CRD in same sync ──> applied first
│       │   └── CRD created by a controller ──> skip dry run for that resource      [C V-38]
│       └── Prune (TYPE C drift):
│           ├── Check: Data-bearing or identity object (PVC, Secret, IAM binding)?
│           │   ├── YES ──> Prune=confirm: human approval required                   [C V-38]
│           │   └── NO ──> Check: Prune=false annotation?
│           │       ├── YES ──> skip; app stays OutOfSync; notify owner             [C V-38]
│           │       └── NO ──> prune last (after new objects are healthy)            [C V-38]
│
└── 8. HEALTH & OUTCOME TIER [P0]
    └── Evaluation: Did the applied state become healthy?
        ├── Rollout objects: health = Argo Rollouts status (canary steps, analysis)  [C V-31]
        ├── HEALTHY ──> State: SYNC_SUCCEEDED ──> commit status success; audit record
        └── DEGRADED / PROGRESSING past cfg.sync.health_timeout
            ├── Inside an active canary ──> Argo Rollouts aborts (traffic first, P0-03)
            ├── Outside a canary ──> NO automatic imperative rollback                [C V-35]
            │   └── Action: page owner ──> fix-forward or revert commit through PR
            └── State: SYNC_FAILED ──> notification + incident timeline entry
```

---

## 2. Invariants, Rationale & Enforcement Trees

### Invariant P0-01: Git Owns Desired State; Runtime-Owned Fields Are Declared
- **Failure mechanism:** Self-heal reverts every live change. That includes legitimate ones from the autoscaler and Argo Rollouts, unless those fields are excluded from both the diff and the sync. [C V-38]
- **Rationale:** "Git is authoritative" without a field-ownership map produces flapping. Self-heal and auto-prune are both off by default, so turning them on is an explicit decision you must make per environment. [C V-40]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: OWNERSHIP-AWARE SELF-HEAL (P0-01)
Trigger: Drift detected on prod Application
├── Check: Field listed in ignore rules AND RespectIgnoreDifferences on?   [C V-38]
│   ├── YES ──> leave field to its runtime owner
│   └── NO ──> Check: Self-heal enabled for this environment?
│       ├── YES ──> revert live to Git @ SHA; emit drift event (who changed it, from audit)
│       └── NO ──> mark OUT_OF_SYNC; notify owner
```

### Invariant P0-02: Only Signed, Reviewed Commits Reach Production
- **Failure mechanism:** Without enforcement, anyone with push access, or a compromised CI token, can change production. Signature verification exists but isn't enforced until configured per project. [C V-37]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: SIGNED SOURCE (P0-02)
Trigger: Sync to prod AppProject
├── Check: Resolved commit signed by an allowed key?                      [C V-37]
│   ├── NO ──> refuse sync; alert security + owner
│   └── YES ──> Check: Merge required role-based review?                  [YOU]
│       ├── NO ──> [blocked upstream by branch protection]
│       └── YES ──> proceed; record {SHA, signer, approver}
└── Note: enforcing verification disables local-source syncs              [C V-37]
```

### Invariant P0-03: Secret Values Never Pass Through Argo CD — D-SEC Required
- **Failure mechanism:** Secrets injected during manifest generation are cached in plaintext in Argo CD's Redis and exposed through the repo-server API. [C V-36]
- **Rationale:** Argo CD's maintainers strongly recommend populating secrets on the destination cluster instead. Your "Kubernetes Secrets" decision still holds: the Secret objects are created in-cluster by a separate controller, not by Argo CD from Git.
- **D-SEC (your decision):** which destination-cluster mechanism creates those Secrets. Options named by Argo CD's documentation include Sealed Secrets, External Secrets Operator and the Secrets Store CSI Driver. [C V-36]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: NO SECRETS IN THE GITOPS PATH (P0-03)
Trigger: CI on every PR + render in repo-server
├── Check: Secret objects with plaintext data/stringData in Git?          [L]
│   ├── YES ──> [CI FAIL]
│   └── NO ──> Check: generation plugin injecting values?                  [C V-36]
│       ├── YES ──> [BLOCK on prod projects]
│       └── NO ──> allowed (Git holds references / encrypted form per D-SEC)
```

### Invariant P0-04: Rollback Is a Git Revert, Never an Imperative Toggle
- **Failure mechanism:** Argo CD refuses an application rollback while automated sync is enabled. An automated sync that failed on a commit won't retry that same commit. [C V-35] Turning auto-sync off to roll back leaves prod silently unmanaged until someone turns it back on.
- **Enforcement:**
```
INVARIANT ENFORCEMENT: GIT-NATIVE ROLLBACK (P0-04)
Trigger: Bad release outside an active canary
├── Action: Revert PR (signed, role-approved) ──> new SHA ──> normal sync
├── Check: Severity requires faster path?
│   ├── YES ──> break-glass: disable auto-sync + history rollback,
│   │          with auto-expiry and a mandatory revert commit before re-enable  [C V-35, L]
│   └── NO ──> revert PR only
└── Check: auto-sync disabled on any prod app > cfg.autosync.max_off?
    └── YES ──> page platform on-call
```

### Invariant P0-05: Tenants Can't Escape Their Project
- **Failure mechanism:** The default project allows any repo, any destination and any kind. A tenant Application could deploy cluster-scoped objects or remove its own quotas. [C V-40]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: PROJECT BOUNDARY (P0-05)
Trigger: AppProject created by Workload API (one per app, envs as destinations) [L]
├── sourceRepos: only the app's repos
├── destinations: only that app's namespaces per env
├── clusterResourceWhitelist: empty (operators/CRDs are platform-only)     [YOU, C V-40]
├── namespaceResourceBlacklist: ResourceQuota, LimitRange, NetworkPolicy   [C V-40]
├── orphanedResources: warn                                                [C V-40]
├── signature verification: enforced for prod                              [C V-37]
└── Check: any Application in the "default" project?
    └── YES ──> [ADMISSION REJECT]                                         [L]
```

### Invariant P0-06: Data-Bearing Objects Are Never Pruned or Deleted Silently
- **Failure mechanism:** Removing a PVC, Secret or IAM binding manifest from Git (or deleting the Application) can remove the live object along with its data or access. [L]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: CONFIRMED DESTRUCTION (P0-06)
Trigger: Prune or Application deletion touching data-bearing kinds
├── Annotation required on these kinds: Prune=confirm, Delete=confirm      [C V-38]
├── Approval = role-based approver applies deletion-approved annotation    [C V-38, YOU]
└── Check: Decommission retention decision recorded (prior tree P0-09)?
    ├── NO ──> [BLOCK]
    └── YES ──> proceed
```

### Invariant P0-07: Argo CD Is a Tier-0 System
- **Failure mechanism:** If Argo CD is down, misconfigured or compromised, every tenant is affected at once. It holds cluster credentials and sees every desired state. [L]
- **Enforcement:**
```
INVARIANT ENFORCEMENT: ARGO CD AS TIER 0 (P0-07)
├── Runs highly available; its own config is itself in Git (bootstrap app)   [L]
├── Network policies restrict access to Redis + repo-server                 [C V-36]
├── SSO via IdP groups; no local admin in prod; API tokens scoped + expiring [L]
├── Argo CD audit events shipped to the locked audit bucket (prior P0-07)   [L]
├── Break-glass path tested (prior Case 11)
└── Its own backup + restore drill (Applications, Projects, settings)       [L]
```

---

## 3. Edge Cases & Decision Trees

### Case 01: Autoscaler or Rollouts vs Argo CD on Owned Fields
```
EDGE CASE 01: FIELD OWNERSHIP FIGHT
Trigger: Field flips repeatedly between Git value and runtime value
├── Check: Autoscaler attached? ──> ignore spec.replicas + RespectIgnoreDifferences [C V-38]
├── Check: Field is a mesh route weight? ──> ignore; Argo Rollouts owns it          [C V-31]
├── Note: RespectIgnoreDifferences only applies to objects that already exist       [C V-38]
│   └── First creation still applies Git's value ──> keep Git value safe as a start
```

### Case 02: Perpetual Drift From Mutating Admission
- **Mechanism:** Sidecar injection or mutating policies add fields that aren't in Git, so the Application never reaches SYNCED and self-heal loops. [L]
```
EDGE CASE 02: MUTATION LOOP
Trigger: Sync succeeds, object immediately OutOfSync again
├── Check: Drifting fields written by a known mutator (mesh injector, policy engine)?
│   ├── YES ──> add ignore rule for exactly those paths (never whole objects)
│   └── NO ──> Check: two Applications or two writers? ──> Case 05 / P0-01
└── Self-heal retry backoff = cfg.selfheal.backoff (never unbounded)               [L]
```

### Case 03: Automated Sync Failed on a Commit
```
EDGE CASE 03: NO AUTOMATIC RETRY ON SAME SHA
Trigger: Sync failed at SHA X
├── Fact: automated sync will not retry the same SHA + parameters              [C V-35]
├── Action: alert owner with failure reason
└── Resolution: fix-forward commit (new SHA) OR authorized manual retry
```

### Case 04: Custom Resource Before Its CRD
```
EDGE CASE 04: MISSING RESOURCE TYPE
Trigger: Dry run fails "could not find the requested resource"
├── Check: CRD is in the same sync?
│   ├── YES ──> applied automatically before the custom resource              [C V-38]
│   └── NO ──> Check: CRD installed by a platform operator?
│       ├── YES ──> SkipDryRunOnMissingResource on that resource + wave after operator [C V-38]
│       └── NO ──> [FAIL: platform must provide the CRD first]
```

### Case 05: Two Applications Claim One Object
```
EDGE CASE 05: SHARED RESOURCE
Trigger: Same object rendered by two Applications
├── FailOnSharedResource enabled on all Applications                           [C V-38]
└── [FAIL sync with both Application names] ──> owner decision, one owner only
```

### Case 06: Unsigned Commit From an Automation Bot
- **Mechanism:** Image updaters and CI bots often push unsigned commits, which then fail verification. [C V-37]
```
EDGE CASE 06: BOT COMMIT REJECTED
Trigger: Sync refused, signer unknown
├── Check: Commit from approved automation identity?
│   ├── YES ──> bot signs with its own key, listed per project, rotated per policy
│   └── NO ──> security incident path (unexpected writer to env branch)
```

### Case 07: Emergency Fix During a Deny Window
```
EDGE CASE 07: FREEZE vs INCIDENT
Trigger: Fix needed while a deny window is active
├── Check: Window allows manual sync?                                          [C V-40]
│   ├── YES ──> manual sync by on-call role, audited, linked to incident
│   └── NO ──> break-glass: time-boxed window override, two-person approval     [L]
└── Post-incident: review why the freeze blocked a fix
```

### Case 08: Application Deleted by Mistake
- **Mechanism:** Deleting an Application that carries the resources finalizer cascades deletion to everything it manages. [L]
```
EDGE CASE 08: CASCADING DELETE
├── Prevention: RBAC denies delete on prod Applications to all non-platform roles  [L]
├── Prevention: Delete=confirm / Delete=false on data-bearing objects            [C V-38]
└── Recovery: recreate Application from Git; restore data per backup tree
```

### Case 09: Git Provider Unreachable
```
EDGE CASE 09: SOURCE OUTAGE
Trigger: repo-server can't fetch
├── Effect: no new revisions sync; live workloads keep running                 [L]
├── Check: Fix needed during outage?
│   └── YES ──> break-glass path (prior Case 11); commit when Git returns
└── Alert: comparison errors across many apps = source outage, not app failures
```

### Case 10: Object Too Large for Client-Side Apply
```
EDGE CASE 10: ANNOTATION SIZE LIMIT
Trigger: Apply fails on large objects (big CRDs, ConfigMaps)
└── Use server-side apply: it doesn't store the last-applied annotation        [C V-38]
```

### Case 11: Break-Glass Change Left Uncommitted
```
EDGE CASE 11: RECONCILIATION ERASES THE FIX
Trigger: Auto-sync about to be re-enabled after emergency change
├── Check: Emergency change committed to Git (signed, reviewed)?
│   ├── NO ──> [BLOCK re-enable] — self-heal would revert the fix
│   └── YES ──> re-enable; verify SYNCED at the new SHA
```

---

## 4. Phase-Gated Delivery Roadmap Tree

```
GITOPS RECONCILIATION ROADMAP (exit gates, no dates)
│
├── PHASE 0: TRUSTED SOURCE + ARGO CD AS TIER 0 (P0)
│   ├── Argo CD HA, SSO via IdP groups, no local admin, audit events shipped
│   ├── Protected env branches; role-based reviews; signed commits enforced    [C V-37]
│   ├── Webhooks configured with secrets; polling kept as safety net           [C V-39]
│   ├── D-SEC decided; secret-in-Git CI check                                  [C V-36]
│   └── EXIT GATE: unsigned commit refused; secret never visible in Argo CD
│
├── PHASE 1: TENANT PROJECTS + SAFE SYNC (P0)
│   ├── One AppProject per app with destinations, resource lists, orphan warnings [C V-40]
│   ├── Ownership map + ignore rules (replicas, weights, mutations)             [C V-38]
│   ├── Waves, FailOnSharedResource, Prune=confirm / Delete=confirm            [C V-38]
│   └── EXIT GATE: tenant can't create cluster-scoped objects; no flapping on load test
│
├── PHASE 2: DRIFT CONTROL + CANARY INTEGRATION (P0/P1)
│   ├── Self-heal on prod with bounded backoff; drift events attributed via audit
│   ├── Argo Rollouts health integrated; canary abort independent of Argo CD
│   ├── Sync windows for freezes; emergency path tested
│   └── EXIT GATE: revert-PR rollback measured; failed-SHA alert fires
│
└── PHASE 3: FLEET (P1 — when cluster count grows, per B-3)
    ├── Templated Applications across clusters, still one project per app
    ├── Cluster rings for platform changes (policy, mesh, Argo CD upgrades)
    └── EXIT GATE: ring rollout halts automatically on degraded health
```

---

**Sources for V-35 to V-40**

- **V-35 Rollback and retries:** [Certain] Argo CD's automated sync documentation states that automatic sync will not reattempt a sync if the previous attempt against the same commit-SHA and parameters failed, and rollback cannot be performed against an application with automated sync enabled.
- **V-36 Secrets:**
  - [Certain] Argo CD's docs describe populating secrets on the destination cluster with tools like Sealed Secrets, External Secrets Operator, and Kubernetes Secrets Store CSI Driver, and strongly recommend this style.
  - [Certain] The reason: Argo CD stores generated manifests in plaintext in its Redis cache, so injecting secrets into the manifests increases risk.
- **V-37 Signed commits:**
  - [Certain] Argo CD refuses to sync any revision without a valid signature from one of the configured keys. By default, verification is enabled but not enforced.
  - [Certain] Once enforced, you can no longer sync from local sources.
  - [Likely] A third-party guide notes that automated tools like image updaters may push unsigned commits. The same guide says newer Argo CD versions replace the legacy `signatureKeys` field with source integrity policies, so check which one your version uses.
- **V-38 Sync options:**
  - [Certain] `Prune=confirm` and `Delete=confirm` require approval via the UI, CLI or a deletion-approved annotation before Argo CD acts.
  - [Certain] FailOnSharedResource fails the sync when a resource is already applied by another Application. RespectIgnoreDifferences only takes effect when the resource already exists in the cluster.
  - [Certain] Server-side apply avoids the 262,144-byte annotation limit.
  - [Certain] A resource can skip the dry run when its CRD isn't present yet.
- **V-39 Triggers:**
  - [Certain] The default reconciliation timeout is 120 seconds plus up to 60 seconds of jitter.
  - [Certain] The webhook secret is described as optional because an unauthenticated event can only trigger a refresh, but it should be set anyway if Argo CD is internet-reachable.
- **V-40 Projects and windows:**
  - [Certain] AppProjects support blacklisting namespaced kinds such as ResourceQuota, LimitRange and NetworkPolicy, plus sync windows that restrict when Applications may sync.
  - [Certain] If any allow window exists, syncs outside all allow windows are blocked, and deny windows block syncs even when an allow window is active.
  - [Certain] By default, automated sync doesn't prune, and live-cluster changes don't trigger automated sync until self-heal is enabled.
