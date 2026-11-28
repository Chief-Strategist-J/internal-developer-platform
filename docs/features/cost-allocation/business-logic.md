# Cost Allocation — Showback, Chargeback and Budgets

Nov 28, 2026 · @Chief

## Pending selections

Thirteen choices are open. Three of them change what a tenant is billed (D-COSTMODEL, D-ALLOCBASIS, D-SHARED) and need the budget role's sign-off; the rest the core developer can close.

| ID | Decision | Options | Provisional default | Decider |
| --- | --- | --- | --- | --- |
| D-COSTMODEL | Showback or chargeback | Showback only · chargeback to cost centers · showback now, chargeback later | Showback first; chargeback only after two clean monthly reconciliations | Escalate (budget role) |
| D-ALLOCBASIS | What a workload pays for | Requests (GKE native) · max(request, usage) · actual usage | Requests — what a tenant reserves is what others cannot use | Escalate |
| D-SHARED | Who pays shared and idle cost | Separate platform cost center · spread by request share · fixed platform fee | Platform cost center for system overhead; idle node capacity spread by request share | Escalate |
| D-COSTTOOL | Allocation engine | GKE cost allocation in the detailed BigQuery export · in-cluster OpenCost · both | GKE native as the financial record; in-cluster estimate only for near-real-time alerts | Core developer |
| D-EGRESS | Network and egress attribution | Per-project network SKUs · flow-log based split · shared platform cost | Per-project where traffic leaves a tenant project; otherwise platform cost center | Core developer |
| D-DISCOUNTS | How CUDs, credits and negotiated discounts flow to tenants | Net (after credits) · list price · both shown | Report both; bill net | Escalate (budget role) |
| D-CURRENCY | Reporting currency and exchange-rate source | Billing account currency only · converted with a named rate source | Billing account currency; no conversion until a rate source is named | Escalate |
| D-SPENDCAP | Use of spend-cap budgets that pause services | Never · nonprod only · everywhere | Nonprod and sandbox projects only; never production | Core developer + business owner |
| D-ANOMALY | Anomaly signal source | Billing export (hours to days late) · in-cluster request estimate (minutes) · both | Both: in-cluster for speed, export for truth | Core developer |
| D-LABELS | Cost label taxonomy and value source | Free text · values validated against the service catalog | owner, cost-center, env, app, request-id; values validated against catalog | Core developer |
| D-PROJECTS | Project layout for dedicated cloud resources | Project per tenant per env · shared projects with labels | Project per tenant per env | Core developer |
| D-BUDGET | Spend ceiling per tenant and overrun approvers | From decision register | Every overrun approved | Escalate |
| B-1 | Location and retention of the billing export dataset | From decision register | In-country region; retention per regime | Escalate |

## Business tree

Cost allocation exists so every unit of platform spend has a named owner, a budget and an approval trail an auditor can follow.

```text
COST ALLOCATION — BUSINESS TREE
│
├── 1. PURPOSE & VALUE
│   ├── Every cost line traceable to a tenant, service and cost center
│   ├── Tenants see what they spend and why (showback), then pay for it (chargeback)
│   ├── Budgets are enforced by approval, never by surprise outages
│   └── Public money spent with evidence: who approved what, when
│
├── 2. ACTORS & WHAT THEY OWN
│   ├── Budget role → budgets per tenant, overrun approvals, chargeback policy
│   ├── Service owner → their spend, right-sizing, responding to anomalies
│   ├── Platform team → allocation engine, labels, shared-cost model, reports
│   ├── Finance / accounts → invoice reconciliation, currency, discounts
│   ├── Security / compliance → billing data location and retention (B-1)
│   └── Auditor → read-only access to allocation history and approvals
│
├── 3. OFFERINGS
│   ├── Monthly showback statement per tenant and service
│   ├── Chargeback to cost centers (after D-COSTMODEL)
│   ├── Budget with alerts and an approval path for overruns
│   ├── Anomaly alerts (fast estimate + confirmed from billing export)
│   └── Right-sizing and waste recommendations, delivered as PRs
│
├── 4. BUSINESS RULES
│   ├── No resource without owner and cost-center labels
│   ├── Allocated total always reconciles to the invoice
│   ├── Shared and idle costs are shown openly, never hidden in tenant lines
│   ├── Budgets alert and require approval; they never pause production
│   ├── Recommendations are proposals; data-bearing resources are never auto-deleted
│   └── Allocation rules change only with notice and from the next period
│
├── 5. LIFECYCLE
│   ├── Onboard → tenant gets cost center, budget, labels, projects
│   ├── Accrue → billing export lands; allocation runs daily
│   ├── Watch → anomalies and budget thresholds alert owners
│   ├── Close month → reconcile to invoice; publish statements
│   ├── Act → right-size, schedule nonprod off-hours, approve overruns
│   └── Offboard → final statement; release budget; keep records per retention
│
├── 6. BUSINESS RISKS
│   ├── Statements that don't reconcile to the invoice → trust and audit failure
│   ├── Unowned spend nobody pays for
│   ├── Late detection: runaway spend found days later
│   ├── A spend cap pausing a citizen-facing service
│   ├── Disputes over shared-cost fairness between tenants
│   └── Billing data stored outside allowed residency
│
└── 7. KPIs
    ├── Allocated share of total spend (target: everything except declared platform cost)
    ├── Unlabelled spend (target: 0)
    ├── Reconciliation difference to invoice (target: 0 after month close)
    ├── Time to detect a cost anomaly
    ├── Budget variance per tenant
    └── Recommended vs realised savings
```

## At a glance

Two paths run side by side: a slow, reconciled path that produces statements, and a fast estimate that only raises alerts.

&#91;embedded content: cost flow · truth path and fast path\]

Keeping the paths separate is what lets a statement survive an audit while owners still hear about runaway spend within minutes.

## How it works on GKE

The billing export is the financial record; everything else is an estimate. Google gives no latency guarantee for it, so fast alerts must come from in-cluster signals and be confirmed later against the export.

### 1. Ingestion

- **Source:** Cloud Billing detailed usage cost export to BigQuery. GKE namespace and label costs appear only in the detailed export, not the standard one.
- **Timing:** loads arrive at intervals with no delivery or latency guarantee; reporting frequency varies by service.
- **Backfill:** a multi-region dataset backfills from the start of the previous month; a regional dataset (likely under B-1) starts only from the day export is enabled. Turn it on before the first tenant.
- **Month boundaries:** invoice month can differ from usage month, and adjustments land in the month they are issued.
- **Schema:** fields can be added; queries that select columns by position or `*` can break.
- **Pricing table:** enable the pricing export too, so list vs net can be shown per SKU.

### 2. Allocation

| Cost type | Method | Notes |
| --- | --- | --- |
| Node vCPU, RAM, GPU, persistent disk capacity | GKE cost allocation: split by **requests** per namespace and Pod label | Not usage. Pods with more than 50 labels lose all labels in billing. Labels on PVCs don't appear. Up to 3 days after enabling. No backfill |
| Unrequested node capacity (idle) | Shown separately in the export as unallocated | Spread by request share or kept as platform cost (D-SHARED) |
| System overhead (kube-system, mesh, agents) | Platform cost center | Shown, never hidden |
| Dedicated cloud resources (Cloud SQL, buckets, Pub/Sub) | Direct by project and labels | Project per tenant per env makes this exact (D-PROJECTS) |
| Network egress, load balancers | Per project where possible; else platform cost | Not covered by GKE cost allocation SKUs (D-EGRESS) |
| Credits, CUDs, negotiated discounts | Net cost = cost + credits | Report list and net (D-DISCOUNTS) |

### 3. Budgets and anomalies

- **Budgets:** per tenant (folder or project scope) with alert thresholds and Pub/Sub notifications to the platform.
- **Spend caps:** Google now offers spend-cap budgets that pause listed services once estimated gross spend passes the cap. They don't stop persistent resources from accruing, and they reset each period. Pausing a citizen-facing service is an outage, so production never gets one (D-SPENDCAP).
- **Fast anomaly signal:** in-cluster requests × list rates, every few minutes.
- **Confirmed anomaly:** the same window recomputed from the export once it lands.

### 4. Optimisation

- Right-sizing from VPA recommendations and request-vs-usage gaps, proposed as PRs to the service repo.
- Off-hours schedules for nonprod, owned and approved by the tenant.
- Orphan sweep (disks, IPs, load balancers, snapshots) through the provisioning ledger — data-bearing items need a retention decision before deletion.
- Spot prices change at most once a day, so per-minute price monitoring adds nothing; Spot cost risk is preempted work being redone.

## Invariants

Fifteen invariants; the first three are what make a statement defensible to an auditor.

| ID | Invariant | Why | Enforce | Detect | Production handling |
| --- | --- | --- | --- | --- | --- |
| CA-01 | Allocated total + declared platform cost = invoice total, per invoice month | Statements that don't reconcile lose trust and fail audit | Month-close job compares sums by invoice month, net of credits | Reconciliation difference ≠ 0 | Block statement publication; TICKET finance + platform |
| CA-02 | Every allocation run is reproducible from stored inputs and a versioned rule set | Disputes need a replay | Rules in Git; inputs snapshot per run | Replay of a sampled month differs | TICKET platform; freeze rule changes until fixed |
| CA-03 | Allocation rules change only from the next period, with notice | Mid-month changes look like manipulation | Rule version effective-from date enforced | Rule change applied to current period | Revert; re-run period |
| CA-04 | No resource or namespace without owner and cost-center labels, values from the catalog | Unowned spend nobody pays | Admission (TEN-01) + Cloud Resource API + org policy on projects | Unlabelled spend in export | TICKET owner; platform cost center holds it until fixed |
| CA-05 | Pods carry at most 50 labels | More than 50 drops all labels from billing | CEL admission rule | Cost lines with namespace but no app label | TICKET owner |
| CA-06 | Cost labels live on Pods, workloads and projects, not only on PVCs | PVC labels don't reach billing | Templates put labels on workload Pods | Disk cost without app label | TICKET platform (template fix) |
| CA-07 | Every container sets requests | Allocation is by requests; none means invisible spend | TEN-04 admission rule | Pods without requests (0) | Reject at admission |
| CA-08 | Shared and idle costs are shown as their own lines | Hidden overhead causes disputes | Allocation output schema has platform and idle rows | Idle row missing from a statement | Block publication |
| CA-09 | Budgets never pause production | A spend cap on production is a self-inflicted outage | Spend-cap budgets only on nonprod/sandbox; org-level check | Spend-cap budget attached to a prod project | PAGE platform; remove cap |
| CA-10 | Every budget has an owner, thresholds and a routed notification | A budget nobody receives is not a control | Budget created by provisioning with Pub/Sub to platform | Budget without notification channel | TICKET budget role |
| CA-11 | Fast estimates are labelled as estimates; statements use the export only | Export has no latency guarantee; estimates drift | Separate tables and report labels | Statement built from estimate table | Block publication |
| CA-12 | Billing export dataset location and retention satisfy B-1 | Billing data can reveal operational detail | Dataset in allowed region; retention lock | Location audit | PAGE security on violation |
| CA-13 | Export never paused or moved without a gap record | Re-enabling or moving export doesn't backfill | Change control on export settings | Days with zero rows | TICKET platform; record gap in evidence |
| CA-14 | Recommendations never delete data-bearing resources automatically | Savings must not cost data | Optimiser opens PRs; deletions follow retention decision | Deletion by optimiser identity | PAGE security |
| CA-15 | Every overrun and rule change is approved and attributable | Public spending needs an approval trail | Approval workflow with identity; audit sink | Overrun without approval record | TICKET budget role |

## Critical edge cases

Sixteen cases where a naive engine bills the wrong tenant, double-counts, or misses spend.

| # | Edge case | What goes wrong | Handling |
| --- | --- | --- | --- |
| EC-C01 | Usage at month end lands on next month's invoice | Statement for month N misses cost; N+1 overstates | Statements keyed by invoice month; usage month shown alongside |
| EC-C02 | Billing adjustment or credit issued months later | Old statement no longer reconciles | Adjustments booked in the month issued, with a reference to the period affected |
| EC-C03 | Export arrives late or with gaps | Anomaly missed; month close runs on partial data | Month close waits for completeness check; fast estimate covers detection |
| EC-C04 | Cost allocation enabled mid-month | No backfill; earlier days unallocated | Enable before onboarding; earlier days held in platform cost with a note |
| EC-C05 | Pod with more than 50 labels | All its labels vanish from billing | CEL limit (CA-05); unlabelled lines held in platform cost |
| EC-C06 | Workload has no requests | Its node share lands in idle, paid by others | Admission rejects (CA-07) |
| EC-C07 | Requests far above usage | Tenant pays for reservation it doesn't use | Intended under D-ALLOCBASIS; right-sizing PR offered |
| EC-C08 | Usage far above requests (burstable) | Tenant uses capacity it doesn't pay for | Show request-vs-usage gap; limits or max(request, usage) if D-ALLOCBASIS changes |
| EC-C09 | Reservations held for DR or tier-critical | Idle reservation cost with no workload | Charged to the tier or DR owner that required it, as its own line |
| EC-C10 | CUD or credit applied at billing-account level | Discount lands where usage happened to be | Net-cost rule per D-DISCOUNTS; list price shown too |
| EC-C11 | Network egress and load balancers | Not split by namespace natively | Per-project attribution; shared ones to platform (D-EGRESS) |
| EC-C12 | Logging, monitoring, audit-log volume | Large shared cost, often forgotten | Allocate by log bucket or project; Data Access log volume shown per tenant |
| EC-C13 | Spend-cap budget on a shared or production project | Citizen-facing service paused | Forbidden (CA-09) |
| EC-C14 | Tenant renamed, reorganised or cost center changed mid-period | Split statements, orphaned budget | Label change effective from next period; mapping table for history |
| EC-C15 | Resource created outside the Cloud Resource API | No labels; unowned | Sweeper flags it; held in platform cost until adopted |
| EC-C16 | Currency or tax differences across exports | Totals don't match finance's numbers | Report in billing-account currency; conversion only with a named rate source (D-CURRENCY) |

## Critical failure points

Twelve failures, ordered by blast radius: the top four affect every tenant's statement at once.

| # | Failure | Impact | Handling | Detection |
| --- | --- | --- | --- | --- |
| CF-01 | Allocation bug charges tenants wrong amounts | Disputes, audit finding, possible repayment | CA-01 reconciliation blocks publication; replay (CA-02) | Reconciliation difference |
| CF-02 | Export schema change breaks queries | Allocation stops or silently drops columns | Explicit column lists; schema-diff test before each run | Run failure or row-count drop |
| CF-03 | Export disabled, moved or paused | Days of spend missing; no backfill | Change control (CA-13); gap recorded as evidence | Zero-row days |
| CF-04 | Rule change applied to a closed period | Published statements change after the fact | Effective-from enforcement (CA-03) | Statement hash changes after close |
| CF-05 | Runaway spend detected days late | Budget blown before anyone sees it | In-cluster estimate alerts within minutes (D-ANOMALY) | Estimate vs budget burn rate |
| CF-06 | Spend cap pauses a production API | Citizen-facing outage | CA-09; caps only on nonprod | Org scan for caps on prod |
| CF-07 | Unlabelled resources grow | Platform cost center balloons; nobody owns it | Admission + sweeper + project-level labels | Unlabelled share above `cfg.unlabelled.max` |
| CF-08 | Optimiser deletes a disk or snapshot still needed | Data loss | PRs only; retention decision required (CA-14) | Deletion audit by optimiser identity |
| CF-09 | Billing data stored in a disallowed location | Compliance breach | Dataset location fixed by B-1 (CA-12) | Location audit |
| CF-10 | Fast estimate published as a statement | Wrong numbers sent to finance | Separate tables and labels (CA-11) | Statement source check |
| CF-11 | Shared-cost model disputed | Tenants contest bills; chargeback stalls | Model approved by budget role before chargeback (D-SHARED) | Dispute count |
| CF-12 | BigQuery cost of the cost system itself grows | Cost allocation becomes a cost problem | Partitioned tables, scheduled aggregates; enabling allocation raises storage and query cost with label cardinality | BigQuery spend on the billing dataset |

## Production handling

Cost allocation runs as a monthly financial process with daily signals; the month close is the one step that must never be skipped or rushed.

### SLIs (targets are `cfg.*`, owned by platform + budget role)

| SLI | Target |
| --- | --- |
| Reconciliation difference to invoice after month close | 0 |
| Unlabelled share of spend | ≤ `cfg.unlabelled.max` |
| Export freshness (newest usage hour in BigQuery) | ≤ `cfg.export.max_lag` |
| Time to detect a runaway spend (estimate) | ≤ `cfg.anomaly.detect` |
| Statement publication after invoice available | ≤ `cfg.close.days` |
| Open disputes older than `cfg.dispute.sla` | 0 |

### Alerts

- **PAGE platform:** export stale beyond `cfg.export.max_lag`; allocation run failed on month-close day; spend-cap budget found on a production project.
- **PAGE owner + platform:** estimated burn rate puts a tenant past budget before period end.
- **TICKET:** budget thresholds reached; unlabelled spend above target; schema diff detected; BigQuery cost of billing dataset above `cfg.billing_bq.max`.

### Runbooks

1. **Month close:** wait for export completeness → run allocation with frozen rule version → reconcile to invoice (CA-01) → publish statements → archive inputs and outputs.
2. **Runaway spend:** confirm on estimate → notify owner → owner scales down or approves → confirm against export when it lands → record outcome.
3. **Reconciliation mismatch:** diff by SKU and project → find unlabelled, adjustment or rule cause → fix → re-run → publish with note.
4. **Export gap:** record gap window → restore export → mark affected statements as provisional → reconcile against invoice when available.
5. **Dispute:** replay the period (CA-02) → show line-level lineage → budget role decides → credit or confirm in next period.

### Cadence and ownership

- **Daily:** export freshness, anomaly review, unlabelled spend.
- **Weekly:** budget burn per tenant; right-sizing PRs reviewed.
- **Monthly:** month close and statements; shared-cost lines reviewed with budget role.
- **Quarterly:** allocation rule review; replay test on a past month; auditor dry run.
- **Owners:** platform (engine, data, alerts), budget role (budgets, approvals, model), finance (invoice, currency), service owners (their spend and actions).

## Roadmap and go-live gates

Showback goes live first; chargeback only after the engine has reconciled cleanly for two consecutive months.

| Phase | Scope | Exit gate |
| --- | --- | --- |
| 0 — Record | Detailed export + pricing export in an allowed region; GKE cost allocation on every cluster before first tenant; label taxonomy and admission rules | Export fresh; zero Pods without requests; labels validated against catalog |
| 1 — Showback | Allocation by requests; platform and idle lines; monthly statements; reconciliation job | Two consecutive months reconcile to invoice with zero difference |
| 2 — Budgets and anomalies | Budgets per tenant with Pub/Sub; in-cluster estimate alerts; spend caps on nonprod only | Injected runaway workload detected within `cfg.anomaly.detect` |
| 3 — Chargeback | Approved shared-cost model and discount rule; dispute process | Budget role signs off the model; one dry-run chargeback with no upheld dispute |
| 4 — Optimisation | Right-sizing PRs, nonprod schedules, orphan sweep through the ledger | Realised savings tracked against recommendations; no data-bearing deletion without a retention decision |

**Go-live gates**

- [ ] D-COSTMODEL, D-ALLOCBASIS, D-SHARED and D-DISCOUNTS signed off by the budget role
- [ ] B-1 answered for the billing dataset location and retention
- [ ] Invariants CA-01 to CA-15 enforced with detection
- [ ] Month-close runbook executed end to end on real data

## Sources checked

| Claim used in this doc | Source |
| --- | --- |
| GKE cost allocation uses requests, appears only in the detailed export, has no backfill, takes up to 3 days, drops labels above 50 per Pod, ignores PVC labels, raises BigQuery cost with label cardinality | [GKE: Cost allocations](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/cost-allocations) |
| Export has no delivery or latency guarantee; regional datasets don't backfill; moving or re-enabling export leaves gaps; schema can change | [Cloud Billing: Data tables in BigQuery](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-tables) |
| Invoice month can differ from usage month; adjustments land in the month issued | [Cloud Billing: Standard usage export schema](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery-tables/standard-usage) |
| Spend-cap budgets pause listed services on estimated gross cost, don't stop persistent resources, reset each period | [Cloud Billing: Manage spend cap budgets](https://docs.cloud.google.com/billing/docs/how-to/budgets-spend-caps) |
| Alerts-only budgets notify and can trigger automation, but don't cap usage | [Google Cloud blog: Protect your spending with budgets](https://cloud.google.com/blog/topics/developers-practitioners/protect-your-google-cloud-spending-budgets) |
| Spot prices change at most once a day | [Compute Engine: Spot VMs](https://docs.cloud.google.com/compute/docs/instances/spot) |