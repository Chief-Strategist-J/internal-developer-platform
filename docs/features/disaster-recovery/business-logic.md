The template treats a region as the unit of disaster. The worst recent Google Cloud event wasn't regional.

On June 12, 2025, a bad policy change replicated globally within seconds and crashed Service Control, the system that authorizes every Google Cloud API request, in every region. API calls couldn't be authorized anywhere for hours, and Google couldn't even post to its status page for about an hour. A failover that works by calling cloud APIs (promote this database, change that DNS record, scale those nodes) would have failed exactly then.

The second uncomfortable truth is about your current dependencies. As designed so far, your platform has single-region trust anchors:

- Argo CD as the only writer
- Kubernetes Secrets
- regional KMS keys for encryption and Binary Authorization attestors
- regional registries
- one audit sink

Until each one has an independent copy in the DR region, you have backups, not disaster recovery.

So this tree treats three things as design requirements rather than nice-to-haves:

- **Static stability:** the DR side can serve without mutating the control plane.
- **A dependency map:** every dependency is duplicated or explicitly accepted as a single point.
- **Exactly one write authority per dataset.**

```
DISASTER RECOVERY — SINGLE COMPLETE TREE (internal platform, government, GKE)
Legend   : [YOU] decision   [GS] gold-standard default   BLOCKER   [C V-xx] verified (V-80..V-89 below;
           earlier V-ids in thread)   [L] inference — verify   cfg.* configurable + owner (no invented numbers)
Failure  : Scenario → Impact → Handling → Detection
│
├── 0. DECISIONS
│   ├── Settled
│   │   ├── GKE only; Argo CD single writer; Backup for GKE; role-based approvals; 24/7 paging [YOU]
│   │   ├── DR topology and RTO/RPO configurable within floors per tier                    [YOU]
│   │   └── Cluster API creates DR clusters to the same hardened baseline                  [GS]
│   └── BLOCKERS
│       ├── B-1   Residency: both regions must satisfy it (in-country pair? key custody?)
│       ├── B-2   RTO/RPO floors per tier — for workloads AND for the platform itself
│       ├── D-DATA Data stores per tier (Cloud SQL / AlloyDB / Spanner / self-managed on GKE)
│       ├── D-SEC Secret delivery source (must have a DR-region copy)
│       ├── D-DRTOP Per tier: active-active | warm standby | pilot light | restore-to-new
│       └── D-DECLARE Who may declare a disaster; manual vs automatic per tier
│
├── 1. PRINCIPLES
│   ├── DR-P1 Disaster scopes: zone │ region │ provider control plane (global) │ data corruption │
│   │         ransomware/insider │ trust-anchor compromise — each has its own playbook     [C V-80]
│   ├── DR-P2 Static stability: DR side serves traffic without needing control-plane mutations
│   │         during failover where possible (pre-provisioned LBs, DNS policies, warm capacity) [C V-80, L]
│   ├── DR-P3 Exactly one write authority per dataset, enforced by fencing          [C V-81]
│   ├── DR-P4 Failover may be automatic for stateless tiers; failback is never automatic for stateful [C V-85]
│   ├── DR-P5 Every dependency has a DR-region copy or is an accepted, documented single point
│   ├── DR-P6 Security never weakens in DR (no admission bypass, no fail-open policies)
│   └── DR-P7 DR exists only if a drill proved it within cfg.drill.interval
│
├── 2. DEPENDENCY MAP (each must be ready in DR region before "READY")
│   ├── Compute: DR GKE cluster(s) pre-created (restore never creates clusters)        [C V-33, V-84]
│   ├── Capacity: reservations for tier-critical node pools in DR zones                [C V-83]
│   ├── Images: registry reachable from DR (regional mirror or multi-region repo — note a
│   │   multi-region repo failure affects all its users)                               [C V-87]
│   ├── Admission: Binary Authorization attestor keys usable in DR; DR policy identical [C V-88, L]
│   ├── Keys: KMS keys for DR data, backups, attestations exist in DR-allowed locations;
│   │   keys can't be moved after creation                                             [C V-82, V-88]
│   ├── Secrets: D-SEC store has DR copy; rotation reaches both regions
│   ├── GitOps: Argo CD instance able to reconcile DR clusters without the primary region [L]
│   ├── Data: per D-DATA replication with measurable lag                              [C V-81]
│   ├── Traffic: DNS routing policy + health checks + backup load balancers pre-created [C V-85]
│   ├── Certificates: valid for DR hostnames; same TLS config where endpoints move    [C V-81]
│   ├── Identity: IdP reachable; break-glass identities independent of portal and primary region
│   ├── Observability + audit: sinks writable during primary-region loss              [C V-32, L]
│   └── Runbooks + tooling: executable without portal, without primary region; credentials tested
│
├── 3. INVARIANTS (12)
│   ├── DR-INV-01 SINGLE WRITE AUTHORITY
│   │   ├── Each dataset has exactly one writable primary at any time
│   │   ├── Why: Google documents that after a cross-region promotion, clients may write to the
│   │   │   original primary (even accidentally) once the failed region returns               [C V-81]
│   │   ├── Enforce: fence old primary (revoke write path: IAM, network, DB role) BEFORE promotion;
│   │   │   fencing epoch recorded; apps reject stale-epoch writes where supported          [L]
│   │   └── Detect: write-endpoint resolution check + per-region write metrics (two writers = PAGE)
│   ├── DR-INV-02 NO AUTOMATIC FAILBACK FOR STATEFUL PATHS
│   │   ├── Why: DNS failover policies fail back automatically when health checks pass     [C V-85]
│   │   ├── Enforce: stateful endpoints use manual/locked routing after failover; failback only via
│   │   │   failback API with reconciliation proof
│   │   └── Detect: routing-policy diff alert during an active DR event
│   ├── DR-INV-03 DEPENDENCY INDEPENDENCE
│   │   ├── Every item in §2 has a DR copy or an accepted single-point record
│   │   ├── Enforce: readiness check fails if any dependency resolves to the primary region
│   │   └── Detect: daily dependency probe from DR region
│   ├── DR-INV-04 RESTORABLE BACKUPS
│   │   ├── Backups restored and data-validated within cfg.restore.drill_interval; keys available
│   │   ├── Why: CMEK keys for backup artifacts must be in the BackupPlan's region; data under a
│   │   │   disabled or destroyed key becomes inaccessible                                  [C V-82]
│   │   ├── Enforce: restore drill gate; key-state monitoring; destruction protection
│   │   └── Detect: last-successful-restore age; key state change alerts
│   ├── DR-INV-05 GUARANTEED CAPACITY
│   │   ├── Tier-critical workloads have reserved DR capacity, or a documented shed plan
│   │   ├── Why: zones can lack capacity; reservations exist to hold it, billed while held [C V-83]
│   │   └── Detect: reservation coverage vs DR plan; stockout errors in drills
│   ├── DR-INV-06 DR CONFORMANCE
│   │   ├── DR clusters match primary on versions, policies, admission, Pod Security, quotas, certs
│   │   ├── Enforce: same Cluster API baseline + Argo CD apps + posture scan
│   │   └── Detect: continuous config diff; any drift = DR status NOT_READY
│   ├── DR-INV-07 DELIBERATE DECLARATION
│   │   ├── Stateful failover requires two-person declaration (D-DECLARE); auto only for tiers marked so
│   │   ├── Detection signal from ≥2 independent vantage points (Google LB failover uses three
│   │   │   source regions and fails over when two or more fail)                         [C V-85]
│   │   └── Detect: declaration audit; auto-failover count per tier
│   ├── DR-INV-08 MEASURED RTO/RPO
│   │   ├── Every drill records actual RTO and RPO per tier vs B-2 floors
│   │   ├── Failing tier → NOT_READY + engineering defect (never "pass with notes")
│   │   └── Detect: drill report vs targets
│   ├── DR-INV-09 AUDIT CONTINUITY
│   │   ├── Every DR action is logged to a sink that survives primary-region loss
│   │   └── Detect: sink write test from DR region during drills
│   ├── DR-INV-10 STATIC STABILITY FOR TIER-0
│   │   ├── Tier-0 failover path requires no new resource creation in the critical window
│   │   ├── Why: global API authorization failure blocked API calls across regions (2025)  [C V-80]
│   │   └── Detect: drill with control-plane mutations blocked (simulated)
│   ├── DR-INV-11 RESIDENCY IN BOTH REGIONS
│   │   ├── DR region, backup locations, key locations, log buckets all satisfy B-1
│   │   └── Detect: location audit of every DR resource
│   └── DR-INV-12 NO SECURITY DOWNGRADE IN DR
│       ├── Admission, policies, mTLS, signing behave identically in DR
│       ├── Break-glass only via audited path with expiry
│       └── Detect: admission/policy config diff; breakglass events
│
├── 4. LIFECYCLE STATE MACHINE (per DR plan = tier + dataset set)
│   ├── NOT_READY ⇄ READY (readiness check: §2 dependencies + DR-INV-03/04/05/06/08)
│   ├── READY → DECLARED (two-person) → FENCING → PROMOTING → SCALING → VALIDATING → ROUTING → FAILED_OVER
│   ├── Any step → ABORTED (only before PROMOTING; after promotion, forward only)
│   ├── FAILED_OVER → RECONCILING (reverse replication, divergence report)
│   ├── RECONCILING → FAILBACK_READY (lag 0, divergence resolved, approvals)
│   ├── FAILBACK_READY → FAILING_BACK → NORMAL (via switchover where supported — zero data loss) [C V-81]
│   └── DRILL_* mirrors the same states with synthetic traffic and production-safe guards
│
├── 5. DR CONTROL API (platform-owned; separate from Workload/Cluster/Cloud APIs)
│   ├── Cross-cutting requirements
│   │   ├── Hosted in BOTH regions; works if either region is lost                      [L]
│   │   ├── Static-stable: reads from local state; writes queued if provider APIs unavailable
│   │   ├── AuthN: IdP + independent break-glass identity; step-up for every mutation
│   │   ├── AuthZ: role per action; two-person approval for declare / failover / failback / fence
│   │   ├── Idempotency key on every mutation; operation IDs; 202 + polling for long operations
│   │   ├── Every call audited to the surviving sink (DR-INV-09)
│   │   └── Errors are explicit: never "success" for a step that wasn't verified
│   │
│   ├── PLANS & READINESS
│   │   ├── POST /v1/dr/plans                 → create DR plan (tier, datasets, topology, targets)
│   │   │   └── Rejects targets below B-2 floors; requires dependency map entries
│   │   ├── GET  /v1/dr/plans/{plan}          → plan + current state + last drill result
│   │   ├── POST /v1/dr/plans/{plan}:checkReadiness
│   │   │   └── Runs §2 probes from DR region; returns per-dependency status; any fail = NOT_READY
│   │   └── GET  /v1/dr/plans/{plan}/rpo      → live replication lag per dataset (source + timestamp)
│   │
│   ├── DECLARATION & FAILOVER
│   │   ├── POST /v1/dr/incidents:declare     → scope (zone/region/provider/corruption/compromise),
│   │   │   evidence links, declarer; second approver required to become DECLARED
│   │   ├── POST /v1/dr/failovers             → {plan, incident, mode: switchover|failover}
│   │   │   ├── Precondition: plan READY or explicit "degraded accepted" with approvals
│   │   │   ├── Returns projected RPO (current lag) — approvers must acknowledge data-loss window [C V-81]
│   │   │   └── Mode switchover only when primary reachable (zero data loss)              [C V-81]
│   │   ├── POST /v1/dr/failovers/{op}:approve → second approver; step-up required
│   │   ├── POST /v1/dr/failovers/{op}:abort   → allowed only before PROMOTING
│   │   └── GET  /v1/dr/failovers/{op}         → state, per-step result, measured RTO so far
│   │
│   ├── FENCING
│   │   ├── POST /v1/dr/datasets/{ds}:fence   → revoke old-primary write paths; increments epoch
│   │   │   └── Must succeed (or be confirmed impossible to reach) before PROMOTING
│   │   └── GET  /v1/dr/datasets/{ds}/epoch   → current write authority + epoch
│   │
│   ├── RESTORE (corruption / ransomware / lost dataset)
│   │   ├── POST /v1/dr/restores              → {backup, target cluster (pre-created), namespaces,
│   │   │   conflict policy}; target must exist in RestorePlan region                    [C V-84]
│   │   └── GET  /v1/dr/restores/{op}         → progress + data validation results
│   │
│   ├── FAILBACK
│   │   ├── POST /v1/dr/failbacks             → requires reconciliation report (divergence = 0 or
│   │   │   signed-off), reverse replication lag 0, approvals; uses switchover where supported [C V-81]
│   │   └── GET  /v1/dr/failbacks/{op}
│   │
│   ├── DRILLS & EVIDENCE
│   │   ├── POST /v1/dr/drills                → {plan, scope, safety guards}; production-safe by default
│   │   ├── GET  /v1/dr/drills/{run}          → measured RTO/RPO vs targets; pass/fail
│   │   └── GET  /v1/dr/evidence/{run}        → immutable evidence bundle for auditors
│   │
│   ├── BREAK-GLASS
│   │   └── POST /v1/dr/breakglass:activate   → time-boxed elevated role; two-person; auto-expire;
│   │       never disables admission or policy (DR-INV-12)
│   │
│   └── Underlying provider operations the API orchestrates (verify per D-DATA)
│       ├── Cloud SQL Enterprise Plus: switchover (zero data loss) / replica failover (may lose data);
│       │   write endpoint re-points; SSL config must match for write endpoint              [C V-81]
│       ├── Backup for GKE: restore into pre-created cluster in RestorePlan region          [C V-84]
│       ├── Cloud DNS routing policies: failover to backup VIPs; health checks from 3 regions [C V-85]
│       ├── Regional external Application LBs as backup (no backend buckets)               [C V-85]
│       └── Compute reservations consumed by DR node pools / compute classes             [C V-83]
│
├── 6. CRITICAL FAILURE POINTS (20)
│   ├── CFP-01 Provider control plane down globally [OUTAGE]
│   │   └── API calls can't be authorized in any region → failover scripts fail → static stability
│   │       (pre-provisioned DR, DNS policies prebuilt), offline runbooks → drill with mutations
│   │       blocked                                                                       [C V-80]
│   ├── CFP-02 Split brain after primary returns [CORRUPT]
│   │   └── Clients write to old primary → fencing before promotion, write-endpoint only →
│   │       two-writer detection → PAGE                                                    [C V-81]
│   ├── CFP-03 Data loss on replica failover [LOSS]
│   │   └── Replication lag at promotion → projected RPO shown and acknowledged; switchover when
│   │       possible → lag SLI per dataset                                                [C V-81]
│   ├── CFP-04 Automatic failback before reconciliation [CORRUPT]
│   │   └── Health checks pass → DNS sends traffic back → DR-INV-02 locks routing → routing diff alert [C V-85]
│   ├── CFP-05 Encryption key unavailable or destroyed [LOSS]
│   │   └── Backups/disks unreadable → keys in DR-allowed locations, destruction protection,
│   │       key-state alerts → restore drill decrypts end to end                          [C V-82]
│   ├── CFP-06 No target cluster for restore [OUTAGE]
│   │   └── Restore can't start → DR clusters pre-created via Cluster API → readiness probe [C V-84]
│   ├── CFP-07 DR region capacity exhausted [OUTAGE]
│   │   └── Nodes never join; pods Pending; stockouts don't show as outages → reservations +
│   │       alternate machine families in compute classes + shed plan → drill scale-up test [C V-83, V-89]
│   ├── CFP-08 Images unavailable in DR [OUTAGE]
│   │   └── Registry in failed region → mirror per region or multi-region repo (shared failure
│   │       caveat) → pull test from DR nodes                                             [C V-87]
│   ├── CFP-09 Attestations can't be produced or verified in DR [OUTAGE | SECURITY]
│   │   └── Attestor/verifier keys or verifier service in failed region → fixes can't deploy →
│   │       DR-capable verifier + keys; never disable admission → deploy-in-DR drill      [C V-88, L]
│   ├── CFP-10 GitOps writer lost with primary region [OUTAGE]
│   │   └── DR clusters can't reconcile → Argo CD reachable from DR + break-glass apply path →
│   │       DR sync test                                                                  [L]
│   ├── CFP-11 Secrets missing or stale in DR [OUTAGE | SECURITY]
│   │   └── Apps can't start or use revoked creds → D-SEC store DR copy; rotation reaches both →
│   │       secret version parity check                                                   [L]
│   ├── CFP-12 Clients keep hitting dead region [OUTAGE]
│   │   └── Caching beyond TTL; hard-coded IPs → low TTLs set in advance; DNS-name-only clients;
│   │       global LB in front where possible → per-region request metrics after cutover [L]
│   ├── CFP-13 False failover from flapping/partial signals [OUTAGE | CORRUPT]
│   │   └── Single-vantage health check → multi-region checks (2 of 3) + human gate for stateful →
│   │       failover-trigger audit                                                         [C V-85]
│   ├── CFP-14 Partial failover (data moved, compute not, or reverse) [DEGRADE]
│   │   └── Cross-region chatty calls, timeouts → failover executed per plan as a unit; state
│   │       machine forbids split → per-tier topology check
│   ├── CFP-15 Backups exist but don't restore correctly [LOSS | CORRUPT]
│   │   └── Crash-consistent DB snapshots; missing objects → app-consistent methods; data validation
│   │       in drills → last-validated-restore age                                         [C V-33]
│   ├── CFP-16 DR cluster drifted from primary [OUTAGE]
│   │   └── Old versions, missing policies, BA config differs → admission denies in DR → continuous
│   │       conformance; drift = NOT_READY (DR-INV-06)
│   ├── CFP-17 Quota, IAM or endpoint config differs in DR project [OUTAGE]
│   │   └── Promotion or scaling refused; write endpoint fails on TLS mismatch → parity checks
│   │       incl. SSL config → readiness probe                                            [C V-81]
│   ├── CFP-18 Audit trail broken during DR [COMPLIANCE]
│   │   └── Sink in failed region → dual-region sinks → sink write test in drills
│   ├── CFP-19 DR tooling unusable when needed [OUTAGE]
│   │   └── Depends on portal, IdP, expired creds, primary-region runner → DR API in both regions,
│   │       break-glass identity, credential expiry monitoring → monthly tooling smoke test
│   └── CFP-20 Failback loses writes made in DR [LOSS]
│       └── No reverse replication or divergence check → reconciliation report mandatory;
│           switchover-based failback where supported → divergence = 0 gate              [C V-81]
│
├── 7. EDGE CASES (beyond the 20)
│   ├── EC-D01 Disaster is data corruption replicated to DR → restore point-in-time, not failover
│   ├── EC-D02 Ransomware/insider deletes backups → locked backups; separate admin domain      [C V-33]
│   ├── EC-D03 Primary returns mid-failover → fencing state wins; no automatic reversal
│   ├── EC-D04 Two incidents at once (region + security) → security playbook first; DR on clean images only
│   ├── EC-D05 DR region also degraded → tier-0 only (shed plan); tertiary only if pre-validated
│   ├── EC-D06 Global LB fails over to fewer regions → overload of remaining backends               [C V-85]
│   ├── EC-D07 Backend buckets behind global LB → regional backup LBs can't serve them             [C V-85]
│   ├── EC-D08 Long-lived connections (streams, gRPC) pinned to dead region → client reconnect logic
│   ├── EC-D09 Scheduled jobs run in both regions after failover → job leader tied to write authority
│   ├── EC-D10 Cached tokens/sessions tied to region → stateless sessions or replicated session store
│   ├── EC-D11 Residency forbids the only nearby DR region → DR design constrained by B-1 first
│   └── EC-D12 Drill accidentally affects production → drill guards: synthetic tenants, read-only
│              checks, abort switch
│
├── 8. PLAYBOOKS (by disaster scope)
│   ├── PB-ZONE   Regional clusters absorb; verify PDB-safe rescheduling; no DR declaration
│   ├── PB-REGION Declare → fence → promote → scale (reservations) → validate → route → monitor
│   ├── PB-GLOBAL Provider control plane down: rely on static stability; avoid mutations that
│   │             will fail; communicate; prepare but don't execute promotions that need APIs [C V-80]
│   ├── PB-CORRUPT Stop writes → identify last good point → restore to new namespace/cluster →
│   │             validate → cut over → forensic hold
│   └── PB-COMPROMISE Rotate trust anchors (keys, attestors, credentials) before restoring workloads
│
├── 9. PRODUCTION OPERATIONS
│   ├── SLIs (targets cfg.*, floors from B-2)
│   │   ├── Plans READY (%); dependency probe pass rate from DR region
│   │   ├── Replication lag per dataset vs RPO
│   │   ├── Last successful drill age; measured RTO/RPO per tier vs targets
│   │   ├── Last validated restore age; key-state anomalies (0)
│   │   ├── Reservation coverage vs DR plan
│   │   └── DR config drift count (0)
│   ├── Alerts
│   │   ├── PAGE: two writers detected; lag > RPO for tier-critical; key disabled/scheduled for destruction;
│   │   │   DR plan flips NOT_READY for tier-critical; routing change during active DR event
│   │   └── TICKET: drill overdue; reservation shortfall; tooling credential expiring
│   ├── Cadence: daily readiness probes; monthly tooling smoke test; per-tier drills within
│   │   cfg.drill.interval; annual full-region drill incl. failback
│   └── Ownership: platform (DR API, clusters, routing), data owners (replication, consistency),
│       security (keys, attestors, break-glass), incident command (declaration)
│
├── 10. ROADMAP (exit gates, no dates)
│   ├── PHASE 0 Dependency map + DR clusters + mirrored registry/keys/secrets + audit continuity
│   │   └── EXIT: readiness probe green from DR region for all §2 items
│   ├── PHASE 1 Backups + restore drills (data validated) + fencing for each dataset
│   │   └── EXIT: restore drill meets B-2 for tier-critical; two-writer test blocked
│   ├── PHASE 2 Failover orchestration (DR API) + DNS/LB failover + reservations
│   │   └── EXIT: region drill meets RTO/RPO; no security downgrade observed
│   └── PHASE 3 Failback with reconciliation + global-outage (static stability) drill
│       └── EXIT: full round trip with zero unreconciled divergence
│
└── 11. GO-LIVE GATES
    ├── B-1, B-2, D-DATA, D-SEC, D-DRTOP, D-DECLARE answered
    ├── All 12 invariants have enforcement + detection proven in a drill
    ├── All 20 CFPs have a tested handling path
    ├── DR API callable with primary region and portal unavailable
    └── Evidence bundle produced for at least one full drill
```

**Sources (V-80 to V-89)**

- **V-80 June 2025 global outage:**
  - [Certain] A policy change with blank fields replicated globally within seconds and crashed Service Control in every region. Google posted its first report about an hour in, because its status infrastructure was also down.
  - [Certain] Analyses describe Service Control as the authorization engine for every API request. A crash there meant control planes couldn't authorize any request, regardless of service.
- **V-81 Cloud SQL DR:**
  - [Certain] After a cross-region promotion, clients might read and write the original primary, even by accident, once the old region returns.
  - [Certain] Replica failover is immediate and can lose data because of replication lag.
  - [Certain] Switchover reverses roles with zero data loss and is also usable for DR tests.
  - [Certain] With a DNS write endpoint, the primary and DR replica must share the same SSL configuration.
  - [Certain] The REST endpoint for switchover is `POST https://sqladmin.googleapis.com/sql/v1beta4/projects/PROJECT_ID/instances/REPLICA_NAME/switchover`, and replica failover is invoked with `promote-replica --failover`.
- **V-82 Keys and backups:**
  - [Certain] A CMEK key registered for Backup for GKE artifacts must be in the same region as the BackupPlan.
  - [Certain] Data protected by a disabled or destroyed CMEK key becomes inaccessible.
- **V-83 Capacity:** [Certain] Reservations hold capacity and are billed at on-demand rates while they exist, with disaster recovery listed as a use case. GKE can consume them through node pools, workload selectors or custom compute classes.
- **V-84 Restore target:** [Certain] A RestorePlan's target cluster must already be a zonal or regional cluster in the RestorePlan's region.
- **V-85 Traffic failover:**
  - [Certain] Cloud DNS failover uses health checks from three source regions and fails over when two or more fail.
  - [Certain] Failback is automatic once health checks pass again.
  - [Certain] Regional backup load balancers don't support Cloud Storage backend buckets.
- **V-86:** reserved (unused).
- **V-87 Registry:** [Certain] A multi-region repository's operational failure affects all its users regardless of region, so multi-region isn't automatically independent.
- **V-88 KMS:**
  - [Certain] Key material is confined to the chosen location, and a key can't be moved after creation, so DR key placement must be decided upfront.
  - [Certain] Multi-region KMS locations don't need cross-datacenter consensus for encrypt and decrypt operations.
- **V-89 Stockouts:** [Likely] A FinOps vendor notes that capacity stockouts don't appear on status pages and surface first as Pending pods and nodes that never join.

**Six decisions block this tree:** B-1 (residency), B-2 (RTO/RPO numbers), D-DATA (which data stores per tier), D-SEC (secret source with a DR copy), D-DRTOP (topology per tier) and D-DECLARE (who declares a disaster). B-1 comes first, because residency may restrict which region pairs and key locations are even legal. No codebase changes have been made.