# Platform Decision Register — kubernetes-workload-provisioning

Nov 27, 2026 · @Chief

## How to use this register

22 decisions and 12 design gaps are open; 6 of them (B-1, D-CLASS, D-SCM, D-CI, B-3, B-2) gate most of the rest. Close them in the order in the Resolution order section, not in table order.

Every decision carries five labels:

| Label | Values | Meaning |
| --- | --- | --- |
| Status | OPEN · PROVISIONAL · ESCALATED · DECIDED | PROVISIONAL = conservative default in use until an owner confirms |
| Decider | Core developer · Escalate | Who may close it. Escalate = needs a written answer from compliance, security or the business |
| Reversibility | Reversible · Costly · Irreversible | Irreversible items get the most conservative default while waiting |
| Blocks | Tree / area names | What cannot go live until this closes |
| Evidence | Document + section, or person + date of written reply | What an auditor will ask for |

Record each closed decision with this template:

```markdown
ID:              B-1
Decision:        <the answer in one sentence>
Status:          DECIDED | PROVISIONAL
Source:          <document, section> or <person, role, date of written reply>
Decider:         <name, role>
Alternatives:    <options considered, one line each>
Reversibility:   Reversible | Costly | Irreversible
Revisit trigger: <event that reopens it>
Affected trees:  <list>
```

Rule: a PROVISIONAL default is never cited as compliance evidence. Only DECIDED items with a written source are.

## Time-sensitive security actions

Three actions need no decision and should be done now, before any of the register closes.

| Issue | Action | Why now | Decider |
| --- | --- | --- | --- |
| Java4Shell (CVE-2026-114938) | Inventory running images for OpenJDK 17.0.6+ and 21/25/27; rebuild on vendor-fixed builds; check node pools for x86-64 CPUs with AVX-512 VBMI | Exploitation attempts observed the day after the fix; not in KEV as of 2026-11-20 | Core developer |
| nginx2root (CVE-2026-123007 / 123633) | Upgrade any nginx 1.9.5–1.31.8 with HTTP/2 to 1.30.7 or 1.31.9 in images, ingress and gateways; disable HTTP/2 only where no gRPC depends on it | Public exploit available | Core developer |
| PHP2Shell (CVE-2026-109618 / 109619) | Confirm every PHP image is on a fixed release | Exploited before disclosure; in KEV since 2026-10-26 | Core developer |

## Already settled

These were decided by the core developer during design and are recorded here so every later decision can cite them. Each still needs a written source before it counts as audit evidence.

| ID | Decision | Reversibility |
| --- | --- | --- |
| S-01 | Platform: GKE only (for now) | Costly |
| S-02 | Separate APIs for clusters, cloud resources and workloads | Costly |
| S-03 | Tenancy: namespace per app per environment | Costly |
| S-04 | Argo CD is the single writer to clusters | Costly |
| S-05 | Admission: in-process CEL policies plus one webhook engine | Reversible |
| S-06 | Delivery: canary (Argo Rollouts, traffic shifted by the mesh) | Reversible |
| S-07 | Service mesh required (default: managed Cloud Service Mesh, mTLS STRICT) | Costly |
| S-08 | Secrets stored as Kubernetes Secrets, with application-layer encryption under your own KMS key | Irreversible (key location) |
| S-09 | Certificates: public and private CA | Reversible |
| S-10 | 24/7 paging for production workloads | Reversible (needs staffing, see gaps) |
| S-11 | Approvals are role-based | Reversible |
| S-12 | Government compliance; all logs retained | Irreversible once deleted |
| S-13 | Operators and CRDs installed by the platform team only | Reversible |
| S-14 | Human production access is just-in-time | Reversible |
| S-15 | Images, backup, DR, session length and retention are configurable within platform floors | Reversible |
| S-16 | Observability and on-call ship in phase 1, not later (required by S-12) | Reversible |

## Tier 0 — gates almost everything

These six decide regions, keys, the build trust path and recovery targets. Five of them need answers from outside the platform team; only D-SCM, D-CI and B-3 can be closed by the core developer, and only if nothing is mandated.

### B-1 — Jurisdiction, compliance regime, residency, retention, key custody

- **Question:** Which laws and frameworks apply; where may data, backups, logs and keys live; how long must each be kept; must keys be customer-held or in an HSM; within what time and to whom are incidents reported?
- **Why it matters:** Decides the legal region pair, KMS key locations, audit-log retention, whether any data may leave the country, and the incident-reporting step.
- **Options:** Not a technical choice. Answered by the contract, RFP or security annex, or in writing by the client's compliance officer.
- **Blocks:** Region pair, KMS locations, log and backup retention, CV vulnerability check (processes data in the US), DR design, incident runbooks, data classification.
- **Reversibility:** Irreversible in parts. KMS keys cannot be moved after creation; deleted logs cannot be recovered.
- **Provisional default:** Strictest reading: in-country regions for both primary and DR; regional in-country keys; retain everything in locked buckets; CV vulnerability check off.
- **Decider:** Escalate — compliance / legal.
- **Evidence needed:** Contract or annex section, or a dated written reply.

### D-CLASS — Data classification scheme

- **Question:** What classes of data exist and what handling does each require?
- **Why it matters:** Retention, DR tier, deletion approval, log redaction, ephemeral-environment rules and VEX approvals all key off "data class", which is currently undefined.
- **Options:** Adopt the agency's existing policy (preferred), or approve a new scheme such as public / internal / restricted / secret with a handling table.
- **Blocks:** Provisioning data-safe compensation, DR tiers, decommission, portal log views.
- **Reversibility:** Reversible, but every resource already labelled needs relabelling.
- **Provisional default:** Treat all data as the highest class.
- **Decider:** Escalate — security + compliance.
- **Evidence needed:** The policy document, or written approval of a new one.

### D-SCM — Git host

- **Question:** Which Git host holds platform, policy, template and tenant repositories?
- **Why it matters:** Signed commits, branch protection, scaffolder integration, provenance source-repo allowlists and provisioning rate limits all depend on it.
- **Options:** GitHub (content creation capped near 80 requests/minute and 500/hour, no way to check secondary-limit status); GitLab; another host the agency mandates.
- **Blocks:** Templates, GitOps signing, image provenance, provisioning saga, portal catalog.
- **Reversibility:** Costly — migrating repos, signing keys and integrations.
- **Provisional default:** Whatever the agency already runs; ask before choosing.
- **Decider:** Core developer, unless mandated.
- **Evidence needed:** Agency IT standard, or your decision record.

### D-CI — CI system

- **Question:** Which system builds, signs and attests images?
- **Why it matters:** Binary Authorization's built-in SLSA check trusts only Cloud Build, and manually triggered Cloud Build runs produce no provenance. Any other CI needs your own provenance verifier.
- **Options:** Cloud Build (simplest provenance path); another CI plus a verifier service you build and operate.
- **Blocks:** D-VERIFY, image admission, templates, provisioning step S3.
- **Reversibility:** Costly.
- **Provisional default:** Cloud Build, trigger-invoked only, unless the agency mandates otherwise.
- **Decider:** Core developer + security sign-off.
- **Evidence needed:** Decision record with the provenance design attached.

### B-3 — GKE mode and fleet size

- **Question:** Standard or Autopilot; how many clusters and regions in phase 1?
- **Why it matters:** Standard gives control of node OS (COS), per-tenant node pools and Sandbox pools. Google's launch note said control-plane metrics (admission latency) were not on Autopilot; verify current docs.
- **Options:** Standard; Autopilot.
- **Blocks:** Node security track, observability SLIs, reservations, DR clusters.
- **Reversibility:** Costly — switching mode means new clusters.
- **Provisional default:** Standard, regional clusters, one primary and one DR region.
- **Decider:** Core developer.
- **Evidence needed:** Decision record.

### B-2 — RTO and RPO per tier

- **Question:** For each critical service, how long may it be down and how much recent data may be lost? Same question for the platform itself.
- **Why it matters:** Sets DR topology, backup cadence, reservation spend and the pass/fail line for every drill.
- **Options:** Numbers from a business impact analysis: ask what happens after 15 minutes, 1 hour, 4 hours and 1 day of outage.
- **Blocks:** D-DRTOP, D-DATA, reservations, DR drills.
- **Reversibility:** Reversible, but tighter targets later may need re-architecture.
- **Provisional default:** None. Drills cannot pass or fail without it.
- **Decider:** Escalate — business / service owner.
- **Evidence needed:** Written targets per tier.

## Tier 1 — gates go-live of specific areas

Most of these are yours to close as core developer once Tier 0 is answered; four need escalation (B-4, D-DECLARE, D-BUDGET, and the licensing part of ENG).

| ID | Decision | Decider | Reversibility | Provisional default |
| --- | --- | --- | --- | --- |
| B-4 | Tenant type | Escalate | Costly | Design for external tenants |
| D-SEC | Secret delivery | Core developer | Reversible | External Secrets Operator |
| D-VERIFY | Provenance verifier | Core developer + security | Costly | Verifier service signing a KMS attestation |
| D-SLOT | Unchecked Pod slots | Core developer | Reversible | Forbid for tenants |
| ENG | Policy webhook engine | Core developer + budget | Reversible | Kyverno until licensing confirmed |
| D-BASE | Base image source | Core developer | Reversible | Own minimal builds |
| D-IAC | Cloud resource engine | Core developer | Costly | Config Connector |
| D-WF | Durable workflow engine | Core developer | Costly | Self-hosted until B-1 allows managed |
| D-DATA | Data stores per tier | Core developer + data owners | Costly | Depends on B-2 |
| D-DRTOP | DR topology per tier | Core developer + budget | Costly | Restore-to-pre-created-cluster |
| D-DECLARE | Who declares a disaster | Escalate | Reversible | Two-person, manual for stateful |
| D-TPL / D-LANG | Scaffolder; supported runtimes | Core developer | Reversible | Three-way-update engine; one runtime first |
| D-PORTAL / D-IDP-ROLE | Portal engine; where roles live | Core developer | Costly | Backstage; roles from IdP groups only |
| D-BUDGET | Budget source and overrun approvers | Escalate | Reversible | Every overrun needs approval |

### B-4 — Tenant type

- **Question:** Internal teams only, or external parties (vendors, other agencies, citizens) too?
- **Trade-off:** External tenants require hard isolation: per-tenant node pools, GKE Sandbox for untrusted code, stricter portal exposure.
- **Blocks:** Node pool design, isolation invariants, portal threat model.
- **Provisional default:** Assume external — loosening isolation later is cheap, retrofitting it is not.

### D-SEC — How secret values reach Kubernetes Secrets

- **Question:** Which in-cluster mechanism creates Secrets, given Argo CD must never carry secret values?
- **Options:** External Secrets Operator syncing from a secret store (best for rotation and DR copies); Sealed Secrets (no external store, harder DR and rotation); Secrets Store CSI driver.
- **Why it matters:** Argo CD caches generated manifests in plaintext in Redis, so values must never pass through it.
- **Blocks:** GitOps go-live, DR secret parity, rotation runbooks.

### D-VERIFY — Who verifies provenance and signs the admission attestation

- **Question:** Which component checks builder, exact source repo and signed commit, then signs the attestation Binary Authorization requires?
- **Why it matters:** Binary Authorization enforcement uses attestors; the SLSA and vulnerability checks exist only in continuous validation, which is monitoring-only and Preview.
- **Options:** Verifier service + Cloud Build provenance; verifier service + your CI's provenance; admission webhook verifier (adds a webhook failure domain).
- **Blocks:** Image admission, every "only attested images run" claim.

### D-SLOT — initContainers, native sidecars, image volumes

- **Question:** How are images in Pod slots that Binary Authorization ignores checked?
- **Options:** Forbid for tenants (simplest, most secure); CEL rule requiring those digests to match BA-checked containers or an allowlist; webhook verifier.
- **Why it matters:** BA checks only containers and ephemeralContainers at deploy time.
- **Blocks:** Invariant A01 in the image tree.

### ENG — Policy webhook engine

- **Question:** Policy Controller or Kyverno?
- **Trade-off:** Policy Controller ships NIST 800-53, NIST 800-190, CIS GKE and NSA/CISA bundles, but appears to require GKE Enterprise licensing. Its upstream default failure policy is Ignore and must be set to Fail.
- **Blocks:** Compliance bundles, policy SLIs, runbooks RB-01 to RB-06.
- **Escalation part:** Licensing budget only.

### D-BASE — Base image source

- **Options:** Vendor-hardened catalog (cost, faster patches); own minimal builds (platform team owns patching).
- **Rule:** Runtimes must come from package managers or checksum-verified vendor artifacts, never tarballs, or scanners cannot see them.

### D-IAC — Cloud resource engine behind the Cloud Resource API

- **Options:** Config Connector (fits Argo CD; default deletion policy deletes the cloud resource with the object, so data classes need `abandon`); Terraform (needs plan-review enforcement of the same rules).
- **Blocks:** Provisioning steps S4, data-safe compensation.

### D-WF — Durable workflow engine

- **Why it matters:** Steps run at least once and retry indefinitely by default; exactly-once effects depend on its history and retry semantics.
- **Options:** Managed (only if B-1 allows); self-hosted.
- **Blocks:** Provisioning invariants INV-02, INV-03, INV-11.

### D-DATA and D-DRTOP — Data stores and DR topology per tier

- **Question:** Which managed or self-hosted stores per tier, and active-active, warm standby, pilot light or restore-to-new?
- **Facts to weigh:** Cloud SQL Enterprise Plus switchover is zero-data-loss; replica failover can lose data. Backup for GKE restores only into a pre-created cluster.
- **Depends on:** B-1 (regions), B-2 (targets), budget.

### D-DECLARE — Who may declare a disaster

- **Question:** Which named roles declare and approve failover, and which tiers may fail over automatically?
- **Provisional default:** Two-person declaration; automatic failover only for stateless tiers.

### D-TPL / D-LANG — Scaffolder and runtimes

- **Options:** Portal scaffolder; CLI engine with three-way updates (records template version and answers, enables safe upgrades); own service.
- **Rule:** Templates emit code plus a Workload API request; the scaffolder holds no cluster or cloud credentials.

### D-PORTAL / D-IDP-ROLE — Portal engine and where roles live

- **Options:** Backstage OSS; commercial Backstage; own portal. Roles from IdP groups only, or IdP plus catalog Groups.
- **Rule:** User and Group entities only from the IdP; ownership changes need dual approval.

### D-BUDGET — Budget source of truth

- **Question:** Which system holds team budgets, and which roles approve overruns?
- **Rule:** Thresholds apply to aggregate spend per tenant per period, so split requests cannot bypass approval.

## Tier 2 — design gaps never covered

Twelve areas have no design yet. The first two carry the most risk: today a platform engineer can bypass every control, and the network perimeter is undefined.

| # | Gap | What to design | Why critical | Decider | First step |
| --- | --- | --- | --- | --- | --- |
| G-01 | Platform insider controls | Two-person rule on platform, policy and template repos; separate policy authors and approvers; audited platform-admin actions; break-glass with expiry | Anyone who can change policies, attestor keys, Argo CD or the ledger can bypass all controls | Core developer + security | Branch protection with required second reviewer on platform repos |
| G-02 | Network architecture | VPC layout (Shared VPC or not), VPC Service Controls perimeters around data and keys, private connectivity, egress allowlists, firewall policy hierarchy | Often an explicit government compliance control | Core developer + security | Draw the target topology for one primary and one DR region |
| G-03 | Service-to-service authorization | Mesh authorization policies per service; default deny between namespaces | mTLS proves identity but does not decide who may call whom | Core developer | Add default-deny mesh policy to the policy catalog |
| G-04 | Regulatory incident reporting | Reporting deadlines, recipients, evidence format per B-1 | A missed reporting step can itself be a violation | Escalate (B-1) | Add the question to the compliance questionnaire |
| G-05 | Platform API contracts | Versioning, deprecation policy, compatibility guarantees, contract tests across Cluster, Cloud Resource, Workload, Provisioning and DR APIs | Platform upgrades break tenants without them | Core developer | Pick a versioning scheme and write the deprecation policy |
| G-06 | Staging environment | A nonprod ring mirroring prod policies, admission, mesh and Binary Authorization config | Every game day and policy rollout depends on it | Core developer + budget | Size it and add it to the cost model |
| G-07 | Platform cost model | Reservations (billed at on-demand rate), Policy Controller licensing, Data Access logs, control-plane metrics, multi-region keys and registries, cross-region backup transfer | Unbudgeted defaults get cut later, usually the security ones | Core developer, approval escalated | List every cost-bearing default with a monthly estimate |
| G-08 | 24/7 staffing and support | On-call rotations for platform and security, escalation, tenant support channel, response SLAs | 24/7 paging (S-10) needs people to answer | Escalate | Name the rotation owners |
| G-09 | Tenant offboarding and legal hold | Agency-level exit: access revocation, data export, retention or legal hold, deletion evidence | Only single-service decommission exists today | Core developer + compliance | Write the offboarding state machine |
| G-10 | Platform load testing | Peak load on platform APIs, admission webhooks, Argo CD, provisioning saga | Untested behaviour under real demand | Core developer | Define peak request profiles per API |
| G-11 | Exit strategy | Portability plan off GKE | Commonly asked for in government contracts (unverified for this contract) | Escalate | Ask whether the contract requires one |
| G-12 | AI agents on the platform | Same identity, step-up and approval rules for agents as for humans; treat catalog, docs and tickets as untrusted input | Only relevant if agents will call platform APIs | Core developer | Decide whether agents are in scope at all |

## Irreversible decisions — build on the conservative side

Keep building while answers are pending, but take the strictest option wherever a choice cannot be undone. Relaxing a strict default later is cheap; undoing a loose one is not.

| Item | Why it cannot be undone | Conservative default now | Status |
| --- | --- | --- | --- |
| KMS key locations | Keys cannot be moved after creation | Regional, in-country keys in both primary and DR regions | PROVISIONAL |
| Region pair | Moving data out of a disallowed region is a migration and possibly a breach | Both regions chosen as if strict in-country residency applies | PROVISIONAL |
| Log and backup retention | Deleted evidence cannot be recovered | Keep everything; locked buckets; repos archived, never deleted | PROVISIONAL |
| Data classification | Data handled loosely may already be exposed | Treat all data as the highest class | PROVISIONAL |
| CV vulnerability check | Processes vulnerability data in memory in the US | Off | PROVISIONAL |
| GKE mode | Switching mode means new clusters | Standard, regional | PROVISIONAL |
| Tenant isolation | Retrofitting isolation into shared pools is a migration | Design for external tenants: per-tenant pools, Sandbox-capable | PROVISIONAL |
| Cloud resource deletion policy | A deleted database is gone | Data classes set to `abandon`; Prune and Delete require confirmation | PROVISIONAL |
| Production immutable tags | Overwritten tags lose the original image | Immutable tags on all prod repositories | PROVISIONAL |

Mark each as PROVISIONAL in the register with today's date and the question it waits on. When the answer arrives, confirm the default or change a reversible setting; nothing irreversible will have been done wrong.

## Escalation package

Send this to your tech lead or project manager to route to the client. Keep every written reply; it becomes the Evidence field for the matching decision.

### Message to your lead

> To finish the platform design I need written answers on: data residency and retention rules, the data classification policy, RTO/RPO for critical services, whether external parties will use the platform, who may declare a disaster, and budget for licensing, standby capacity and 24/7 on-call. Until then I am building with conservative defaults (listed in the attached register) and will adjust when answers arrive. The questionnaires below are ready to forward.

### Documents to request first

- [ ] Contract, RFP or tender, SOW, and any security or technical annex
- [ ] Agency information security policy
- [ ] Agency data classification policy
- [ ] Any mandated cloud, region or vendor certification rules
- [ ] Existing enterprise standards: Git host, CI, identity provider, ticketing

### Questionnaire — compliance / legal (B-1, G-04, G-11)

1. Which laws and compliance frameworks apply to this system?
2. Must all data, backups, logs and encryption keys stay inside a specific country or region? Any exceptions?
3. May any data be processed outside the country, even temporarily or in memory?
4. How long must audit logs, application logs and backups be retained? Any legal-hold requirements?
5. Must encryption keys be customer-controlled? Is an HSM required?
6. Within what time must a security incident be reported, to whom, and in what format?
7. Does the contract require a portability or exit plan from the cloud provider?

### Questionnaire — security officer (D-CLASS, B-4, G-01)

1. Do you have a data classification policy? If yes, please share it; if no, who approves one?
2. Will external parties (citizens, vendors, other agencies) deploy or access workloads, or only internal teams?
3. What separation of duties do you require for platform administrators?
4. Are there required certifications or penetration-test cadences for this platform?

### Questionnaire — business / service owner (B-2, D-DECLARE, D-BUDGET, G-08)

1. Which services are critical?
2. For each: what happens after 15 minutes, 1 hour, 4 hours and 1 day down? How much recent data could be lost without serious harm?
3. Who is authorised to declare a disaster and approve failover?
4. What budget exists for standby capacity in a second region, policy licensing, and logging volume?
5. Who staffs 24/7 on-call for the platform and for security?

## Resolution order

Close B-1 first: residency can rule out regions, key locations and Google features that every later choice depends on.

&#91;embedded content: resolution order · 6 steps, 22 decisions plus gaps\]

Steps 1 and 3 need answers from outside the platform team; steps 2, 4, 5 and 6 are mostly yours once those answers exist.

1. **Step 1 — Escalate (B-1, D-CLASS, B-4):** unlocks region pair, key locations, retention, isolation depth and the data-class rules used everywhere else.
2. **Step 2 — You decide (D-SCM, D-CI):** unlocks D-VERIFY, templates and the provisioning saga.
3. **Step 3 — Split (B-3 yours; B-2 and D-BUDGET escalated):** unlocks DR topology, data stores, reservations and node design.
4. **Step 4 — You decide (D-VERIFY, D-SEC, D-SLOT, ENG, D-BASE):** closes the image, GitOps and policy trees.
5. **Step 5 — You plus data owners (D-IAC, D-WF, D-DATA, D-DRTOP, D-DECLARE):** closes provisioning and DR.
6. **Step 6 — Then (D-TPL, D-LANG, D-PORTAL, D-IDP-ROLE, G-01 to G-12):** start the gaps with G-01 (insider controls) and G-02 (network).

Steps 1–3 fit in one working session with compliance, security and the platform lead present.

## Facts behind the defaults

Each provisional default rests on a documented behaviour, checked against vendor or standards pages during design. Re-check before closing a decision; vendor behaviour changes.

| Fact | Drives | Source |
| --- | --- | --- |
| KMS key material is confined to its location and a key cannot be moved after creation | Irreversible key locations | [Cloud KMS locations](https://docs.cloud.google.com/kms/docs/locations) |
| Backup for GKE CMEK keys must be in the BackupPlan's region; restore targets must already exist in the RestorePlan's region | B-1, D-DRTOP | [Backup for GKE locations](https://docs.cloud.google.com/kubernetes-engine/docs/add-on/backup-for-gke/concepts/about-locations) |
| Binary Authorization checks only containers and ephemeralContainers at deploy time; the default policy is ALWAYS\_ALLOW | D-SLOT | [Binary Authorization setup for GKE](https://docs.cloud.google.com/binary-authorization/docs/setting-up) |
| The SLSA check trusts only Cloud Build; manual Cloud Build runs produce no provenance | D-CI, D-VERIFY | [SLSA check](https://docs.cloud.google.com/binary-authorization/docs/cv-slsa-check) |
| Continuous validation is monitoring-only and in Preview; its vulnerability check processes data in memory in the US | B-1, CV default off | [CV overview](https://docs.cloud.google.com/binary-authorization/docs/overview-cv) · [vulnerability check](https://docs.cloud.google.com/binary-authorization/docs/cv-vulnerability-check) |
| GKE encrypts Secrets at rest by default, but application-layer encryption with your key is off by default | S-08 | [GKE hardening](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/hardening-your-cluster) |
| Argo CD stores generated manifests in plaintext in Redis; destination-cluster secret management is recommended | D-SEC | [Argo CD secret management](https://argo-cd.readthedocs.io/en/stable/operator-manual/secret-management/) |
| Gatekeeper's admission webhook defaults to failurePolicy Ignore | ENG | [Gatekeeper failing closed](https://open-policy-agent.github.io/gatekeeper/website/docs/failing-closed/) |
| Config Connector deletes the cloud resource with the object unless deletion-policy is abandon | D-IAC | [Config Connector annotations](https://docs.cloud.google.com/config-connector/docs/reference/annotations) |
| Durable workflow activities run at least once and retry indefinitely by default | D-WF | [Temporal idempotency](https://temporal.io/blog/idempotency-and-durable-execution) |
| GitHub caps content creation near 80/minute and 500/hour; secondary-limit status cannot be checked | D-SCM | [GitHub REST rate limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api) |
| Cloud SQL Enterprise Plus switchover is zero data loss; replica failover can lose data | D-DATA | [Cloud SQL advanced DR](https://docs.cloud.google.com/sql/docs/mysql/use-advanced-disaster-recovery) |
| A global Service Control failure on 2025-06-12 blocked authorised API calls across regions for hours | D-DRTOP, static stability | [Google Cloud incident report](https://status.cloud.google.com/incidents/ow5i3PPK96RduMcb1SsW) |
| Trivy's release pipeline was compromised in March 2026 and backdoored images were published | D-VERIFY, D-CI | [Docker on the Trivy compromise](https://www.docker.com/blog/trivy-supply-chain-compromise-what-docker-hub-users-should-know/) |
