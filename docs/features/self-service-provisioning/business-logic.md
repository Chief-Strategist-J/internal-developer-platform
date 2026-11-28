Several invariants in my last tree were stated but not mechanized, and the most important one breaks on real cloud resources. SP-INV-02 ("exactly once via deterministic names") only works when you choose the resource's name.

- **Service-generated IDs:** many Google Cloud resources get an ID assigned by the service. Config Connector can only re-acquire those through the separately recorded `resourceID` field, not by name.
- **Retried steps:** durable workflow engines retry steps at least once, and by default indefinitely.

Together, a step that creates a resource with a server-assigned ID, then crashes before recording that ID, will create a second one on retry. Naming discipline can't prevent it. The fix is mechanical: record the operation ID before polling, label every resource with the request ID, and look up by label before any create.

Below are 15 invariants and 15 edge cases. Each one has:

- the mechanism that enforces it,
- the signal that detects a breach,
- the production runbook,
- the drill that proves it.

```
SELF-SERVICE PROVISIONING — CRITICAL INVARIANTS, EDGE CASES & PRODUCTION HANDLING
Legend : [C V-xx] verified (V-94, V-95 below; others earlier in thread)   [L] inference — verify
         cfg.* configurable with named owner   SEV per incident matrix   PAGE / TICKET
Leaf   : Why │ Enforce │ Detect │ Production handling (runbook) │ Proof (drill)
│
├── A. INVARIANTS (15)
│   │
│   ├── INV-01 INTENT BEFORE EFFECT
│   │   ├── Statement: no external side effect without a persisted intent row {request, step, attempt,
│   │   │   target type, lookup labels}; outcome {resource ID, operation ID} persisted immediately after
│   │   ├── Why: crash between effect and record = invisible resource
│   │   ├── Enforce: step SDK refuses to call downstream without intent row ID; intent row ID sent as
│   │   │   request label/idempotency key downstream
│   │   ├── Detect: sweeper — resources labeled with request IDs but no outcome row (target 0)
│   │   ├── Production: PAGE platform if > 0 for data-bearing classes, else TICKET
│   │   │   └── Runbook: adopt into ledger (record ID) → reattach to request → resume or compensate
│   │   └── Proof: kill worker between effect and outcome write → resume adopts, no duplicate
│   │
│   ├── INV-02 EXACTLY-ONCE EFFECT PER STEP
│   │   ├── Statement: retrying a step never creates a second resource
│   │   ├── Why: activities execute at least once; default retries are unbounded          [C V-94]
│   │   │   and service-generated IDs defeat name-based dedupe                         [C V-91]
│   │   ├── Enforce (in order): 1) downstream idempotency key if supported; 2) lookup by request
│   │   │   labels before create; 3) persist long-running operation ID before polling; 4) deterministic
│   │   │   names where you choose the name
│   │   ├── Detect: duplicate scan — >1 resource per (request, step, type)
│   │   ├── Production: PAGE (COST/CORRUPT risk) → freeze step type → keep oldest, quarantine others →
│   │   │   fix lookup → resume
│   │   └── Proof: inject timeout after successful create on every step type
│   │
│   ├── INV-03 FENCED ATTEMPTS
│   │   ├── Statement: only the current attempt may write outcomes; stale attempts are rejected
│   │   ├── Why: TTL locks don't stop a paused holder from acting after expiry            [C V-90]
│   │   ├── Enforce: attempt counter = fencing token; ledger UPDATE … WHERE attempt = :mine
│   │   ├── Detect: stale-outcome rejections metric (non-zero is normal under chaos; spikes are not)
│   │   ├── Production: TICKET on spike → check worker pauses / clock / DB latency
│   │   └── Proof: pause worker (SIGSTOP) past lease; second attempt proceeds; first is rejected
│   │
│   ├── INV-04 RESERVE BEFORE ACT
│   │   ├── Statement: names, quota units, IP blocks, budget reserved atomically before step 1
│   │   ├── Why: discovering exhaustion mid-DAG leaves partial state
│   │   ├── Enforce: single transaction over reservation tables; reservation expiry cfg.reserve.ttl;
│   │   │   live cloud quota read before commit                                        [L]
│   │   ├── Detect: steps failing with quota/IP errors after reservation (target 0)
│   │   ├── Production: TICKET → reconcile reservation model with real limits
│   │   └── Proof: request just above remaining headroom → rejected before any effect
│   │
│   ├── INV-05 IP SPACE IS A FIRST-CLASS QUOTA
│   │   ├── Statement: provisioning never pushes a cluster's node/pod ranges past cfg.ip.max_util
│   │   ├── Why: GKE reserves a node's whole pod CIDR regardless of pods; high initial node counts keep
│   │   │   reserving after scale-down; extra pod ranges only apply to new node pools  [C V-92]
│   │   ├── Enforce: capacity service models per-node CIDR blocks per node pool; per-tenant ceilings
│   │   ├── Detect: range utilization per cluster/tenant
│   │   ├── Production: PAGE at cfg.ip.page_threshold → stop new node-pool-affecting requests →
│   │   │   add pod range + new node pool (planned, not emergency) → migrate
│   │   └── Proof: simulate near-full range → requests rejected with capacity reason
│   │
│   ├── INV-06 DATA-SAFE COMPENSATION
│   │   ├── Statement: no compensation, prune, TTL or decommission deletes a data-bearing resource
│   │   │   without a recorded retention decision
│   │   ├── Why: default deletion policy deletes the cloud resource with the object; Argo CD can trigger it [C V-91]
│   │   ├── Enforce: admission rule — data classes require deletion-policy abandon + Delete/Prune=confirm [C V-38]
│   │   ├── Detect: cloud audit log deletes of data classes vs retention decisions
│   │   ├── Production: PAGE SEV-1 on mismatch → restore per DR tree → incident review
│   │   └── Proof: compensate a failed request containing a database → database retained, owner notified
│   │
│   ├── INV-07 NO DANGLING NAMES
│   │   ├── Statement: DNS records removed before their targets; released names enter cooldown
│   │   ├── Why: attackers scan for CNAMEs to deleted targets; resolve-checks miss already-claimed targets [C V-93]
│   │   ├── Enforce: teardown DAG order fixed (DNS → LB/endpoint → resource); ledger cooldown table
│   │   ├── Detect: ownership scan (record target ∈ your inventory), not "does it resolve"   [C V-93]
│   │   ├── Production: PAGE security → remove record → check for takeover content → rotate cookies/
│   │   │   sessions for that domain if claimed
│   │   └── Proof: decommission drill shows DNS removed first
│   │
│   ├── INV-08 SINGLE MUTATION PATH
│   │   ├── Statement: orchestrator changes infrastructure only via owning APIs → Git → Argo CD
│   │   ├── Why: direct writes bypass policy, audit and reconciliation (and get reverted)
│   │   ├── Enforce: orchestrator identity has no cloud/cluster write IAM; only API + Git bot rights
│   │   ├── Detect: IAM scan; cloud audit logs with orchestrator principal as writer (target 0)
│   │   ├── Production: PAGE security on any direct write → revoke → investigate
│   │   └── Proof: attempt direct write with orchestrator identity → denied
│   │
│   ├── INV-09 APPROVAL INTEGRITY
│   │   ├── Statement: approver ≠ requester; approval bound to plan hash + expiry; aggregate budget
│   │   │   thresholds per tenant per period
│   │   ├── Why: self-approval and split requests bypass governance
│   │   ├── Enforce: approval service checks identity, hash, expiry; aggregate counters in ledger
│   │   ├── Detect: self-approval attempts; split-request pattern report
│   │   ├── Production: TICKET security for patterns; reject at runtime
│   │   └── Proof: modify plan after approval → execution refuses
│   │
│   ├── INV-10 PLAN PINNING
│   │   ├── Statement: execution uses exactly the template version + policy SHA in the approved plan
│   │   ├── Why: mid-flight template/policy changes produce unreviewed output
│   │   ├── Enforce: plan hash includes template SHA, policy SHA, sizing, names
│   │   ├── Detect: execution-time hash mismatch (hard stop)
│   │   ├── Production: auto re-plan; requester notified; approvals re-requested if material
│   │   └── Proof: publish template v+1 during a running saga → saga completes on v; upgrade PR later
│   │
│   ├── INV-11 BOUNDED RETRIES AND BACKPRESSURE
│   │   ├── Statement: every step has finite attempts, classified non-retryable errors, and per-downstream
│   │   │   concurrency limits
│   │   ├── Why: default activity retries are unbounded; infinite retries without circuit breaking
│   │   │   are a known anti-pattern                                                     [C V-94]
│   │   │   Git hosts cap content creation and concurrency; continuing while limited risks a ban [C V-95]
│   │   ├── Enforce: retry policy per step type (cfg.retry.*); token bucket per downstream; honor
│   │   │   retry-after / reset headers                                                   [C V-95]
│   │   ├── Detect: retries per step; 429/403-rate-limit counts per downstream
│   │   ├── Production: TICKET → queue drains; PAGE if a downstream stays limited > cfg.ratelimit.page
│   │   └── Proof: inject 429s for a sustained period → queue grows, no ban-level retry storm
│   │
│   ├── INV-12 OBSERVED READY
│   │   ├── Statement: READY only after the first deploy is observed healthy
│   │   ├── Why: accepted objects ≠ running service                                      [C V-03]
│   │   ├── Enforce: final step polls owning API status (available replicas, canary state)
│   │   ├── Detect: READY requests with zero available replicas (target 0)
│   │   ├── Production: TICKET owner + platform; classify reason (quota/policy/image/sync)
│   │   └── Proof: request with image that fails admission → never READY, reason shown
│   │
│   ├── INV-13 NO SECRETS CREATED OR INJECTED BY PROVISIONING
│   │   ├── Statement: CI and workloads use workload identity; provisioning never writes secret values
│   │   ├── Enforce: CI wiring via federation only; secret scans on rendered repo before first push
│   │   ├── Detect: secret scanner findings in new repos (target 0)
│   │   ├── Production: PAGE security → rotate → purge history per policy → template fix
│   │   └── Proof: template with embedded token → publish gate rejects   (templates tree)
│   │
│   ├── INV-14 FULL TRACEABILITY
│   │   ├── Statement: every resource carries request ID, owner, cost center; every action carries the
│   │   │   end user's identity
│   │   ├── Enforce: label injection at owning APIs; user token propagated (exchanged per audience) [C V-77]
│   │   ├── Detect: unlabeled resources; actions attributed only to service identities
│   │   ├── Production: TICKET → backfill labels from ledger; fix emitting step
│   │   └── Proof: sampled resource → trace to request, approver, user in < cfg.audit.trace_time
│   │
│   └── INV-15 TENANT CAPACITY ISOLATION
│       ├── Statement: no tenant can consume more than its ceiling of shared scarce resources
│       │   (IP ranges, projects, clusters, node pools, Git creation budget, workflow concurrency)
│       ├── Enforce: per-tenant ceilings in reservations; per-tenant workflow concurrency
│       ├── Detect: tenant share vs ceiling dashboards
│       ├── Production: auto-reject above ceiling with reason; capacity review ticket
│       └── Proof: one tenant floods requests → others' requests still progress
│
├── B. EDGE CASES (15)
│   │
│   ├── EC-01 Timeout after successful create (server-assigned ID unknown)
│   │   ├── Decision: on retry → lookup by request labels → found? adopt : create
│   │   ├── Production: duplicate scan alert if lookup misses (label propagation lag) [C V-91, V-94]
│   │   └── Test: inject network drop after 200 OK on each provider
│   │
│   ├── EC-02 Double submit / client retry
│   │   ├── Decision: same Idempotency-Key + same payload → existing operation; different payload → 422
│   │   ├── Production: no alert; metric of dedup hits
│   │   └── Test: concurrent identical POSTs → one operation
│   │
│   ├── EC-03 Two tenants race for the same name
│   │   ├── Decision: unique constraint on normalized name at reservation → loser gets 409
│   │   ├── Production: none; UX suggests alternatives
│   │   └── Test: parallel reservations
│   │
│   ├── EC-04 Downstream rate-limited (Git host, cloud APIs)
│   │   ├── Decision: retry-after / reset headers → wait; no header → wait ≥ provider guidance with
│   │   │   exponential backoff, then fail step after cfg.retry.max_attempts                [C V-95]
│   │   ├── Production: queue depth alert; per-downstream concurrency auto-throttle
│   │   └── Test: sustained 429 injection; also "secondary limit with no status endpoint" case [C V-95]
│   │
│   ├── EC-05 Cloud quota fine at reservation, exhausted at execution (other consumers)
│   │   ├── Decision: step fails retryable → backoff → after cfg.quota.wait fail request + compensate
│   │   ├── Production: TICKET capacity; quota increase request path
│   │   └── Test: shrink quota between reservation and execution
│   │
│   ├── EC-06 Zone capacity stockout during node pool creation
│   │   ├── Decision: try allowed alternate zones/families per compute class; else fail with reason
│   │   ├── Production: TICKET; consider reservations for that tier                     [C V-83]
│   │   └── Test: simulate resource-availability errors
│   │
│   ├── EC-07 Compensation fails (e.g. resource has deletion protection)
│   │   ├── Decision: retry bounded → ORPHANED_DIRTY with resource list
│   │   ├── Production: PAGE platform; runbook: manual completion with two-person approval; ledger updated
│   │   └── Test: enable deletion protection on a test resource before compensation
│   │
│   ├── EC-08 Failure after point of no return (repo pushed, public DNS live, cert issued)
│   │   ├── Decision: forward-complete preferred; else compensate per class (archive repo, DNS first)
│   │   ├── Production: TICKET owner with state summary
│   │   └── Test: fail S8 after S7
│   │
│   ├── EC-09 Template or policy updated while saga runs
│   │   ├── Decision: plan pinned (INV-10); new version only via upgrade PR afterwards
│   │   ├── Production: none if pinned; PAGE if hash mismatch detected at execution (bug)
│   │   └── Test: publish during saga
│   │
│   ├── EC-10 Policy promoted to DENY makes template output non-compliant mid-saga
│   │   ├── Decision: S5/S12 fail with rule IDs → template fix → resume from failed step
│   │   ├── Production: TICKET platform (template conformance gap)
│   │   └── Test: tighten a rule in nonprod during saga
│   │
│   ├── EC-11 Owner group deleted or reorganized mid-saga
│   │   ├── Decision: pause before ownership-bearing steps; require reassignment or cancel
│   │   ├── Production: TICKET requester + platform
│   │   └── Test: delete group in test IdP during saga
│   │
│   ├── EC-12 Argo CD sync fails after commits (Git says yes, cluster says no)
│   │   ├── Decision: final READY gate fails; reason from Argo CD; no automatic retry on same SHA [C V-35]
│   │   ├── Production: TICKET owner; fix-forward commit
│   │   └── Test: commit manifest that fails sync
│   │
│   ├── EC-13 Decommission requested while provisioning still running
│   │   ├── Decision: serialize per service key; decommission waits or cancels saga first
│   │   ├── Production: none; state machine enforces
│   │   └── Test: concurrent decommission + resume
│   │
│   ├── EC-14 Ephemeral environment TTL expires with a data-bearing resource attached
│   │   ├── Decision: TTL controller refuses (INV-06); escalates to owner
│   │   ├── Production: TICKET; ephemeral offerings exclude prod data classes
│   │   └── Test: attach data class to ephemeral env
│   │
│   └── EC-15 Workflow engine or ledger DB failover mid-saga
│       ├── Decision: workflow resumes from persisted history; fenced attempts prevent double effects
│       ├── Production: PAGE platform if stalls > cfg.saga.stall; resume runbook
│       └── Test: kill all workers + fail over DB; verify no duplicates            [C V-94]
│
└── C. PRODUCTION OPERATING MODEL
    ├── SLOs (targets cfg.*)
    │   ├── Request → READY latency p95 per offering
    │   ├── Success rate excluding user-caused rejections
    │   ├── ORPHANED_DIRTY open count (0) and age
    │   ├── Duplicates (0); untracked resources (0); dangling DNS (0)
    │   └── Rejections with actionable reason (100%)
    ├── Alerts → runbooks
    │   ├── PAGE platform: ORPHANED_DIRTY; duplicates; untracked data-bearing resource; IP utilization;
    │   │   saga stall; downstream limited beyond cfg.ratelimit.page
    │   ├── PAGE security: direct writes by orchestrator identity; data-class deletion without decision;
    │   │   dangling DNS; secrets in new repos
    │   └── TICKET: quota drift; approval anomalies; READY-without-replicas; template conformance gaps
    ├── Dashboards
    │   ├── Saga funnel by state and step; retries and stale-attempt rejections
    │   ├── Per-downstream rate-limit and latency
    │   └── Capacity: quota, IP ranges, tenant shares, reservations vs usage
    ├── Cadence
    │   ├── Daily: sweeper + DNS ownership + duplicate reports reviewed
    │   ├── Weekly: capacity review; ORPHANED_DIRTY burn-down
    │   ├── Monthly: game day covering the INV proofs and EC tests above
    │   └── Per release of orchestrator: replay/determinism tests and kill-worker test       [C V-94]
    └── Ownership
        ├── Platform: orchestrator, ledger, sweeper, capacity service, runbooks
        ├── Security: approval rules, deletion rules, DNS ownership scans, secret scans
        └── Owners: respond to TICKETs for their requests; retention decisions
```

**Sources (V-94, V-95; V-91 extended)**

- **V-94 Durable workflow semantics:**
  - [Certain] Temporal's guidance says activities execute at least once by default and should be idempotent. Activity retries are unlimited unless configured otherwise.
  - [Certain] Temporal's own pre-production checklist names non-idempotent activities and infinite retries without circuit breaking as anti-patterns. It recommends abruptly killing all workers to validate at-least-once behavior.
- **V-95 Git host limits (if D-SCM is GitHub):**
  - [Certain] GitHub generally allows no more than 80 content-creating requests per minute and 500 per hour, and some endpoints are lower. There is no way to check secondary-limit status.
  - [Certain] GitHub says to wait for the `retry-after` or reset time. Continuing to send requests while rate-limited can get an integration banned.
- **V-91 extended:** [Certain] Config Connector can only acquire resources with a service-generated ID through the `resourceID` field. Some resource types can't be acquired at all, which is why INV-02 relies on recorded IDs and label lookup rather than names.

The blockers from the previous tree are unchanged: D-IAC, D-WF, D-SCM, D-CI, D-TPL, D-BUDGET and B-1. D-WF now matters more, because INV-02, INV-03 and INV-11 depend on its retry and history semantics. Slices 1-3 (`INV-01`, `INV-03`, `INV-04`, `INV-05`, `INV-09`, `INV-10`, `INV-15`, `EC-02`, `EC-03`, `EC-09`) are implemented in Go + PostgreSQL migrations; `INV-02`, `INV-06`–`INV-08`, `INV-11`–`INV-14`, and remaining edge cases await those decisions.