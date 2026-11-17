Two open decisions still change this tree and can't be defaulted away:

- **ENG:** Policy Controller appears to require GKE Enterprise licensing. If that's rejected, Kyverno replaces it, and every Policy Controller metric, alert and runbook in §11–§12 changes.
- **B-1:** your jurisdiction sets evidence retention.

Both stay marked as blockers below. I misread your last message; this is only policy-as-code, merged into one complete tree: architecture, rules, invariants, lifecycle, edge cases, failures and production operations.

```
POLICY-AS-CODE — FINAL COMPLETE TREE
Legend    : [YOU] your decision   [GS] gold-standard default   BLOCKER = open, blocks go-live
            [C V-xx] verified earlier in thread   [L] strong inference, verify in your environment
            cfg.* configurable: floor / default / ceiling + named owner (no invented numbers)
Rule leaf : ID │ Enforcement point │ End-state │ Exceptable │ Detection pair
EP codes  : ORG Org Policy │ CI pipeline │ ACD Argo CD AppProject │ PSA Pod Security
            CEL in-process policy │ PC Policy Controller webhook │ BA Binary Authorization │ CV continuous validation
Severity  : SEV-1..4 (§16)   PAGE = 24/7 on-call   TICKET = business hours
│
├── 0. DECISIONS & GOVERNING PRINCIPLES
│   ├── Decisions
│   │   ├── Admission = in-process CEL + one webhook engine                          [YOU]
│   │   ├── Webhook engine = Policy Controller (NIST/CIS/NSA bundles)                [GS, C V-45]
│   │   │   └── ENG BLOCKER: GKE Enterprise licensing acceptable? NO → Kyverno       [L, V-50]
│   │   ├── Image trust = Binary Authorization                                       [GS, C V-29]
│   │   ├── Cloud guardrails = Organization Policy custom constraints               [GS, C V-46]
│   │   ├── Policy delivery = policy repo → signed commits → Argo CD                [YOU, C V-37]
│   │   └── Evidence retention = per regime                                          [BLOCKER B-1]
│   └── Principles
│       ├── G-1 One primary enforcement point per rule; CI mirrors it                [L]
│       ├── G-2 CRITICAL rules only on paths that cannot fail open                  [C V-02, V-41]
│       ├── G-3 Every enforcing rule has a detection pair                     [C V-04, V-29, V-46]
│       ├── G-4 Rules change only via signed, reviewed commits                       [C V-37]
│       ├── G-5 Tightening staged: Audit → [Warn, Audit] → [Deny, Audit], ring by ring [C V-43]
│       ├── G-6 Exceptions are data in Git, never labels                            [C V-42]
│       └── G-7 The policy system is a tier-0 service with SLOs, alerts, runbooks   [L]
│
├── 1. ENFORCEMENT ARCHITECTURE
│   ├── 1.1 Enforcement points and failure behaviour
│   │   ├── ORG  Cluster / NodePool CREATE+UPDATE; ~2 min to enforce; not retroactive → dry-run [C V-46]
│   │   ├── CI   Every PR (app, platform, policy repos); pipeline down → no merge (fails closed) [L]
│   │   ├── ACD  Repos, destinations, kinds per AppProject; Argo CD down → nothing syncs   [C V-40]
│   │   ├── PSA  In-process; ENFORCE applies to Pods, workloads get warn/audit only        [L]
│   │   ├── CEL  Validating (GA 1.30), mutating (GA 1.36); in-process; set failure policy Fail [C V-02, V-43]
│   │   ├── PC   Webhook; upstream default failurePolicy Ignore → MUST set Fail + HA       [C V-41]
│   │   └── BA/CV Image admission + runtime revalidation; breakglass logged, still flagged [C V-29]
│   ├── 1.2 Layer map
│   │   ├── L0 Cloud: ORG on Cluster API / Cloud Resource API requests
│   │   ├── L1 Shift-left: CI runs the same rules at the same policy-repo SHA
│   │   ├── L2 GitOps boundary: ACD
│   │   ├── L3 Admission: CEL mutate → webhook mutate → PSA → CEL validate → PC → BA  [C V-44]
│   │   ├── L4 Detection: PC audit, CV, Argo CD drift, ORG dry-run
│   │   └── L5 Exceptions registry (Git)
│   └── 1.3 Severity classes → placement
│       ├── CRITICAL → PSA / CEL / BA / ORG only (never webhook-only)
│       ├── HIGH     → PC bundles, failurePolicy Fail, HA
│       ├── MEDIUM   → [Warn, Audit] then Deny after soak
│       └── ADVISORY → Audit only, reported to owners
│
├── 2. RULE CATALOG
│   ├── 2.1 POD SECURITY (PSS restricted)                                           [C V-48]
│   │   ├── POD-01 No privileged containers                       │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-02 No host network / PID / IPC                    │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-03 No hostPath volumes                            │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-04 Drop ALL caps; only NET_BIND_SERVICE re-added  │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-05 runAsNonRoot = true                            │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-06 allowPrivilegeEscalation = false               │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-07 seccomp RuntimeDefault/Localhost, never Unconfined │ PSA │ Deny │ NO │ PC audit
│   │   ├── POD-08 Restricted volume-type allowlist               │ PSA │ Deny │ NO  │ PC audit
│   │   ├── POD-09 Unsafe sysctls, /proc mount, AppArmor/SELinux overrides │ PSA │ Deny │ NO │ PC audit
│   │   ├── POD-10 Pod Security version label pinned              │ CEL │ Deny │ NO  │ namespace scan [C V-04]
│   │   └── POD-11 readOnlyRootFilesystem (not in PSS)           │ CEL │ Deny │ YES │ PC audit [L]
│   ├── 2.2 IMAGES & SUPPLY CHAIN
│   │   ├── IMG-01 Attested by CI                                │ BA  │ Enforce │ breakglass │ CV [C V-29]
│   │   ├── IMG-02 Referenced by digest                          │ CEL │ Deny │ NO  │ PC audit [C V-21]
│   │   ├── IMG-03 Approved registries only                      │ CEL │ Deny │ NO  │ CV
│   │   ├── IMG-04 imagePullPolicy explicit                      │ CEL │ Deny │ YES │ PC audit [C V-20]
│   │   └── IMG-05 Debug images meet IMG-01..03                  │ BA+CEL │ Deny │ NO │ CV
│   ├── 2.3 IDENTITY & ACCESS
│   │   ├── RBAC-01 Subjects are IdP groups, never users         │ CEL │ Deny │ NO  │ RBAC scan [C V-28]
│   │   ├── RBAC-02 No nodes/proxy for humans or monitoring      │ CEL │ Deny │ NO  │ RBAC scan [C V-25]
│   │   ├── RBAC-03 exec / ephemeralcontainers only in JIT roles │ CEL │ Deny │ NO  │ can-i matrix [C V-24, V-26]
│   │   ├── RBAC-04 Tenants can't grant escalate / bind / impersonate │ CEL │ Deny │ NO │ RBAC scan [L]
│   │   ├── SA-01  Workload identity only to API-issued service accounts │ CEL │ Deny │ NO │ IAM scan
│   │   └── SA-02  Token automount off unless declared           │ CEL │ Deny │ YES │ PC audit [L]
│   ├── 2.4 TENANCY & GOVERNANCE
│   │   ├── TEN-01 owner, cost-center, env, request-ID labels (≤63) │ CEL │ Deny │ NO │ label report [C V-11]
│   │   ├── TEN-02 No cluster-scoped kinds from tenants          │ ACD+CEL │ Deny │ NO │ ACD audit [C V-40]
│   │   ├── TEN-03 Tenants can't edit Quota / LimitRange / NetworkPolicy │ ACD+CEL │ Deny │ NO │ ACD audit
│   │   ├── TEN-04 CPU/memory requests on every container        │ CEL │ Deny │ NO  │ quota events [C V-03]
│   │   ├── TEN-05 No Applications in Argo CD "default" project  │ CEL │ Deny │ NO  │ ACD inventory
│   │   └── TEN-06 Exemption labels set only by Workload API identity │ PC label webhook (Fail)+CEL │ Deny │ NO │ alert [C V-41]
│   ├── 2.5 NETWORK & MESH
│   │   ├── NET-01 Default-deny NetworkPolicy per tenant namespace │ READY gate │ block │ NO │ namespace scan
│   │   ├── NET-02 mTLS STRICT in prod; weakening denied         │ CEL │ Deny │ NO  │ plaintext = 0 [C V-30]
│   │   ├── NET-03 No EnvoyFilter / WasmPlugin                   │ CEL │ Deny │ NO  │ inventory [C V-30]
│   │   ├── NET-04 No tenant LoadBalancer / NodePort Services    │ CEL │ Deny │ YES │ PC audit [L]
│   │   └── NET-05 Unique ingress/gateway hostnames (referential) │ PC │ Deny │ NO │ PC audit [L]
│   ├── 2.6 DELIVERY & GITOPS
│   │   ├── GIT-01 Prod commits signed by allowed keys           │ ACD │ Refuse sync │ NO │ ACD events [C V-37]
│   │   ├── GIT-02 PVC/Secret/IAM objects carry Prune/Delete=confirm │ CI+CEL │ Deny │ NO │ manifest scan [C V-38]
│   │   ├── GIT-03 No replicas in Git when autoscaler targets object │ CI │ Fail │ NO │ flapping alert [C V-12]
│   │   ├── GIT-04 Prod Rollouts declare canary analysis         │ CEL │ Deny │ YES │ inventory [C V-31]
│   │   └── GIT-05 FailOnSharedResource on every Application     │ CEL │ Deny │ NO  │ ACD inventory [C V-38]
│   ├── 2.7 SECRETS (D-SEC open)
│   │   ├── SEC-01 No plaintext Secret data in Git                │ CI │ Fail │ NO │ repo scan [C V-36]
│   │   ├── SEC-02 No secret-injection generation plugins        │ ACD config │ block │ NO │ config review [C V-36]
│   │   └── SEC-03 No subPath mounts of rotating secrets         │ CEL │ Deny │ YES │ PC audit [C V-15]
│   ├── 2.8 STORAGE & DATA
│   │   ├── STO-01 Zonal StorageClasses: WaitForFirstConsumer    │ CEL │ Deny │ NO │ SC inventory [C V-13]
│   │   ├── STO-02 PVCs carry retention-decision label           │ CEL │ Deny │ NO │ PVC report [C V-07]
│   │   └── STO-03 Stateful workloads covered by a backup plan   │ CEL/PC │ Deny │ YES │ coverage report [C V-33]
│   ├── 2.9 AVAILABILITY
│   │   ├── AVL-01 PDB must allow ≥1 disruption                  │ CEL │ Deny │ NO │ drain test [C V-06]
│   │   ├── AVL-02 PDB unhealthy-pod policy AlwaysAllow          │ CEL │ Deny │ YES │ PC audit [C V-06]
│   │   └── AVL-03 HPA and VPA not on same resource metric       │ CEL │ Deny │ NO │ oscillation alert [C V-14]
│   └── 2.10 CLOUD LAYER
│       ├── CLD-01 Private nodes + private endpoint              │ ORG │ Deny │ NO │ posture scan [C V-28]
│       ├── CLD-02 Shielded nodes + secure boot                  │ ORG │ Deny │ NO │ posture scan [C V-28]
│       ├── CLD-03 Workload Identity Federation                  │ ORG │ Deny │ NO │ posture scan [C V-28]
│       ├── CLD-04 App-layer secrets encryption, your KMS key    │ ORG │ Deny │ NO │ posture scan [C V-34]
│       ├── CLD-05 Bound to required CV platform policy          │ ORG │ Deny │ NO │ posture scan [C V-46]
│       └── CLD-06 Enrolled in a release channel                 │ ORG │ Deny │ NO │ posture scan [C V-28]
│
├── 3. PER-REQUEST EVALUATION FLOW (one workload change)
│   ├── 3.1 Before the cluster
│   │   ├── CI: G-1 mirror rules on rendered manifests (incl. Pod templates vs PSA)
│   │   │   ├── FAIL ──> PR blocked: rule ID + fix + docs link
│   │   │   └── PASS ──> signed, role-reviewed merge                              [C V-37]
│   │   └── ACD: project boundary (repos, destinations, kinds)                    [C V-40]
│   │       ├── FAIL ──> sync refused
│   │       └── PASS ──> apply to API server
│   ├── 3.2 Exemption check
│   │   ├── Platform namespace AND platform identity ──> platform-scoped rules only
│   │   ├── Platform namespace, other identity ──> [DENY]
│   │   └── Tenant ──> full rule set
│   ├── 3.3 Mutation (adds safe defaults only)
│   │   ├── CEL mutating (1.36+) ──> seccomp default, token automount off         [C V-43]
│   │   └── Webhook mutating (mesh injection) — idempotent, reinvocation-aware   [C V-42]
│   ├── 3.4 Pod Security (Pods)
│   │   ├── FAIL ──> Pod denied; Deployment already exists → ReplicaFailure → FAILED [C V-03]
│   │   └── PASS ──> continue
│   ├── 3.5 CEL validating
│   │   ├── Deny binding + violation ──> [DENY rule ID + message]
│   │   ├── Warn/Audit binding ──> admit + warning header / audit annotation    [C V-43]
│   │   └── Expression error ──> failure policy Fail ──> [DENY] + AL-11
│   ├── 3.6 Policy Controller
│   │   ├── Violation ──> [DENY]
│   │   ├── Timeout / unreachable ──> Fail ──> [DENY] + AL-02                   [C V-41]
│   │   └── PASS ──> continue
│   ├── 3.7 Binary Authorization (Pod create)
│   │   ├── Not attested ──> [DENY]
│   │   └── Breakglass ──> admit + audit event + AL-08                          [C V-29]
│   └── 3.8 Persisted ──> PC audit + CV re-check continuously
│
├── 4. INVARIANTS
│   ├── P0-01 No critical rule on a fail-open path
│   │   ├── Mechanism: PC webhook default Ignore admits when unreachable          [C V-41]
│   │   ├── Enforce: CI fails CRITICAL rule not in PSA/CEL/BA/ORG
│   │   └── Enforce: cluster READY requires PC failurePolicy Fail, replicas ≥ cfg.policy.min_replicas,
│   │       PDB, zone spread
│   ├── P0-02 Policy engine can't lock out its own recovery
│   │   ├── Mechanism: fail-closed webhook blocks objects needed to restore it    [C V-47]
│   │   ├── Mechanism: namespace selectors don't exempt cluster-scoped objects  [C V-42]
│   │   ├── Enforce: exclude kube-system, gatekeeper-system, mesh control plane, Argo CD
│   │   ├── Enforce: never match Nodes or webhook configurations
│   │   └── Enforce: tested break-glass (RB-02), two-person, audited, auto-restore
│   ├── P0-03 Exemptions can't be self-granted
│   │   ├── Mechanism: object-selector webhooks bypassable by labels             [C V-42]
│   │   ├── Enforce: no objectSelector on enforcing webhooks
│   │   ├── Enforce: label-protection webhook stays Fail                        [C V-41]
│   │   └── Enforce: exemptions only from Git registry (§6)
│   ├── P0-04 Every prevention has a detection
│   │   ├── PSA change ──> audit scan before Deny                                [C V-04]
│   │   ├── BA policy ──> matching CV platform policy                           [C V-29]
│   │   ├── ORG constraint ──> dry-run first                                     [C V-46]
│   │   ├── PC constraint ──> audit reviewed before Deny
│   │   └── CI fails any enforcing rule without a detection mapping
│   ├── P0-05 Policy changes signed, staged, reversible
│   │   ├── Signed commit + security approval                                    [C V-37]
│   │   ├── Tightening: Audit → [Warn, Audit] → [Deny, Audit] per ring           [C V-43]
│   │   ├── Note: Deny and Warn can't be combined in one CEL binding             [C V-43]
│   │   ├── Loosening: security approval + audit event; never from tenant repos
│   │   └── Rollback: revert commit through Argo CD
│   └── P0-06 CI and admission evaluate identical rules
│       └── Both pinned to the same policy-repo SHA; mismatch fails CI           [L]
│
├── 5. RULE LIFECYCLE (state machine per rule ID)
│   ├── DRAFT ──> CI_ONLY (PR comments only)
│   ├── CI_ONLY ──> AUDIT (violations counted per owner)                        [C V-43]
│   ├── AUDIT ──> WARN ([Warn, Audit])                                           [C V-43]
│   ├── WARN ──> DENY ([Deny, Audit]) ring by ring: nonprod → prod
│   │   └── Guard: running violations in ring = 0 AND SLI-7 within target
│   ├── Any state ──> ROLLED_BACK (revert; reason recorded)
│   └── DENY ──> RETIRED (replacement proven; security approval)
│
├── 6. EXCEPTIONS
│   ├── Request: PR {rule ID, single workload scope, justification, expiry}
│   ├── Exceptable = NO? ──> CI rejects automatically
│   ├── Non-exceptable set: POD-01..09, IMG-01 (except BA breakglass), IMG-02/03, RBAC-01..04,
│   │   NET-02/03, TEN-02/03/06, CLD-*
│   ├── Approval: owner lead + security; incident commander for Sev-1               [YOU]
│   ├── Expiry ≤ cfg.exception.max_ttl, else rejected
│   ├── Active: rendered into CEL binding params / PC exclusions; weekly report
│   ├── Expiry: controller removes; workload must comply or re-request
│   └── Images: BA breakglass only; CV flags until pod ends; review ≤ cfg.breakglass.review [C V-29]
│
├── 7. CHANGE MANAGEMENT
│   ├── Classes
│   │   ├── C1 Tightening / new bundle / wider scope ──> full lifecycle
│   │   ├── C2 Messages, docs, metadata ──> normal review
│   │   ├── C3 Loosening / narrowing / exemption ──> security approval + audit event
│   │   └── C4 Emergency ──> two-person, time-boxed, retro review
│   ├── C1 PR must include
│   │   ├── Golden fixtures: ≥1 allow and ≥1 deny case per rule
│   │   ├── Replay result on live inventory: projected denies by owner
│   │   ├── Blast radius (rings, namespaces), detection pair, revert SHA
│   │   └── Owner notice plan before WARN and before DENY
│   ├── Progression: AUDIT cfg.rollout.audit_soak → WARN cfg.rollout.warn_soak → DENY
│   ├── Abort: AL-04, SLI-1/2 breach, or upheld owner escalation
│   └── Scheduling: no C1 promotion during freezes / Argo CD deny windows        [C V-40]
│
├── 8. TESTING
│   ├── Unit: each CEL expression / constraint template with fixtures
│   ├── Golden fixtures versioned with each rule ID
│   ├── Rendered Pod templates evaluated against PSA in CI (closes accepted-but-denied gap)
│   ├── Nightly replay against live inventory snapshot
│   ├── Version conformance: full suite on next GKE version before upgrade
│   ├── Load: webhook at cfg.capacity.peak_admission_rps
│   └── Game days
│       ├── Kill all PC pods ──> critical rules still deny
│       ├── Break referential sync ──> AL-06; promotions blocked
│       ├── Unattested image ──> BA deny; breakglass ──> AL-08
│       ├── Unsigned policy commit ──> Argo CD refuses; AL-09                  [C V-37]
│       └── RB-02 lockout procedure end-to-end in nonprod
│
├── 9. EDGE CASES
│   ├── E-01 Deployment accepted, Pods denied by PSA → zero pods        → CI Pod-template check + READY gate
│   ├── E-02 Rule tightened, existing pods violate                    → Audit counts, owner tickets, hold Deny
│   ├── E-03 Tenant adds exemption label                              → TEN-06 + Fail label webhook + alert
│   ├── E-04 Breakglass image during incident                         → CV flags; mandatory review
│   ├── E-05 Two mutators write same path                             → CI ownership map; Argo CD ignore per path
│   ├── E-06 Cluster-scoped kinds not exempt by namespace             → explicit kind lists; exclude recovery kinds
│   ├── E-07 Org policy applied seconds before cluster create         → posture scan before READY
│   ├── E-08 Cluster below 1.36 (no CEL mutation)                     → webhook mutation fallback, tracked
│   ├── E-09 Referential data stale                                   → AL-06; referential rules never CRITICAL
│   ├── E-10 Bundle upgrade adds constraints                          → treated as C1, starts at AUDIT
│   ├── E-11 CI and cluster on different rule versions                → same SHA pin; mismatch fails CI
│   ├── E-12 Emergency fix blocked by new rule                        → §6 exception (if exceptable) or comply
│   ├── E-13 Webhook latency near timeout under deploy wave           → RB-01; never switch to Ignore
│   ├── E-14 CEL expression hits missing/optional field               → guarded expressions; fixture added
│   └── E-15 Policy Controller licensing rejected mid-project          → ENG fallback to Kyverno; §11–12 remapped
│
├── 10. FAILURE MODES
│   ├── F-01 PC pods down + Fail [OUTAGE of writes] → HA, PDB, exclusions → AL-02 → RB-02
│   ├── F-02 PC left at Ignore [SECURITY | COMPLIANCE] → READY gate → AL-03 → RB-03
│   ├── F-03 Bad CEL rule pushed to Deny [OUTAGE] → lifecycle forbids skipping stages → AL-04 → RB-04
│   ├── F-04 Policy repo compromised [SECURITY] → signed commits, review → AL-09 → RB-08
│   ├── F-05 Detection pair missing [COMPLIANCE] → CI check → evidence sampling
│   ├── F-06 Org policy disabled higher in hierarchy [SECURITY] → inheritance check in posture scan
│   ├── F-07 Audit stopped [COMPLIANCE] → AL-05 → RB-05
│   ├── F-08 Evidence sink broken [COMPLIANCE] → AL-13 → RB-12
│   └── F-09 Expired exception still active [SECURITY] → AL-12 → RB-11
│
├── 11. SLIs & SLOs
│   ├── SLI-1 Webhook latency: apiserver_admission_webhook_admission_duration_seconds p99
│   │   per webhook name < cfg.slo.webhook_p99                                   [C V-49]
│   │   └── Requires GKE control-plane metrics (reportedly not on Autopilot → B-3) [L]
│   ├── SLI-2 Webhook health: gatekeeper_validation_request_count by admission_status
│   │   error share < cfg.slo.webhook_error_ratio                               [C V-50]
│   ├── SLI-3 Audit freshness: now − gatekeeper_audit_last_run_end_time < cfg.slo.audit_staleness [C V-50]
│   ├── SLI-4 Referential convergence: watched_gvk == intended_watch_gvk         [C V-50]
│   ├── SLI-5 Violations: gatekeeper_violations by enforcement_action → 0 within SLA [C V-50]
│   ├── SLI-6 Runtime image conformance: CV log entries = 0 (non-breakglass)    [C V-29]
│   ├── SLI-7 Deny precision: overturned denies ÷ all denies < cfg.slo.false_deny_ratio [L]
│   ├── SLI-8 Exception debt: expired-but-active = 0; p90 age < cfg.exception.p90_age
│   └── SLI-9 Evidence completeness: 100% of sampled controls traceable
│
├── 12. ALERTS → RUNBOOKS
│   ├── AL-01 Webhook p99 high → PAGE → RB-01
│   │   └── RB-01: identify webhook → check pods/throttling → check recent constraint/sync change →
│   │       scale → narrow scope → revert last change → never set Ignore → move hot rule to CEL
│   ├── AL-02 Webhook errors / unavailable → PAGE → RB-02
│   │   └── RB-02: scope of blocked writes → pod cause (capacity, crash, cert, network) → fix
│   │       └── Lockout or > cfg.breakglass.webhook_max: two-person approval → remove webhook config →
│   │           PSA/CEL/BA keep enforcing → Argo CD deny window for tenants → restore via Argo CD →
│   │           full audit run → lift freeze → document gap as evidence
│   ├── AL-03 failurePolicy ≠ Fail → PAGE → RB-03
│   │   └── RB-03: treat as SEV-2 → restore Fail via Argo CD → find writer in audit logs →
│   │       audit all objects admitted during drift window
│   ├── AL-04 Deny spike after policy commit → PAGE → RB-04
│   │   └── RB-04: rule ID + ring + commit → false positives? revert : roll ring back to [Warn, Audit]
│   │       → owner tickets → review why AUDIT/WARN missed it
│   ├── AL-05 Audit stale → TICKET → RB-05
│   │   └── RB-05: audit pod health, duration vs interval → tune per capacity → log staleness window
│   ├── AL-06 GVK not converged → TICKET → RB-06
│   │   └── RB-06: missing kinds → fix sync config/RBAC/memory → block Deny promotions until converged
│   ├── AL-07 CV violation (non-breakglass) → PAGE security → RB-07
│   ├── AL-08 Breakglass image deployed → PAGE security → RB-07
│   │   └── RB-07: pods, digests, deploy path → incident-linked? review : SEV-1 (isolate via network
│   │       policy, preserve evidence, rotate reachable credentials) → replace with attested image
│   ├── AL-09 Loosening commit / unknown signer / force-push on policy repo → PAGE security → RB-08
│   │   └── RB-08: freeze policy sync → verify signer, reviewer, diff → malicious? SEV-1 (revoke keys,
│   │       revert, full audit) : revert + fix branch protection
│   ├── AL-10 Org policy blocks Cluster API → TICKET → RB-09
│   │   └── RB-09: read constraint name → fix template or correct constraint (dry-run first) → wait
│   │       for propagation → retest
│   ├── AL-11 CEL expression errors → PAGE → RB-10
│   │   └── RB-10: policy + failing object shape → guard fields / revert → add fixture
│   ├── AL-12 Expired exception active → TICKET → RB-11
│   │   └── RB-11: remove rendering → notify owner → record overrun window
│   ├── AL-13 Evidence pipeline gap → PAGE → RB-12
│   │   └── RB-12: sink errors / bucket lock / Data Access logs → restore → quantify gap → notify
│   │       compliance owner                                                     [C V-32]
│   └── AL-14 Workload accepted, pods denied → TICKET owner → RB-13
│       └── RB-13: ReplicaFailure reason → rule ID → owner PR fix → add CI coverage
│
├── 13. DEVELOPER EXPERIENCE CONTRACT
│   ├── Deny message: rule ID, what failed, how to fix, docs link, exception path or "not exceptable"
│   ├── Same rules runnable locally and in CI
│   ├── Docs page per rule ID: rationale, control mapping, examples, owner
│   ├── Exception PR template; security review SLA cfg.exception.review_sla
│   └── Monthly deny report per team
│
├── 14. OPERATING CADENCE
│   ├── Daily: deny/violation dashboard; breakglass and CV follow-ups
│   ├── Weekly: exception review; violation burn-down per owner
│   ├── Monthly: game day; SLI-7 per rule; bundle update assessment
│   ├── Quarterly: control-mapping review; auditor dry run; break-glass credential rotation
│   └── Per GKE upgrade: conformance suite; exec/RBAC matrix; mutation version gates [C V-24, V-43]
│
├── 15. CAPACITY & PERFORMANCE
│   ├── Inputs: peak admission RPS, object inventory size, synced GVK count
│   ├── Webhook: replicas, PDB, zone spread, resource requests sized to peak
│   ├── Audit duration < audit interval (SLI-3)
│   ├── Referential sync memory scales with synced objects (SLI-4)
│   └── Metrics ingestion billed per sample                                      [C V-49, V-50]
│
├── 16. SEVERITY MATRIX
│   ├── SEV-1 Unauthorized image or privileged workload in prod; policy repo compromise;
│   │   compliance evidence lost
│   ├── SEV-2 Enforcement gap without break-glass record; prod deploys blocked fleet-wide
│   ├── SEV-3 Single-ring false-positive blocks; audit stale beyond SLO
│   └── SEV-4 Advisory noise; documentation gaps
│
├── 17. UPGRADES
│   ├── GKE: conformance in nonprod ring first; Pod Security pin checked         [C V-04]
│   ├── Policy Controller: canary one nonprod cluster; replay inventory, expect zero decision diff
│   ├── Bundles (NIST/CIS/NSA): C1 change, full lifecycle                        [C V-45]
│   └── Mesh / Argo CD: rerun NET-* and GIT-* fixtures
│
├── 18. ONBOARDING
│   ├── New cluster: ORG passes → policy stack synced → failurePolicy Fail verified →
│   │   game-day subset → READY                                                  [C V-41, V-46]
│   └── New tenant: baseline rules in Deny from day one; rule docs + local tooling to owners
│
├── 19. EVIDENCE CHAIN
│   ├── Control (NIST 800-53 family etc.) → rule IDs                              [C V-45]
│   ├── Rule ID → enforcement point + mode + ring state (policy-repo SHA)
│   ├── Enforcement → audit-log denies, warns, breakglass events                  [C V-32]
│   ├── Detection → PC audit, CV logs, posture scans, ORG dry-run reports
│   ├── Exceptions → registry entries with approver + expiry
│   ├── Gaps → incident-linked windows (RB-02, RB-03, RB-12)
│   ├── Retention → locked buckets, period per regime                             [BLOCKER B-1]
│   └── Drill: auditor samples controls → trace within cfg.audit.trace_time (SLI-9)
│
├── 20. OWNERSHIP
│   ├── Security: rule content, exceptable flags, loosening, bundles, exceptions
│   ├── Platform: enforcement points, HA, lifecycle tooling, alerts, runbooks, break-glass
│   ├── App owners: remediation within cfg.remediation.sla
│   └── Auditors: read-only access to evidence chain
│
├── 21. ROADMAP (exit gates, no dates)
│   ├── PHASE 0 Critical rules in-process: PSA, CEL (labels, RBAC, mesh bans), BA + CV, ORG
│   │   └── EXIT: all PC pods killed → critical rules still deny
│   ├── PHASE 1 Bundles + CI parity: PC (or Kyverno per ENG) with Fail + HA; same rules in CI
│   │   └── EXIT: bundle audit evidence lands in locked bucket
│   ├── PHASE 2 Operations: SLIs, alerts, runbooks, exceptions registry, staged rollout tooling
│   │   └── EXIT: RB-02 and emergency-exception drills pass
│   └── PHASE 3 In-process mutation + continuous evidence (fleet on 1.36+)
│       └── EXIT: auditor traces any control → rule → enforcement → evidence
│
└── 22. GO-LIVE GATES
    ├── ENG and B-1 answered
    ├── PC failurePolicy Fail verified on every prod cluster
    ├── Every CRITICAL rule on an in-process path; every enforcing rule has a detection pair
    ├── All game days in §8 passed in nonprod
    ├── Alerts AL-01..14 routed with runbooks tested
    └── Evidence drill (SLI-9) passed
```

[Certain] Every `[C V-xx]` tag traces to the sources in earlier replies; this tree introduces no new claims. To close it, answer **ENG** (is GKE Enterprise licensing acceptable, or switch to Kyverno?) and **B-1** (which jurisdiction and regime). No codebase has been changed.