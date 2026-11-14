There is no single gold standard for a government Kubernetes platform. There are recognized baselines (the CIS GKE Benchmark, Google's GKE hardening guide, Google's Binary Authorization and Backup for GKE guidance, SRE-style canary analysis), but the compliance numbers have to come from your regime. No baseline can name retention periods, data residency or RTO/RPO for you.

So in this final tree, your answers are followed exactly and marked `[YOU]`. Where you didn't decide or said "configurable", I applied a baseline as the platform floor, marked `[GS]`, and you can override it. Four items stay as blockers because guessing them would make the tree look complete while being wrong.

```
KUBERNETES WORKLOAD PROVISIONING — FINAL BUSINESS TREE WITH DAILY OPERATIONS (v4)
Legend   : [YOU] your decision   [GS] gold-standard default applied (overridable)
           [C V-xx] verified (sources below)   [L] strong inference, verify in your environment
           BLOCKER = cannot be defaulted; go-live waits for an answer
Severity : LOSS | CORRUPT | OUTAGE | SECURITY | DEGRADE | COST | COMPLIANCE
Branch   : Rules → Edge cases → Failures (Scenario → Impact → Handling → Detection)
│
├── 0. DECISION REGISTER
│   ├── Settled
│   │   ├── Scope: clusters + cloud resources + workloads, each behind its own API     [YOU]
│   │   ├── Tenancy: namespace per app per environment                                  [YOU, as read]
│   │   ├── Single writer: Argo CD                                                       [YOU]
│   │   ├── Admission: in-process CEL policies + one webhook engine                      [YOU]
│   │   │   └── Image trust enforced by Binary Authorization                             [GS, C V-29]
│   │   ├── Platform: GKE only                                                           [YOU]
│   │   ├── Paging: all production workloads page 24/7                                   [YOU]
│   │   ├── Approvals: decided by role                                                   [YOU]
│   │   ├── Compliance: government project; all logs retained                           [YOU]
│   │   ├── Secrets: Kubernetes Secrets                                                  [YOU]
│   │   │   └── + application-layer encryption with your Cloud KMS key                   [GS, C V-34]
│   │   ├── Certificates: public + private CA                                            [YOU]
│   │   ├── Delivery: canary                                                             [YOU]
│   │   │   └── via Argo Rollouts, traffic shifted by the mesh                           [GS, C V-31]
│   │   ├── Service mesh: required                                                       [YOU]
│   │   │   └── managed Cloud Service Mesh, mTLS STRICT                                  [GS, C V-30]
│   │   ├── Operators/CRDs: platform team only; tenants cannot install                   [YOU]
│   │   ├── Human access: requested just-in-time when required                           [YOU]
│   │   ├── Access source: IdP groups mapped to RBAC                                    [GS, C V-28]
│   │   └── Observability + on-call: phase 1, not later       [GS — overrides "not initial phase", see §6]
│   ├── Configurable — within platform floors (§6)                                       [YOU + GS]
│   │   └── Images, backup, DR, session length, decommission retention
│   ├── Small open choices (defaulted, confirm)
│   │   ├── Webhook engine: pick exactly one engine, never two                           [L]
│   │   ├── Ownership metadata: service catalog; access always from IdP groups           [GS]
│   │   └── GPU/batch: out of phase 1 unless requested (§4.19)                            [GS]
│   └── BLOCKERS
│       ├── B-1 Jurisdiction + regime → log retention period, data residency, key custody
│       ├── B-2 RTO / RPO numbers per workload class AND for the platform itself
│       ├── B-3 GKE Standard vs Autopilot; cluster and region count for phase 1
│       └── B-4 Tenants: internal teams only, or external parties too
│
├── 1. PURPOSE & VALUE
│   ├── Government teams get compliant, owned, observable, recoverable workloads via three APIs
│   ├── Every running thing traceable to owner, cost center, request, and approval
│   └── Every daily operation is a paved, audited, tested path
│
├── 2. ACTORS & AUTHORITY
│   ├── Requester (human or CI) → acts only inside its app/env namespaces
│   ├── Role-based approvers → approval binds to a hash of the spec                      [L]
│   ├── Workload owner → 24/7 on-call, runbooks, restore drills
│   ├── Platform team → all three APIs, policies, operators, mesh, fleet; its own on-call
│   ├── Security → signing, keys, access policy, exceptions, audit review
│   ├── Auditor → read-only access to audit logs and evidence
│   └── Machines (Argo CD, Argo Rollouts, autoscalers, cert/secret controllers) → explicit fields only
│
├── 3. OFFERINGS — THE THREE APIS
│   ├── Cluster API → creates and lifecycles GKE clusters to the hardened baseline (§4.1)
│   ├── Cloud Resource API → IAM bindings, DNS, load balancers, disks, data stores (§4.2)
│   ├── Workload API → namespaces per app/env, workloads, add-ons (§4.3)
│   ├── Shared across all three
│   │   ├── One request ID and one ledger spanning all three APIs                       [L]
│   │   ├── One approval model, one audit trail
│   │   └── Cross-API ordering: cluster → cloud resources → workload; teardown in reverse [L]
│   └── Workload classes: service | worker | batch | stateful | (GPU deferred) | node-level (platform only)
│
├── 4. DAILY OPERATIONS
│   │
│   ├── 4.1 CLUSTER PROVISIONING (Cluster API)
│   │   ├── Rules — hardened baseline on every cluster                                   [GS, C V-28]
│   │   │   ├── Private nodes; control-plane endpoint not publicly exposed
│   │   │   ├── Shielded GKE Nodes with secure boot and integrity monitoring
│   │   │   ├── Workload Identity Federation; no exported service-account keys
│   │   │   ├── Enrolled in a release channel; versions inside support window (V-10)
│   │   │   ├── Google Groups for RBAC; application-layer secrets encryption (V-34)
│   │   │   ├── Binary Authorization enforcement + continuous validation (V-29)
│   │   │   └── Backup for GKE agent enabled at creation (V-33)
│   │   ├── Edge cases
│   │   │   ├── Cluster created but a baseline control failed to apply → not READY       [L]
│   │   │   ├── Cluster API itself unavailable while workloads keep running              [L]
│   │   │   └── Baseline changes after cluster creation → drift across fleet            [L]
│   │   └── Failures
│   │       ├── Non-baseline cluster reaches prod [SECURITY | COMPLIANCE]
│   │       │   → READY gated on every control verified → posture scan per cluster
│   │       └── Fleet drift [COMPLIANCE] → continuous baseline diff → drift count per cluster
│   │
│   ├── 4.2 CLOUD RESOURCE PROVISIONING (Cloud Resource API)
│   │   ├── Rules
│   │   │   ├── Declarative, reconciled through Argo CD like everything else             [L]
│   │   │   ├── IAM granted to workload identities, never to keys or individuals         [C V-28]
│   │   │   ├── Every cloud resource carries request ID + owner + cost center
│   │   │   └── Cloud resources outlive no workload without a recorded decision          [L]
│   │   ├── Edge cases
│   │   │   ├── Cloud quota exhausted → resource pending, workload waiting               [L]
│   │   │   ├── Cloud-side deletion protection blocks teardown                           [L]
│   │   │   └── Resource created outside the API (console) → unmanaged                   [L]
│   │   └── Failures
│   │       ├── Orphaned cloud resources [COST | SECURITY] (V-08) → ledger-driven teardown + sweep
│   │       └── Over-broad IAM [SECURITY] → least-privilege templates → IAM recommender findings
│   │
│   ├── 4.3 WORKLOAD ONBOARDING (Workload API)
│   │   ├── Rules
│   │   │   ├── Namespace per app per env, with quota, LimitRange, pinned Pod Security,
│   │   │   │   default-deny network policy, mesh injection, ownership labels            [V-03, V-04, V-11]
│   │   │   ├── Idempotent by request ID; approval bound to spec hash                    [L]
│   │   │   └── READY = available replicas observed, not "apply succeeded"               [V-03]
│   │   ├── Edge cases: quota-rejected pods behind an accepted Deployment; surge headroom; name
│   │   │   collisions after normalization; spec edited during approval                 [V-03, L]
│   │   └── Failures: partial provisioning [OUTAGE]; silent quota rejection [OUTAGE]   (from v3 §4.1)
│   │
│   ├── 4.4 IMAGE SUPPLY CHAIN — configurable above floor
│   │   ├── Floor (tenants may tighten, never loosen)                                    [GS]
│   │   │   ├── Images by digest; pull policy explicit                                  [V-20, V-21]
│   │   │   ├── Attested by your CI; enforced at deploy by Binary Authorization         [C V-29]
│   │   │   ├── Continuous validation on, matching the enforcement policy               [C V-29]
│   │   │   └── Only approved registries; per-region mirror                             [L]
│   │   ├── Edge cases
│   │   │   ├── Policy tightened → running pods are not re-checked by enforcement        [C V-29]
│   │   │   ├── Breakglass deploy bypasses enforcement; only monitoring still sees it    [C V-29]
│   │   │   ├── Tag re-push; registry outage; expired pull credentials                   [V-21, L]
│   │   │   └── Attestor key rotated or compromised                                     [L]
│   │   └── Failures
│   │       ├── Non-conformant image running after policy change [SECURITY]
│   │       │   → continuous validation → violation log alerts
│   │       └── Breakglass never reviewed [COMPLIANCE] → audit event alert + mandatory review
│   │
│   ├── 4.5 DELIVER, CANARY & ROLL BACK
│   │   ├── Rules
│   │   │   ├── Same digest promoted dev → nonprod → prod; never rebuilt                 [L]
│   │   │   ├── Canary steps and metric analysis owned by Argo Rollouts                 [C V-31]
│   │   │   ├── Mesh routes are written by Argo Rollouts only (single field owner)     [V-12, L]
│   │   │   ├── Automated abort on analysis failure; promotion gates per role approval  [C V-31, YOU]
│   │   │   ├── Rollback for config or code = Git revert through Argo CD                [L]
│   │   │   └── Schema changes expand/contract; one release backward compatible         [L]
│   │   ├── Edge cases
│   │   │   ├── Analysis metric missing → treat as failure, never as pass               [L]
│   │   │   ├── Canary too small to produce significant signal → false pass             [L]
│   │   │   ├── Argo CD sync fights Rollouts-managed traffic weights                    [V-12, L]
│   │   │   ├── Undo reverts pod template only, not data or in-place config             [V-22]
│   │   │   └── Release across several clusters half done                               [L]
│   │   └── Failures
│   │       ├── Bad release promoted [OUTAGE] → analysis gates → canary vs stable error delta
│   │       └── Forward-only migration [LOSS | CORRUPT] → expand/contract review gate
│   │
│   ├── 4.6 SECRETS, CONFIG & CERTIFICATES
│   │   ├── Rules
│   │   │   ├── Kubernetes Secrets + application-layer encryption with your KMS key     [YOU + C V-34]
│   │   │   ├── KMS key in a separate project; key custody per regime (B-1)             [C V-34, BLOCKER]
│   │   │   ├── Consumer inventory per secret; dual-valid rotation window               [V-15, L]
│   │   │   ├── No subPath mounts for rotating material; env-var consumers restarted    [V-15]
│   │   │   ├── Public certs: automated renewal against shrinking lifetimes             [V-17, V-18]
│   │   │   └── Private CA issues mesh workload certificates                             [C V-30]
│   │   ├── Edge cases: env-var secrets frozen; subPath frozen; app caches secrets; renewal failing
│   │   │   silently; KMS key disabled or destroyed → Secrets unreadable                [V-15, V-17, L]
│   │   └── Failures
│   │       ├── Expired cert [OUTAGE] → automated renewal + days-to-expiry alerts
│   │       ├── KMS key loss [LOSS | OUTAGE] → key destruction protection + key admin separation
│   │       └── Secret leaked [SECURITY] → volume delivery, log scrubbing, rotation runbook
│   │
│   ├── 4.7 NETWORKING & SERVICE MESH
│   │   ├── Rules
│   │   │   ├── Managed Cloud Service Mesh; mTLS STRICT for all prod namespaces         [GS, C V-30]
│   │   │   ├── No in-cluster Istio alongside the managed mesh                          [C V-30]
│   │   │   ├── Default-deny network policy + mesh authorization policies per namespace [L]
│   │   │   ├── DNS zones owned by the Cloud Resource API; no manual records            [L]
│   │   │   └── Egress allowlisted per tenant through egress gateway or policy          [L]
│   │   ├── Edge cases
│   │   │   ├── Required mesh features unsupported in managed mode (e.g. EnvoyFilter)   [C V-30]
│   │   │   ├── Switching STRICT on before every client has a sidecar → outages        [L]
│   │   │   ├── Default-deny blocks DNS egress                                          [L]
│   │   │   └── Mesh upgrade changes proxy behavior fleet-wide                          [L]
│   │   └── Failures
│   │       ├── Plaintext traffic in prod [SECURITY | COMPLIANCE] → STRICT + verification
│   │       │   → non-mTLS connection count must be zero
│   │       └── Mesh control-plane issue [OUTAGE] → staged mesh upgrades per cluster ring
│   │
│   ├── 4.8 OBSERVE & AUDIT (compliance-critical)
│   │   ├── Rules
│   │   │   ├── Data Access audit logs enabled — they're off by default                 [GS, C V-32]
│   │   │   ├── Logs routed to user-defined buckets with locked retention + CMEK        [GS, C V-32]
│   │   │   ├── Retention period = regime requirement                                    [BLOCKER B-1]
│   │   │   ├── If the regime requires > 400 days, Admin Activity logs exported too     [C V-32]
│   │   │   ├── Container logs shipped off-node continuously                           [V-23]
│   │   │   ├── Alerts route to owners resolved from catalog; synthetic route tests     [L]
│   │   │   └── PII scrubbed before storage                                              [L]
│   │   ├── Edge cases
│   │   │   ├── Data Access logs left in default bucket → 30-day retention              [C V-32]
│   │   │   ├── Log bucket in wrong location for residency rules                         [BLOCKER B-1]
│   │   │   ├── Evicted pod → node logs gone; kubectl shows newest file only            [V-23]
│   │   │   └── Audit log volume cost surprise                                          [L]
│   │   └── Failures
│   │       ├── Audit evidence missing [COMPLIANCE] → locked buckets + routing tests
│   │       └── Logs missing during incident [DEGRADE] → agent lag and drop alerts
│   │
│   ├── 4.9 ACCESS & DEBUG
│   │   ├── Rules
│   │   │   ├── No standing prod access; JIT requests approved by role                  [YOU]
│   │   │   ├── Session length configurable under a security-owned ceiling (§6)        [YOU + GS]
│   │   │   ├── RBAC bound to IdP groups only; group removal revokes access             [C V-28]
│   │   │   ├── Exec RBAC verified per Kubernetes/kubectl version                      [V-24]
│   │   │   ├── No human or monitoring role holds nodes/proxy                          [V-25]
│   │   │   └── Every session audited (Data Access logs on)                            [C V-32]
│   │   ├── Edge cases: create-only or get-only exec roles breaking across versions; ephemeral
│   │   │   containers permanent; debug image blocked by policy mid-incident; grant never expires
│   │   │                                                               [V-24, V-26, L]
│   │   └── Failures: locked out during incident [OUTAGE]; standing privilege [SECURITY]
│   │
│   ├── 4.10 ON-CALL & INCIDENTS (phase 1)
│   │   ├── Rules                                                                        [GS, L]
│   │   │   ├── 24/7 paging owner per prod workload                                     [YOU]
│   │   │   ├── Runbook per alert; no runbook → no page
│   │   │   ├── Deploys, canary steps, rotations visible on the incident timeline
│   │   │   ├── Mitigate first: abort canary, revert, fail over
│   │   │   └── Platform on-call + tested break-glass write path when Argo CD is the incident
│   │   └── Failures: nobody paged; fix can't ship because the platform is down [OUTAGE]
│   │
│   ├── 4.11 SCALE & CAPACITY
│   │   └── One replica owner; HPA/VPA separation; drainable PDBs; topology-aware storage
│   │                                                                 (V-06, V-12, V-13, V-14)
│   │
│   ├── 4.12 BACKUP & RESTORE — configurable above floor
│   │   ├── Floor                                                                        [GS]
│   │   │   ├── Backup for GKE plans include Secrets and volume data                    [C V-33]
│   │   │   ├── Backups stored in a second region; retention locked                     [C V-33]
│   │   │   ├── Databases: application-consistent method per data store                 [V-27, L]
│   │   │   ├── Restore drills on schedule, measured against RTO/RPO                    [BLOCKER B-2]
│   │   │   └── Backups protected like secrets (they contain them)                      [L]
│   │   ├── Edge cases
│   │   │   ├── Restore needs an existing target cluster; it isn't created for you       [C V-33]
│   │   │   ├── Cross-region backup storage adds transfer cost                         [C V-33]
│   │   │   ├── Restore reactivates since-revoked credentials                           [L]
│   │   │   └── Restored workloads reconnect to live dependencies                       [L]
│   │   └── Failures: restore never tested [LOSS]; inconsistent DB restore [CORRUPT]
│   │
│   ├── 4.13 DR & MULTI-CLUSTER — configurable above floor
│   │   ├── Floor                                                                        [GS, L]
│   │   │   ├── Standby cluster pre-created via Cluster API with the same baseline (§4.1)
│   │   │   ├── Restore plans pre-defined targeting that cluster (V-33)
│   │   │   ├── KMS keys, registry mirror, certs, DNS usable in DR region independently
│   │   │   ├── One writer per data set during failover (fencing)
│   │   │   └── Failover and failback drilled
│   │   ├── Topology per workload class: active-passive | active-active             [YOU: configurable]
│   │   └── Failures: DR drifted; split brain [CORRUPT]; failback loses data [LOSS]
│   │
│   ├── 4.14 UPGRADE & MAINTENANCE
│   │   ├── Release channel drives cadence; upgrades staged by cluster ring             [C V-28, L]
│   │   ├── Removed APIs scanned; PDBs drainable; Pod Security pinned        (V-04, V-06, V-09)
│   │   └── Exec RBAC and mesh behavior re-tested every upgrade                        [V-24, L]
│   │
│   ├── 4.15 OPERATORS & CRDs (platform-only)                                           [YOU]
│   │   ├── Rules
│   │   │   ├── Platform team installs, upgrades, and removes all operators and CRDs
│   │   │   ├── Operators upgraded before CRDs they serve are removed; never the reverse [V-05]
│   │   │   └── Admission blocks tenant-created CRDs and cluster-scoped objects          [L]
│   │   └── Failures
│   │       ├── Operator removed before its custom resources → stuck namespaces [OUTAGE] (V-05)
│   │       └── CRD version removed under live resources [OUTAGE] → upgrade gate (V-09)
│   │
│   ├── 4.16 PEOPLE LIFECYCLE
│   │   ├── Rules
│   │   │   ├── Access granted only via IdP groups → leaving a group revokes access      [C V-28]
│   │   │   ├── JIT grants expire automatically                                         [YOU + L]
│   │   │   └── Team reorg → ownership transferred before old group is deleted          [L]
│   │   └── Failures: leaver retains access [SECURITY]; workloads with no owner [COMPLIANCE]
│   │
│   ├── 4.17 COST & CHARGEBACK
│   │   └── Mandatory owner/cost-center labels; orphan sweep; audit-log and backup storage tracked
│   │
│   ├── 4.18 DECOMMISSION — configurable above floor
│   │   ├── Floor: no data deleted without recorded decision; retention ≥ regime    [GS, BLOCKER B-1]
│   │   ├── Order: traffic → workload → bindings → storage → identity → DNS → audit record
│   │   └── Edge: stuck namespaces; LB finalizers; locked backups outlive the workload (V-05, V-08, V-33)
│   │
│   └── 4.19 GPU & BATCH — deferred                                                    [GS]
│       └── Re-opened only on request: dedicated node pools, queueing, quota per team
│
├── 5. PLATFORM-WIDE INVARIANTS
│   ├── One writer per object (Argo CD), one owner per field
│   ├── Prod guardrails fail closed                                                    (V-01, V-02)
│   ├── Everything traceable across all three APIs by one request ID
│   ├── Every automated path has a tested manual fallback
│   └── "Done" means observed in the running system, not accepted by an API
│
├── 6. CONFIGURABLE-VALUE MODEL (your "configurable" answers)
│   ├── Every configurable item has three platform-owned values: floor, default, ceiling
│   ├── Tenants can only tighten between floor and ceiling; never go below the floor
│   ├── Floors come from the regime (B-1) and business RTO/RPO (B-2)
│   ├── Applies to: image rules, backup, DR topology, session length, retention, canary steps
│   └── Why observability + on-call aren't deferred: "keep all logs" (your compliance answer)
│       can't be met if logs aren't shipped from day one, and a 24/7 system needs someone paged
│
├── 7. KPIs
│   ├── Lead time per API; self-serve rate
│   ├── Canary abort rate; change failure rate; time to roll back; MTTR
│   ├── Cert-expiry incidents (target zero); rotation success rate
│   ├── Restore drill pass rate; measured RTO/RPO vs B-2
│   ├── Binary Authorization violations in running pods (target zero)
│   ├── Standing privileged access (target zero); JIT grants expired on time
│   └── Audit-log routing test pass rate; orphaned spend
│
└── 8. GO-LIVE GATES
    ├── Every cluster passes the hardened-baseline scan (§4.1)
    ├── Admission down in each mode; Argo CD down → break-glass path works
    ├── Unsigned image rejected; breakglass deploy detected by continuous validation
    ├── Canary with injected errors aborts automatically
    ├── Secret rotation with env-var consumers; cert renewal with failing issuer
    ├── Exec/debug with every role on the next GKE version
    ├── Restore into the pre-created DR cluster, data validated; failover + failback
    ├── Audit evidence retrievable for a sampled request end to end
    └── Decommission with a missing operator; locked backups retained correctly
```

**Sources for the new tags.** V-01 to V-27 carry over from v2 and v3.

- **V-28 GKE hardening:**
  - [Certain] Google's hardening guide maps CIS GKE recommendations to Shielded GKE Nodes, Workload Identity Federation as the recommended way to authenticate to Google Cloud APIs, and private nodes.
  - [Certain] Google's GKE policy automation set also checks Google Groups for RBAC, Kubernetes secrets encryption, release channel enrollment, and a private control-plane endpoint.
- **V-29 Binary Authorization:**
  - [Certain] It enforces policy for attempted deployments and can continuously validate workloads running on GKE.
  - [Certain] When the enforcement policy is updated, only images deployed afterwards are validated; running Pods aren't affected.
  - [Certain] Breakglass deploys bypass enforcement and log a single audit event, while continuous validation keeps logging the violating Pods.
- **V-30 Cloud Service Mesh:**
  - [Certain] The managed control plane lists Certificate Authority Service support and mTLS STRICT mode.
  - [Certain] It must not be deployed in a cluster that also runs OSS Istio or an in-cluster control plane.
  - [Certain] The EnvoyFilter and WasmPlugin APIs are unsupported.
- **V-31 Argo Rollouts:** [Certain] It integrates with service meshes such as Istio to shift traffic gradually, and queries metrics to drive automated promotion or rollback.
- **V-32 Cloud Logging:**
  - [Certain] Admin Activity audit logs go to the _Required bucket with a fixed 400-day retention. Data Access audit logs land in the _Default bucket, which keeps entries for 30 days unless configured. User-defined buckets can be locked against updates.
  - [Certain] Data Access logs are disabled by default for most services.
- **V-33 Backup for GKE:**
  - [Certain] You must create the target cluster before a restore plan; the service doesn't create it during a restore. Backups stored in a different region incur outbound transfer charges.
  - [Certain] Backup plans can retain backups and lock them for a set period after creation.
- **V-34 GKE Secrets encryption:** [Certain] GKE encrypts Secrets at rest by default, but application-layer Secrets encryption with a customer-managed key is disabled by default. Google recommends separate projects for the key and the cluster.

**What closes the tree:** answer B-1 (jurisdiction and regime), B-2 (RTO/RPO numbers), B-3 (Standard or Autopilot, cluster and region count) and B-4 (internal or external tenants). Every floor in §6 then becomes a real number, and nothing in the tree is defaulted on your behalf. Nothing has been changed in any codebase.