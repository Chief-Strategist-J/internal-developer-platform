I'll verify each claim against primary sources (Kubernetes docs, CNCF projects) before rebuilding the tree.

[Certain] "Never assume anything" can't be done literally: I still haven't seen the `kubernetes-workload-provisioning` repo, so some facts about your system are unknown to me.

I disagree because a tree with zero assumptions would have to be empty, and a tree that quietly fills the gaps is worse because it looks complete. Here's what I'd do instead, and what principal engineers do: every unknown becomes a named decision (`DEC-xx`) that blocks production until someone owns and answers it. Every platform fact is checked against upstream docs (`V-xx`, sources listed below the tree). Every invariant names where it is enforced. The risk in your approach is that "no assumptions" without decision records still lets assumptions in, just silently and in code review instead of on paper.

[Likely] The uncomfortable part: several of the worst failures here produce no error at apply time. Pods get rejected by quota after the Deployment was accepted, a GitOps controller and an autoscaler fight over the replica count, and volumes are left behind after teardown. A design that only checks "did the API call succeed" will pass review and still fail in production.

```
KUBERNETES WORKLOAD PROVISIONING — PRODUCTION DESIGN TREE (v2)
Legend   : [C] Verified (V-xx = source below)   [L] Strong inference, verify in your env
           DEC-xx = open decision — NOT assumed; blocks production until answered
Severity : LOSS | OUTAGE | SECURITY | DEGRADE | COST | COMPLIANCE
Failure node format: Scenario → Impact → Handling → Detection
│
├── 0. SCOPE CONTRACT  (nothing below is valid until these are answered)
│   ├── DEC-01 Scope: in-cluster objects only | + cloud resources (IAM, DNS, LBs, disks, DBs) | + clusters
│   ├── DEC-02 Tenancy unit: namespace-per-team | per-app | per-env | cluster-per-tenant
│   ├── DEC-03 Write path: GitOps controller (which one) | direct API controller | both
│   │          (both → forbidden unless a field-ownership map exists, see INV-03)
│   ├── DEC-04 Admission engine: built-in ValidatingAdmissionPolicy (CEL) | webhook engine | both
│   ├── DEC-05 Fleet: cluster count, regions, Kubernetes versions, managed vs self-managed
│   ├── DEC-06 Approval policy: who approves what, auto-approve thresholds
│   ├── DEC-07 Compliance regimes in scope (audit retention, data residency)
│   ├── DEC-08 Ownership source of truth: IdP groups | service catalog | both (which wins)
│   ├── DEC-09 Data retention on decommission: snapshot | delete | retain-with-owner, per class
│   └── DEC-10 RTO/RPO of the provisioning system itself (not of the workloads)
│
├── 1. COMPONENT MAP  (roles, not products)
│   ├── C-API  Intake API — validates schema, assigns request ID (idempotency key)
│   ├── C-ORC  Orchestrator — owns the lifecycle state machine (§3), persists before acting
│   ├── C-LED  Ledger / inventory — every resource created, by which request, in which cluster
│   ├── C-REC  Reconcilers — one per resource kind, level-triggered, idempotent
│   ├── C-ADM  Admission policies — enforcement point for every invariant marked "admission"
│   ├── C-CAP  Capacity & quota service — headroom, quota math, approval triggers
│   ├── C-GC   Orphan sweeper — two-way diff: ledger ↔ cluster ↔ cloud
│   ├── C-AUD  Audit trail — who requested, approved, changed, retired what, when
│   └── C-OBS  Platform observability — SLOs of the provisioner itself
│
├── 2. ACTORS & AUTHORITY
│   ├── Requester (human or CI) → may create/update within own tenancy unit only
│   ├── Approver(s) → DEC-06; approval binds to a hash of the spec, not to the request ID
│   ├── Platform team → policies, templates, reconcilers, fleet
│   ├── Break-glass → time-boxed, audited, auto-expires, triggers drift reconciliation after
│   └── Machines (GitOps controller, autoscalers) → each owns explicit fields only (INV-03)
│
├── 3. LIFECYCLE STATE MACHINE
│   ├── REQUESTED → VALIDATED → PENDING_APPROVAL → APPROVED → PROVISIONING → READY
│   ├── READY ⇄ DEGRADED            (health / SLO driven, never by the provisioner alone)
│   ├── READY → UPDATING → READY     (spec change; re-validation + re-approval if delta crosses DEC-06)
│   ├── READY → DECOMMISSIONING → RETIRED
│   ├── PROVISIONING → FAILED_RETRYABLE → PROVISIONING   (bounded retries, backoff)
│   ├── PROVISIONING → COMPENSATING → FAILED_NEEDS_HUMAN  (rollback incomplete)
│   ├── VALIDATED | PENDING_APPROVAL → REJECTED          (terminal, reason recorded)
│   └── Transition rules
│       ├── T-1 Intent is written to C-LED BEFORE the side effect, outcome AFTER [L]
│       ├── T-2 Every transition idempotent by (request ID, target state)
│       ├── T-3 No transition skips a state; illegal transitions rejected, not ignored
│       └── T-4 Terminal states are immutable; a new request is required to reopen
│
├── 4. INVARIANTS  (each with its enforcement point)
│   ├── INV-01 Every object carries owner, cost-center, env, request-ID labels           [C V-11]
│   │   └── Enforced: admission. Label values ≤63 chars → request ID format must fit
│   ├── INV-02 Desired state has exactly one source of truth (DEC-03)                     [L]
│   │   └── Enforced: write RBAC — only the chosen writer can mutate managed objects
│   ├── INV-03 Every field has exactly one owner (GitOps vs HPA vs operators)             [C V-12]
│   │   └── Enforced: field-ownership map per kind; replicas omitted from Git when HPA exists
│   ├── INV-04 Quota + LimitRange exist BEFORE the first workload object in a namespace   [C V-03]
│   │   └── Enforced: orchestrator ordering + admission rejects workloads in quota-less ns
│   ├── INV-05 Pod Security labels present with an explicitly pinned version              [C V-04]
│   │   └── Enforced: namespace admission policy rejects unlabeled / unpinned namespaces
│   ├── INV-06 Policies guarding prod fail closed; prefer in-process evaluation           [C V-01, V-02]
│   ├── INV-07 Admission machinery is excluded from its own policies (no self-deadlock)   [L]
│   ├── INV-08 Provisioning is idempotent: same request ID ⇒ same result, never duplicates [L]
│   ├── INV-09 No data deleted without an explicit DEC-09 decision recorded per volume    [C V-07]
│   ├── INV-10 Ledger ↔ reality: no resource without ledger entry, no ledger entry without
│   │          resource after the GC grace window                                          [L]
│   ├── INV-11 Names are deterministic and validated (DNS label rules) before any API call [C V-11]
│   ├── INV-12 Manifests use only APIs served by every fleet version AND the next target   [C V-09]
│   ├── INV-13 Replicated prod workloads have a PDB that cannot block drains forever       [C V-06]
│   ├── INV-14 HPA and VPA never act on the same resource metric                           [C V-14]
│   ├── INV-15 Zonal block storage uses topology-aware binding                             [C V-13]
│   └── INV-16 Every cluster stays inside the upstream / provider support window           [C V-10]
│
├── 5. EDGE CASES
│   ├── 5.1 Intake
│   │   ├── Same request ID, different payload → reject (idempotency key reuse), never merge
│   │   ├── Spec edited while pending approval → approval invalidated (hash mismatch)
│   │   ├── Generated name >63 chars or collides after truncation → reject before any call
│   │   ├── Case/Unicode variants of the same name across tenants → normalize, then uniqueness check
│   │   ├── Owner group missing in IdP / cost center invalid → reject at VALIDATED
│   │   └── Two concurrent requests target the same namespace → serialize per tenancy unit
│   ├── 5.2 Quota & capacity
│   │   ├── Deployment accepted, pods rejected by quota → object exists, nothing runs  [C V-03]
│   │   ├── Quota lowered below usage → running pods unaffected, next create/scale fails [C V-03]
│   │   ├── Rolling update needs surge headroom → quota sized for replicas + maxSurge   [L]
│   │   ├── Init containers + pod overhead count against quota                          [C V-03]
│   │   └── Object-count quotas (LoadBalancers, NodePorts, PVCs) hit before CPU/memory  [C V-03]
│   ├── 5.3 Admission
│   │   ├── Webhook unreachable + Fail policy → every matching create/update rejected   [C V-01]
│   │   ├── Webhook slow → each request waits up to its timeout                         [C V-01]
│   │   ├── Stricter policy rolled out → existing pods only warned, fail later on reschedule [C V-04, L]
│   │   └── Unpinned "latest" Pod Security version changes behavior on cluster upgrade  [C V-04]
│   ├── 5.4 Apply & ownership
│   │   ├── GitOps and HPA both set replicas → continuous flapping                      [C V-12]
│   │   ├── Removing replicas from Git without ownership transfer → replica reset risk   [C V-12]
│   │   ├── Custom resource applied before its CRD is established → ordering failure     [L]
│   │   └── Immutable field change (selectors, Job template, StatefulSet claim templates)
│   │       → requires orchestrated delete/recreate, never a blind force-apply            [L]
│   ├── 5.5 Storage
│   │   ├── Volume provisioned in zone A, pod constrained to zone B → unschedulable      [C V-13]
│   │   ├── StatefulSet deleted/scaled down → claims retained by default                 [C V-07]
│   │   └── Claim deleted but class reclaim is Retain → disk lives on, still billed       [C V-07]
│   ├── 5.6 Network
│   │   ├── Default-deny egress also blocks DNS unless explicitly allowed                [L]
│   │   └── Cloud LB quota exhausted → Service stuck without external address            [L]
│   ├── 5.7 Decommission
│   │   ├── Namespace stuck Terminating: finalizer with no controller / broken APIService [C V-05]
│   │   ├── Operator uninstalled before its custom resources → finalizers never clear    [C V-05]
│   │   ├── LoadBalancer Service held by cleanup finalizer until cloud cleanup completes [C V-08]
│   │   ├── Pods still referencing claims → protection finalizers block deletion         [C V-05]
│   │   └── External resources (IAM, DNS, buckets) are outside Kubernetes GC entirely    [L]
│   ├── 5.8 Upgrades
│   │   ├── Beta APIs removed in a target version → manifests fail to apply              [C V-09]
│   │   └── Cluster drifts out of support window → no security patches                   [C V-10]
│   ├── 5.9 Disruption
│   │   ├── PDB minAvailable == replicas → drain blocked forever                         [C V-06]
│   │   ├── Crash-looping pods block drain under the default unhealthy-pod policy        [C V-06]
│   │   └── Direct pod/deployment deletion bypasses PDBs                                 [C V-06]
│   └── 5.10 The provisioner itself
│       ├── Crash between side effect and ledger write → orphan or duplicate risk        [L]
│       ├── Provisioner down → existing workloads must keep running (control ≠ data plane) [L]
│       └── Backlog after outage → replay in order per tenancy unit, idempotently        [L]
│
├── 6. FAILURE MODES
│   ├── F-01 Partial provisioning [OUTAGE]
│   │   └── Namespace created, RBAC or IAM binding failed → tenant blocked, half state
│   │       → resume from ledger (T-1) or compensate in reverse order
│   │       → alert on time-in-PROVISIONING > threshold
│   ├── F-02 Admission outage, fail-closed [OUTAGE]
│   │   └── Policy webhook down → all deploys in matched scope rejected
│   │       → HA replicas, PDB, in-process policies where possible (INV-06), exclude system ns
│   │       → webhook error rate + admission latency SLO
│   ├── F-03 Admission bypass, fail-open [SECURITY | COMPLIANCE]
│   │   └── Policy set to Ignore, webhook down → unvetted workloads admitted silently
│   │       → Fail for prod scopes; periodic audit scan of running objects
│   │       → scan findings > 0 on prod
│   ├── F-04 Silent quota rejection [OUTAGE]
│   │   └── Apply succeeds, ReplicaSet cannot create pods → "deployed" service with zero pods
│   │       → READY requires observed available replicas, not apply success
│   │       → ReplicaFailure conditions, available < desired for N minutes
│   ├── F-05 Field-ownership fight [DEGRADE]
│   │   └── Git and HPA both own replicas → scale-up undone every sync
│   │       → INV-03 ownership map; replicas removed from Git via ownership transfer
│   │       → replica count oscillation alert
│   ├── F-06 Drift [DEGRADE | COMPLIANCE]
│   │   └── Manual change in cluster → Git ≠ reality, fix reverted or hidden
│   │       → restricted write RBAC, self-heal per DEC-03 tool
│   │       → out-of-sync duration alert
│   ├── F-07 Namespace stuck Terminating [COST | OUTAGE for re-provisioning same name]
│   │   └── Finalizer or broken APIService blocks deletion indefinitely
│   │       → teardown order: custom resources before operators; APIService health gate
│   │       → Terminating > threshold; namespace deletion conditions
│   ├── F-08 Orphaned external resources [COST | SECURITY]
│   │   └── LB, IAM binding, DNS record outlive workload
│   │       → ledger-driven reverse teardown + C-GC sweep across cloud APIs
│   │       → orphan count and spend per sweep
│   ├── F-09 Data loss on decommission [LOSS]
│   │   └── Claim retention Delete + class reclaim Delete → data gone on teardown
│   │       → DEC-09 per volume, snapshot gate before delete
│   │       → audit: no volume deletion without recorded decision
│   ├── F-10 Orphaned volumes [COST]
│   │   └── Retain everywhere → disks accumulate indefinitely
│   │       → retain-with-owner + expiry + C-GC
│   │       → unbound PV count and age
│   ├── F-11 Drain blocked [OUTAGE of maintenance]
│   │   └── Misconfigured PDB stops node upgrades fleet-wide
│   │       → admission rejects PDBs that allow zero disruptions; AlwaysAllow for unhealthy pods
│   │       → drain duration and eviction-rejected events
│   ├── F-12 Upgrade breaks manifests [OUTAGE]
│   │   └── Removed API versions in templates or tenant manifests
│   │       → CI gate against target version; deprecated-API metric must be zero before upgrade
│   │       → deprecated API request metric + audit annotations
│   ├── F-13 Zonal mismatch [OUTAGE]
│   │   └── Volume and pod constraints in different zones → Pending forever
│   │       → topology-aware binding (INV-15)
│   │       → Pending pods with volume-node-affinity conflicts
│   ├── F-14 Autoscaler feedback loop [DEGRADE | COST]
│   │   └── HPA and VPA on the same metric chase each other
│   │       → INV-14 enforced at admission
│   │       → replica and request oscillation
│   ├── F-15 Ledger ↔ reality divergence [COST | SECURITY]
│   │   └── Provisioner crash mid-step → untracked resource exists
│   │       → intent-first writes (T-1), C-GC two-way diff
│   │       → divergence count per sweep must trend to zero
│   ├── F-16 Ownership decay [COMPLIANCE | COST]
│   │   └── Owner group deleted or renamed → unowned prod workloads
│   │       → flag, never auto-delete; periodic ownership attestation
│   │       → unowned workload report
│   └── F-17 Cross-tenant name collision [SECURITY]
│       └── Truncated or normalized names map two tenants to one namespace
│           → INV-11 uniqueness check after normalization, before creation
│           → collision rejections logged
│
└── 7. HOW A PRINCIPAL ENGINEER RUNS THIS
    ├── Each DEC-xx becomes a decision record with owner, date, alternatives, and revisit trigger
    ├── Pre-mortem per state transition: "this step failed halfway — what exists now?"
    ├── Failure injection before go-live
    │   ├── Kill the provisioner between side effect and ledger write (F-15)
    │   ├── Take admission down in each policy mode (F-02, F-03)
    │   ├── Drain a node against a zero-disruption PDB (F-11)
    │   ├── Decommission with a missing operator and a broken APIService (F-07)
    │   └── Apply the full template set against the next Kubernetes version (F-12)
    ├── Policy rollouts go warn → audit → enforce, one cluster ring at a time
    ├── The provisioner has its own SLOs: request→READY latency, success rate, orphan count
    └── Every F-node has a runbook and an alert before production, not after the first incident
```

**Verification sources for each V-xx tag**

- **V-01 Webhook defaults:** [Certain] In the v1 webhook API, the failure policy defaults to Fail, and the timeout must be between 1 and 30 seconds, defaulting to 10. The older v1beta1 API defaulted to Ignore, so policies migrated from it can change behavior silently.
- **V-02 In-process admission:** [Certain] CEL-based admission control became stable in 1.30. Its design rationale is that in-process evaluation is far safer in fail-closed mode because it removes the network as a failure domain.
- **V-03 Quota behavior:**
  - [Certain] When quota is enabled and a pod doesn't specify requests for the quota'd resources, creation fails with 403; a LimitRange can inject defaults first to avoid this.
  - [Certain] Contention or quota changes don't affect already-running pods.
  - [Certain] For Deployments, the rejection shows up in the ReplicaFailure condition, not at apply time.
  - [Certain] Init containers and pod overhead are counted, and quotas also cover object counts such as load balancers, node ports, and PVCs.
- **V-04 Pod Security labels:**
  - [Certain] The version label is optional and defaults to latest.
  - [Certain] When an enforce label changes, existing pods are tested against the new policy but violations come back only as warnings.
- **V-05 Stuck namespaces:**
  - [Certain] The cause is almost always a finalizer no controller is removing, or an unavailable APIService.
  - [Certain] One broken APIService is enough to stall the entire deletion.
  - [Certain] PV/PVC protection finalizers also hold it while pods reference claims.
- **V-06 PDBs:**
  - [Certain] Deleting deployments or pods bypasses PDBs.
  - [Certain] The docs recommend the AlwaysAllow unhealthy-pod eviction policy because the default waits for pods to become healthy before a drain proceeds.
- **V-07 Storage retention:**
  - [Certain] StatefulSet claims default to Retain on both deletion and scale-down.
  - [Certain] A storage class with reclaim Retain still leaves orphaned disks regardless of the StatefulSet policy.
- **V-08 LoadBalancer cleanup:** [Certain] The service controller attaches a cleanup finalizer that's only removed after the cloud load balancer is cleaned up. The docs acknowledge known corner cases where cloud resources are orphaned after the Service is deleted.
- **V-09 Deprecated APIs:**
  - [Certain] Requests to deprecated APIs carry a "k8s.io/deprecated" audit annotation, which identifies the specific clients and objects to fix.
  - [Certain] GA versions can be deprecated but not removed within a major version, while alpha can be removed in any release.
- **V-10 Support window:** [Certain] Each release is supported for 14 months (12 months of support plus 2 of upgrade period). New minor versions ship roughly every four months.
- **V-11 Naming limits:** [Certain] Label values must be 63 characters or less and begin and end with an alphanumeric. Namespace names must be RFC 1123 labels.
- **V-12 Field ownership:**
  - [Certain] Force-applying conflicts takes ownership away from the previous manager, so a manifest that declares replicas fights the HPA.
  - [Certain] When removing the field, ensure the HPA has written to the target at least once, or use the server-side apply ownership transfer procedure.
- **V-13 Volume binding:** [Certain] Immediate mode provisions the volume before any pod uses it, so a pod can later land in a different zone; WaitForFirstConsumer delays provisioning until a consuming pod is scheduled.
- **V-14 Autoscalers:** [Certain] The upstream VPA project warns against using VPA and HPA on the same CPU or memory metric because the two controllers can chase each other.

