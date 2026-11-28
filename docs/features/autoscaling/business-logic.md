# Autoscaling — Pods, Nodes, Databases, Serverless and Ingestion

Nov 28, 2026 · @Chief

## Pending selections

Thirteen autoscaling choices are open. Four of them carry the most risk: D-SPOT (what may be preempted), D-POOL (database connections under scale-out), D-INGEST (which queue, which decides the scaling signal) and D-SERVERLESS (scale-to-zero breaks the GKE-only rule if it means Cloud Run).

| ID | Decision | Options | Provisional default | Blocks | Decider |
| --- | --- | --- | --- | --- | --- |
| D-HSCALE | Horizontal engine | HPA only · HPA + KEDA for event sources | HPA; KEDA only for queue consumers | Pod scaling, ingestion | Core developer |
| D-VSCALE | Vertical sizing per class | VPA Off (recommend) · VPA InPlaceOrRecreate (beta) · none | Recommend-only | Right-sizing, cost | Core developer |
| D-NODE | Node scaling model | Cluster autoscaler + custom compute classes · fixed pools · node auto-provisioning | Compute classes with reservation first, standard fallback | Capacity, DR | Core developer |
| D-SPOT | Classes allowed on Spot | None in prod · batch and tolerant workers only · broader | None in prod until shutdown tests exist | Cost, availability | Core developer |
| D-SERVERLESS | Scale-to-zero mechanism | None · KEDA (queues) · KEDA HTTP add-on (beta) · Knative on GKE · Cloud Run (leaves GKE-only) | KEDA for queue workers only; no HTTP scale-to-zero in prod | Cost of idle services | Core developer; Cloud Run needs B-1 + S-01 change |
| D-DATA | Database engine per tier | Cloud SQL Enterprise Plus · AlloyDB · Spanner · self-managed on GKE | Cloud SQL Enterprise Plus | DB scaling, DR | Core developer + data owners |
| D-DBSCALE | DB scaling policy | Vertical with planned windows · read replicas · Spanner compute autoscaling | Vertical + read replicas; planned windows | Peak capacity | Core developer |
| D-POOL | Connection pooling | Managed Connection Pooling · in-app pools only · self-run pooler | Managed pooling (transaction mode) where SQL features allow | Pod scale-out safety | Core developer |
| D-INGEST | Ingestion backbone | Pub/Sub · managed Kafka · self-managed Kafka on GKE | Decide from existing producers | Ingestion scaling signal | Core developer |
| D-INGEST-SCALE | Consumer scaling signal | Backlog / oldest unacked age (Pub/Sub) · consumer lag (Kafka) · CPU | Backlog or lag plus CPU, min ≥ 1 | Ingestion SLO | Core developer |
| B-2 / B-3 | Tier floors; Standard vs Autopilot | From register | Standard; floors from tiers | Floors, Spot grace tuning | Escalate (B-2) |
| D-BUDGET | Spend ceiling per tenant | From register | Every overrun approved | maxReplicas, node pool max | Escalate |
| B-1 | Residency for managed services | From register | Strictest reading | Managed DB, queue, serverless choices | Escalate |

## Business tree

Autoscaling exists to keep government services inside their SLO at peak while spending only what each tenant's budget allows; every technical rule below serves one of these branches.

```text
AUTOSCALING — BUSINESS TREE
│
├── 1. PURPOSE & VALUE
│   ├── Services stay within SLO at peak demand (citizen-facing outages are the top risk)
│   ├── Spend tracks real demand, inside each tenant's approved budget
│   ├── One tenant's growth never degrades another tenant
│   └── Capacity is predictable enough to plan government budget cycles
│
├── 2. ACTORS & WHAT THEY OWN
│   ├── Business / service owner → tier per service, RTO/RPO (B-2), peak-event calendar
│   ├── Budget role → spend ceiling per tenant, overrun approvals (D-BUDGET)
│   ├── Service team → scaling targets and signals within platform floors and ceilings
│   ├── Platform team (core developer) → engines, node classes, reservations, guardrails
│   ├── Data owners → database engine and scaling windows (D-DATA, D-DBSCALE)
│   ├── Security / compliance → residency of managed services (B-1), audit of scaling decisions
│   └── Incident commander → manual capacity overrides during incidents
│
├── 3. CAPACITY OFFERINGS (what a tenant can choose)
│   ├── Tier-critical (sync, citizen-facing)
│   │   └── Reserved capacity, tier floor across zones, no Spot, no scale-to-zero
│   ├── Standard
│   │   └── Autoscaled within ceilings; on-demand nodes; floor from tier
│   ├── Tolerant / batch / queue workers
│   │   └── Spot-eligible once D-SPOT allows; scale-to-zero via KEDA (D-SERVERLESS)
│   ├── Database capacity classes
│   │   └── Vertical resize in planned windows, read replicas, managed pooling (D-DBSCALE, D-POOL)
│   └── Ingestion consumers
│       └── Scaled on backlog or lag plus CPU, min ≥ 1 (D-INGEST, D-INGEST-SCALE)
│
├── 4. BUSINESS RULES
│   ├── Ceiling chain: maxReplicas ≤ quota ≤ IP headroom ≤ node pool max ≤ budget
│   ├── Scaling above a ceiling is a capacity request with approval, never automatic
│   ├── Tier decides floors, Spot eligibility and scale-to-zero — not the team's preference
│   ├── Known peak events are pre-scaled from the calendar, not left to reactive scaling
│   ├── Databases scale on a schedule with owners present; they never autoscale silently
│   └── Every scaling decision is attributable and retained per B-1
│
├── 5. LIFECYCLE (value stream)
│   ├── Onboard → choose tier and capacity offering; declare peak calendar
│   ├── Plan → platform computes ceiling chain, IP and reservation needs, cost band
│   ├── Approve → budget role approves max-scale spend
│   ├── Operate → autoscalers act inside the plan; alerts on caps, Pending, flapping
│   ├── Review (monthly) → right-size from VPA recommendations; adjust ceilings with approval
│   └── Retire → release quota, reservations, IP and budget
│
├── 6. BUSINESS RISKS
│   ├── Outage at peak because a silent ceiling (quota, IP, stockout) capped scaling
│   ├── Cost runaway from a bad signal or a metric outage that blocks scale-down
│   ├── Budget exhausted mid-year with no approval trail
│   ├── Noisy neighbour consuming shared IP space or node capacity
│   ├── Spot preemption hitting a service that should never have been on Spot
│   └── Managed database or queue choice violating residency (B-1)
│
├── 7. DECISIONS THE BUSINESS MUST MAKE (escalate)
│   ├── Tier per service and RTO/RPO (B-2)
│   ├── Spend ceiling per tenant and overrun approvers (D-BUDGET)
│   ├── Peak-event calendar and who maintains it
│   └── Residency limits on managed services (B-1)
│
└── 8. KPIs
    ├── SLO attainment during peak windows, per tier
    ├── Time from load increase to serving capacity
    ├── Capped-scaling events (desired ≠ achieved), target 0 for tier-critical
    ├── Cost per unit of demand; idle cost share; reservation utilisation
    └── Budget variance per tenant per period
```

## At a glance

Five layers scale independently, but all of them stop at the same chain of limits, ending at the tenant's budget.

&#91;embedded content: scaling layers and the ceiling chain\]

A scale-up that hits any link in the chain is capped silently unless it is alerted, which is why the ceiling chain is an invariant and not a guideline.

## Pod and node autoscaling

One writer per scaling field: replicas belong to HPA or KEDA, pod size to VPA or Git, the canary split to Argo Rollouts, and Argo CD ignores the first two. Everything else in this section follows from that rule.

### Horizontal (HPA, KEDA)

- The HPA loop runs every 15 seconds and ignores changes within a 0.1 ratio tolerance.
- Pods that are not ready, and CPU samples inside the 5-minute initialization window, are set aside, so startup spikes do not inflate scale-up.
- If any metric fails, scale-down is skipped but scale-up can still happen. A metrics outage therefore ratchets capacity and cost upward.
- Scale-down waits a 5-minute stabilization window by default; scale-up rate and window are `cfg.scaleup.*`.
- KEDA creates and owns the HPA for its target and refuses a second autoscaler on the same workload.
- With Argo Rollouts, the HPA targets the Rollout and averages metrics across stable and canary ReplicaSets.

### Vertical (VPA)

- In-place pod resize is stable since Kubernetes 1.35; VPA's InPlaceOrRecreate mode is beta.
- Resize is not allowed with swap or the static CPU and memory managers.
- Default: recommendation-only. In-place mode only for opted-in classes, never on a metric an HPA scales on.

### Nodes (cluster autoscaler, compute classes)

- Pending pods trigger scale-up through the compute class priority list: reservation, preferred family, then fallbacks.
- Nodes below the utilization threshold (0.5 on the balanced profile) become scale-down candidates.
- Scale-down is blocked by restrictive PDBs, pods without a controller, and pods marked not safe to evict.
- Zone balancing happens only during scale-up.
- Compute classes actively migrate workloads back to their top priority when it returns; set a disruption window for latency-critical classes.
- Never enable Compute Engine autoscaling on the node instance groups; it fights the cluster autoscaler.

### Spot

- GKE Spot VMs stop 30 seconds after notice. By default ordinary pods get 15 seconds, system pods the next 15.
- On Standard pools the total can be extended up to 120 seconds; `terminationGracePeriodSeconds` alone does not extend it.
- Preemption is involuntary, so PodDisruptionBudgets do not protect against it.
- Protection is placement: taint Spot pools, allow tolerations only for D-SPOT classes, keep at least one standard pool.

## Database scaling

Databases do not autoscale the way pods do. The real risk is the opposite direction: pods scale out in seconds and each new pod opens connections, so the database is the first thing a pod scale-up breaks.

### What scales, and how

| Dimension | Cloud SQL behaviour | Platform rule |
| --- | --- | --- |
| Storage | Automatic increase is on by default; the default increase limit is 0, meaning unlimited; capacity cannot be decreased in place | Always set an increase limit per instance; alert at `cfg.db.storage_alert` |
| CPU and memory | Changed by machine type; Enterprise Plus supports near-zero-downtime planned operations, other changes can restart | Vertical changes only in planned windows, via the Cloud Resource API, with approval |
| Read capacity | Read replicas (and cross-region replicas for DR) | Read-heavy tiers route reads explicitly; replicas count against budget |
| Connections | PostgreSQL uses a process per connection | Connection budget is a hard ceiling on pod scale-out (see below) |
| Spanner | Compute capacity can be scaled automatically by a managed autoscaler | Only if D-DATA selects Spanner; set min and max capacity explicitly |

### The connection ceiling

- Pod scale-out multiplies connections: maxReplicas × per-pod pool size must stay below what the database (or pooler) accepts.
- Managed Connection Pooling places a pooler between clients and Cloud SQL; it requires Enterprise Plus and the Auth Proxy (2.15.2+) or a direct connection.
- Transaction pooling (the default) does not support session features such as SET/RESET, LISTEN, PREPARE/DEALLOCATE and WITH HOLD cursors. Services that need them use session mode or their own pool.
- Enabling pooling on an existing instance restarts the database: schedule it as a planned change.

### Rules

1. Every autoscaled workload that talks to a database declares its per-pod pool size; admission computes maxReplicas × pool against the database's connection budget.
2. Database scaling is never automatic in production; it is a provisioning request with approval and a window.
3. Storage growth limits are mandatory; unbounded growth is a cost and capacity incident waiting to happen.
4. Read replicas and DR replicas are sized for peak, not average, because failover routes full load to them.

## Serverless and scale-to-zero

GKE does not scale a Deployment to zero by itself; KEDA is Google's documented route, and it is safe for queue workers but not yet for production HTTP services on this platform.

| Option | What scales to zero | Maturity / fit | Use here |
| --- | --- | --- | --- |
| KEDA on queue backlog | Workers consuming Pub/Sub or Kafka | Documented by Google for GKE; HPA rules apply once one pod exists | Yes, for tolerant workers in nonprod; prod only where tier allows |
| KEDA HTTP add-on | HTTP services, via an interceptor proxy in front of the app | Described by Google as a beta feature; adds a proxy in the request path | Nonprod and preview environments only |
| Knative Serving on GKE | HTTP services via its activator | Self-operated control plane; another component in the mesh path | Only if a strong case appears; extra failure domain |
| Cloud Run | Whole services | Managed, but outside GKE-only (S-01) and subject to B-1 residency | Not without changing S-01 and confirming B-1 |

### Rules

1. Synchronous user-facing services in production never scale to zero; the tier floor applies.
2. Scale-to-zero is allowed for: queue workers whose tier tolerates the first-message delay, scheduled jobs, preview and dev environments.
3. A scaled-to-zero HTTP service fails requests unless a proxy holds them; any HTTP scale-to-zero must sit behind the interceptor and declare a start-time budget.
4. Cold start (node provisioning + image pull + app start) is measured per service; scale-to-zero is rejected if it exceeds the tier's latency budget.
5. Scale-to-zero never applies to services holding database connections that cannot tolerate a reconnect storm on wake-up.

## Ingestion scaling

Ingestion consumers scale on backlog, not CPU, and both common backbones have a hard limit that more pods cannot fix: Kafka partitions and Pub/Sub metric delay.

### Pub/Sub consumers

- Backlog metrics (undelivered messages) are sampled every 60 seconds and can take up to 120 seconds more to become visible, so scaling reacts minutes late.
- Google advises against scaling on one Pub/Sub signal alone and against subscriber throughput metrics (ack counts), which create a loop where fewer acks cause scale-down.
- Scale on `num_unacked_messages` or `oldest_unacked_message_age` (the `by_region` variants), combined with CPU.
- Keep a minimum above zero so messages are handled even when recent metrics look empty, and use client flow control so under-scaled workers are not overwhelmed.

### Kafka consumers

- By default KEDA never scales beyond the topic's partition count, because extra consumers in a group sit idle.
- `limitToPartitionsWithLag` caps replicas at partitions that actually have lag, which avoids scaling to max for one hot key.
- Partition count is therefore the real ceiling on throughput; adding partitions is a planned change, not an autoscaling action.
- Rebalances happen on every scale event; frequent scaling causes repeated pauses. Scale-down stabilization must exceed rebalance cost.
- With no committed offset, the scaler's behaviour (keep one consumer or scale to zero) is set by `scaleToZeroOnInvalidOffset`; decide it explicitly.

### Rules

1. Every consumer declares its scaling signal (backlog or lag), a CPU co-signal, a minimum ≥ 1 in production, and a maximum ≤ partitions (Kafka).
2. Consumer maxReplicas also respects downstream limits: database connection budget and any external API rate limits.
3. Ingestion SLO is measured as age of oldest unprocessed message, not replica count.
4. Poison messages go to a dead-letter destination after `cfg.ingest.max_attempts`; they must never drive scale-up by inflating backlog forever.

## Invariants

Twenty invariants cover pods, nodes, databases, scale-to-zero and ingestion. Each has an enforcement point and a detection signal; none relies on someone remembering.

| ID | Invariant | Enforce | Detect | If broken |
| --- | --- | --- | --- | --- |
| AS-01 | One writer per replica field | CEL: one HPA or ScaledObject per target; no replicas in Git when autoscaled; Argo CD ignores replicas | Replica oscillation; Argo CD out-of-sync on replicas | PAGE in prod; remove second writer |
| AS-02 | Prod min replicas ≥ tier floor, spread across zones | CEL on HPA/ScaledObject; topology spread required | Replicas below floor; zone imbalance | PAGE tier-critical; TICKET others |
| AS-03 | Ceiling chain holds: maxReplicas × requests (+ surge + canary) ≤ quota ≤ node pool max ≤ IP headroom ≤ budget | Provisioning plan computes it; admission rejects beyond | HPA desired > achieved; ReplicaFailure | PAGE if tier-critical capped under load |
| AS-04 | A missing metric never reads as idle | Alerts on metric fetch failures; KEDA fallback (AverageValue only) | FailedGet\*Metric conditions | PAGE if metrics API down cluster-wide |
| AS-05 | Every container sets requests | Policy rule TEN-04 | Pods without requests = 0 | Deny at admission |
| AS-06 | HPA and VPA never act on the same metric | Policy rule AVL-03; VPA recommend-only by default | Requests and replicas oscillating together | TICKET; disable VPA mode |
| AS-07 | Spot only for D-SPOT classes; tier-critical never on Spot | Tainted Spot pools; CEL limits tolerations | Tier-critical pods on Spot = 0 | PAGE; move via compute class |
| AS-08 | Spot-eligible workloads finish shutdown inside the grace window | preStop + shutdown test in CI | Errors after preemption | TICKET owner; revoke Spot eligibility |
| AS-09 | Tier-critical scale-up has reserved capacity | Reservations in compute class priority list | Pending duration for tier-critical | PAGE at `cfg.pending.page` |
| AS-10 | Pod IP space planned for maximum scale | Capacity model includes per-node CIDR × node pool max | Pod range utilization | PAGE at `cfg.ip.page_threshold` |
| AS-11 | Scale-down never breaks availability or blocks forever | PDBs allow ≥ 1 disruption; AlwaysAllow unhealthy eviction; no bare pods | noScaleDown reasons; idle nodes | TICKET naming the blocker |
| AS-12 | Canary judged per revision, not by HPA averages | Analysis on per-revision metrics; quota sized for surge + canary | Scale-up during canary steps | Abort canary on regression |
| AS-13 | No Compute Engine autoscaler on node instance groups | Cluster API baseline; posture scan | MIG autoscaler configured = 0 | Remove; review |
| AS-14 | Scaling bounded by tenant budget | maxReplicas and pool max derived from D-BUDGET | Projected spend at max vs budget | TICKET; approval for overrun |
| AS-15 | Every scaling decision auditable | Event export to audit sink | Export gaps | TICKET compliance |
| AS-16 | Connections never exceed database budget at max scale | Admission: maxReplicas × pool ≤ DB/pooler limit | Connection count vs limit; connection errors | PAGE at `cfg.db.conn_page` |
| AS-17 | Database storage growth is bounded | Increase limit mandatory per instance | Storage vs limit | TICKET at `cfg.db.storage_alert` |
| AS-18 | Database capacity changes only via approved, windowed requests | Cloud Resource API; no direct console changes | Audit log: DB tier changes outside API = 0 | PAGE security |
| AS-19 | No production sync service scales to zero | CEL: minReplicas ≥ floor for sync tiers; HTTP scale-to-zero only behind interceptor in nonprod | minReplicas = 0 on sync prod services = 0 | Deny |
| AS-20 | Consumers scale on backlog or lag with min ≥ 1 and max ≤ partitions (Kafka) | CEL on ScaledObject; signal + co-signal declared | Oldest message age vs SLO; idle consumers | PAGE if ingestion SLO breached |

## Critical edge cases

Twenty-six cases where autoscaling either does the wrong thing or silently does nothing. Most are silent: the system reports "scaled" while capacity never arrives.

### Pods and nodes

| ID | Scenario | Why it happens | Handling | Detection |
| --- | --- | --- | --- | --- |
| EC-01 | Mass Spot preemption | 30 s notice, 15 s for pods by default; PDBs don't apply | Only tolerant classes on Spot; compute class falls back to standard | Preemptions per pool; tier-critical on Spot = 0 |
| EC-02 | Compute class pulls pods back to Spot when it returns | Active migration to top priority | Disruption window per class; off for latency-critical | Evictions tied to migration |
| EC-03 | Startup CPU spike triggers over-scaling | Heavy init (JIT, cache warm) | Initialization window + accurate readiness | Scale-ups right after rollouts |
| EC-04 | Metrics API outage | Adapter or API down | Scale-down skipped automatically; alert; hold manual capacity | FailedGetResourceMetric |
| EC-05 | HPA wants N, quota allows fewer | Quota sized for steady state | Ceiling chain at admission | ReplicaFailure events |
| EC-06 | Pending pods, zone stockout | No capacity in family/zone | Next compute class priority; reservations for tier-critical | Pending duration |
| EC-07 | Pod IP exhaustion mid-surge | Per-node CIDR reserved regardless of use | IP capacity model; new pod ranges planned ahead | Range utilization |
| EC-08 | Canary regression drives scale-up | HPA averages stable + canary | Per-revision analysis aborts canary | Scale-up during canary step |
| EC-09 | Pinned canary + full stable exceeds quota | Surge + canary above steady state | Quota includes surge and canary | ReplicaFailure during rollout |
| EC-10 | VPA resize cannot fit on node | Node full; static CPU manager | Recommend-only default; resize deferred, alert | Resize infeasible events |
| EC-11 | Scale-down blocked forever | Bare pod, strict PDB, not-safe-to-evict | Policy rules AVL-01/02; no bare pods | noScaleDown reasons |
| EC-12 | Load rises faster than nodes arrive | Node provisioning + image pull time | Low-priority placeholder pods as headroom for tier-critical | Pending pods with HPA at target |
| EC-13 | Registry or attestation slow during surge | New pods can't start | Regional mirror; pre-pulled base images | ImagePullBackOff; admission latency |

### Databases

| ID | Scenario | Why it happens | Handling | Detection |
| --- | --- | --- | --- | --- |
| EC-14 | Pod scale-out exhausts database connections | maxReplicas × pool > DB limit | AS-16 admission check; managed pooling | Connection errors; connection count |
| EC-15 | Reconnect storm after DB restart or failover | Every pod reconnects at once | Jittered backoff in clients; pooler absorbs | Connection spikes after restart |
| EC-16 | Session feature breaks under transaction pooling | SET, LISTEN, PREPARE unsupported | Session mode or app pool for those services | Errors after enabling pooling |
| EC-17 | Enabling pooling restarts the database | Documented behaviour on existing instances | Planned window via API | Restart events |
| EC-18 | Storage grows without limit | Auto-increase on, limit 0 = unlimited | Mandatory increase limit | Storage vs limit |
| EC-19 | Read replica lag under scale-out | Replica can't keep up | Read routing tolerant of staleness; lag alert | Replication lag |

### Scale-to-zero

| ID | Scenario | Why it happens | Handling | Detection |
| --- | --- | --- | --- | --- |
| EC-20 | First request to zeroed HTTP service fails | No backend without a proxy | Interceptor in path; nonprod only | 5xx on wake |
| EC-21 | Wake-up exceeds latency budget | Node + image + app start | Measured cold start; reject scale-to-zero above budget | Cold start p95 |
| EC-22 | Zeroed worker misses messages because metrics look empty | Backlog metric delay | Minimum ≥ 1 for tier-relevant consumers | Oldest message age |

### Ingestion

| ID | Scenario | Why it happens | Handling | Detection |
| --- | --- | --- | --- | --- |
| EC-23 | Kafka consumers capped at partitions | One consumer per partition | Plan partitions for peak; scale caps at partitions | Lag with max replicas reached |
| EC-24 | One hot partition scales whole group to max | Total lag counted | `limitToPartitionsWithLag` | Idle consumers at max replicas |
| EC-25 | Rebalance storms from frequent scaling | Every scale event rebalances | Longer scale-down stabilization; cooperative rebalancing | Rebalance frequency; processing pauses |
| EC-26 | Poison message inflates backlog forever | Same message retried | Dead-letter after `cfg.ingest.max_attempts` | Backlog flat while consumers busy |

## Critical failure points

Fourteen ways autoscaling causes an outage, cost runaway or data loss; each maps to the invariant that prevents it.

| ID | Failure | Impact | Prevented by |
| --- | --- | --- | --- |
| CF-01 | Replica tug-of-war between Git, HPA and KEDA | Outage (flapping) | AS-01 |
| CF-02 | Silent capacity cap (quota, IP, node max, budget) at peak | Outage | AS-03, AS-10 |
| CF-03 | Tier-critical workload on Spot during mass preemption | Outage | AS-07 |
| CF-04 | Metrics outage misread as idle, or ratchet-up | Outage or cost | AS-04 |
| CF-05 | Stockout during surge or DR failover | Outage | AS-09 |
| CF-06 | Scale-down evicts below safe capacity | Outage | AS-02, AS-11 |
| CF-07 | Cost runaway from wrong target or unbounded storage | Cost | AS-14, AS-17 |
| CF-08 | Canary regression hidden by autoscaling | Outage | AS-12 |
| CF-09 | HPA and VPA oscillation | Degraded service | AS-06 |
| CF-10 | Database connection exhaustion from scale-out | Outage | AS-16 |
| CF-11 | Reconnect storm after DB failover | Outage | AS-16 + client backoff |
| CF-12 | Unapproved database resize or restart | Outage, compliance | AS-18 |
| CF-13 | Sync service scaled to zero in prod | Outage (failed first requests) | AS-19 |
| CF-14 | Ingestion falls behind with consumers at partition cap | Data delay; SLO breach | AS-20 + partition planning |

## Production operations

The single most useful signal is the gap between the replicas an autoscaler wants and the replicas actually serving; every silent failure above shows up there first.

### SLIs (targets are `cfg.*`, owned by platform)

| SLI | Measures | Layer |
| --- | --- | --- |
| Desired vs achieved replicas | Silent caps (quota, IP, nodes, policy) | Pods |
| Time from load increase to serving capacity, p95 | Scale-up speed per tier | Pods + nodes |
| Pending pod duration | Node scale-up and stockouts | Nodes |
| Tier-critical pods on Spot | Placement safety (target 0) | Nodes |
| Metric fetch failure rate | Signal health | All |
| DB connections vs budget | Scale-out safety | Database |
| DB storage vs limit; replica lag | Growth and read capacity | Database |
| Cold start p95 | Scale-to-zero fitness | Serverless |
| Oldest unprocessed message age | Ingestion SLO | Ingestion |
| Idle node hours; cost at current vs max scale | Efficiency and budget | All |

### Alerts

- **PAGE:** tier-critical Pending > `cfg.pending.page`; desired ≠ achieved for tier-critical; metrics API down; tier-critical on Spot; IP utilization ≥ `cfg.ip.page_threshold`; replica flapping in prod; DB connections ≥ `cfg.db.conn_page`; DB tier change outside the API; ingestion SLO breached.
- **TICKET:** ReplicaFailure from quota; blocked scale-down; VPA resize infeasible; DB storage near limit; budget approaching; rebalance frequency high.

### Runbooks

1. **Capped scaling:** find which ceiling (quota, IP, node max, budget, DB connections) → raise through a provisioning request with approval, or shed lower tiers.
2. **Stockout:** confirm Pending cause → next compute class priority → reservations → shed lower tiers.
3. **Metrics outage:** scale-down is already frozen → restore the adapter → verify HPA conditions.
4. **Flapping:** find the second writer (Argo CD diff, KEDA, manual change) → remove → verify stable.
5. **Mass preemption:** confirm only tolerant classes were hit → watch fallback → review Spot share.
6. **DB connection exhaustion:** cap maxReplicas → enable or resize pooling (planned) → fix per-pod pool size.
7. **Ingestion behind:** check partitions vs max replicas → dead-letter poison messages → add partitions via planned change.

### Cadence and ownership

- Weekly: capacity review (quota, IP, reservations, DB connections and storage).
- Monthly: load test for each tier-critical service; scale-to-zero cold-start check.
- Quarterly: Spot preemption game day; DB failover reconnect test.
- Platform owns autoscalers, compute classes, reservations and guardrails. Service owners own HPA targets, readiness, shutdown and pool sizes. Data owners own DB scaling requests. The budget role owns D-BUDGET.

## Roadmap and go-live gates

Each phase ends on a gate, not a date; tier-critical services go live only after phase 1.

| Phase | Scope | Exit gate |
| --- | --- | --- |
| 0 — Guardrails | One replica writer, tier floors, ceiling chain, requests required, metric-failure alerts, business tree owners named | Flapping and over-ceiling tests are denied |
| 1 — Node layer | Compute classes, reservations for tier-critical, pod IP plan, no MIG autoscaler | Surge test reaches capacity within `cfg.surge.time` with no Pending tier-critical pods |
| 2 — Data and ingestion | D-DATA, D-DBSCALE, D-POOL, D-INGEST, D-INGEST-SCALE decided; pooling in front of every scaled-out service; lag-based consumer scaling | Load test shows connections stay under the database limit at max replicas; backlog drains within `cfg.ingest.drain` |
| 3 — Spot and scale-to-zero | Spot for tolerant classes (D-SPOT); KEDA scale-to-zero for queue workers (D-SERVERLESS) | Preemption game day with zero tier-critical impact; zero-to-one start inside its declared budget |
| 4 — Vertical | VPA recommend-only everywhere; in-place resize for opted-in classes | One month without HPA/VPA oscillation |

**Go-live gates**

- [ ] Every row in Pending selections is DECIDED or PROVISIONAL with an owner
- [ ] Business tree section 7 answered in writing (tiers, budget, peak calendar, residency)
- [ ] All invariants enforced with an alert and a tested runbook
- [ ] Load test and preemption game day passed for every tier-critical service

## Sources checked

| Claim used in this doc | Source |
| --- | --- |
| HPA syncs every 15 s, tolerance 0.1, 5-minute downscale stabilization; failed metrics skip scale-down but allow scale-up | [Kubernetes: Horizontal Pod Autoscaling](https://kubernetes.io/docs/concepts/workloads/autoscaling/horizontal-pod-autoscale) |
| Spot VMs end 30 s after notice; regular Pods get 15 s by default, extendable on Standard to 120 s total | [GKE: Spot VMs](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/spot-vms) |
| Scale-down blocked by restrictive PDBs and bare Pods; 0.5 utilization threshold on the balanced profile | [GKE: Troubleshoot cluster autoscaler scale-down](https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cluster-autoscaler-scale-down) |
| No Compute Engine autoscaling on node MIGs; compute classes for Spot; zone balancing only on scale-up | [GKE: About cluster autoscaling](https://docs.cloud.google.com/kubernetes-engine/docs/cluster-autoscaler) |
| Compute classes migrate workloads back to their top priority | [Google Cloud blog: GKE features to optimize resource allocation](https://cloud.google.com/blog/products/containers-kubernetes/gke-features-to-optimize-resource-allocation) |
| KEDA blocks a second autoscaler on one target; fallback only for AverageValue metrics | [KEDA: Admission webhooks](https://keda.sh/docs/concepts/admission-webhooks/) |
| KEDA is Google's documented scale-to-zero route on GKE; zeroed HTTP services fail requests without the KEDA-HTTP proxy | [GKE: Scale to zero using KEDA](https://cloud.google.com/kubernetes-engine/docs/tutorials/scale-to-zero-using-keda) |
| HPA on a canary Rollout uses metrics from all its ReplicaSets | [Argo Rollouts: HPA support](https://argo-rollouts.readthedocs.io/en/release-1.4/features/hpa-support/) |
| In-place Pod resize GA in 1.35; VPA InPlaceOrRecreate beta; not with static CPU or memory managers | [Kubernetes blog: In-place Pod resize GA](https://kubernetes.io/blog/2025/12/19/kubernetes-v1-35-in-place-pod-resize-ga) |
| Managed Connection Pooling needs Enterprise Plus; transaction mode drops SET/RESET, LISTEN, PREPARE and similar features | [Cloud SQL: Managed Connection Pooling](https://docs.cloud.google.com/sql/docs/postgres/managed-connection-pooling) |
| Per-node pod CIDR reserved regardless of Pod count | [Google Cloud blog: Avoiding GKE IP\_SPACE\_EXHAUSTED](https://cloud.google.com/blog/products/containers-kubernetes/avoiding-the-gke-ip_space_exhausted-error) |
| Reservations hold capacity for DR and peaks, billed while held | [GKE: Consuming reserved zonal resources](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/consuming-reservations) |