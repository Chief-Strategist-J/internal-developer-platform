The pasted template assumes tools you haven't chosen: Node, Go, Python and Java; SonarQube, Trivy, Datadog and Slack. It also lets the scaffolder create repos, webhooks and dashboards directly. In your platform that breaks two settled rules: the Workload API is the single entry point, and Argo CD is the single writer.

I disagree with mirroring it because a scaffolder that provisions things itself becomes a second, unaudited control plane. It's also a high-value attack target: Backstage has had 2026 vulnerabilities in exactly this area. Here's what I'd do instead: templates only produce a repo plus a Workload API request, and every template output must pass the policy rule catalog by construction. The risk in the template's approach is golden paths that quietly generate non-compliant services and bypass your approval flow.

Three decisions you haven't made yet block this tree:

- **D-SCM:** which Git host.
- **D-CI:** which CI system.
- **D-TPL:** which scaffolding engine.

They're listed as blockers at the top, not defaulted.

```
SERVICE TEMPLATES — FINAL COMPLETE TREE
Legend   : [YOU] your decision   [GS] gold-standard default   BLOCKER = open, blocks go-live
           [C V-xx] verified (V-51, V-52 below; others earlier in thread)   [L] strong inference
           cfg.* configurable: floor / default / ceiling + named owner (no invented numbers)
           Rule IDs (POD-, IMG-, TEN-, GIT-, NET-, SEC-, STO-, AVL-) = policy-as-code final tree
Failure  : Scenario → Impact → Handling → Detection
│
├── 0. DECISIONS & PRINCIPLES
│   ├── Open decisions
│   │   ├── D-SCM  Git host (GitHub / GitLab / other) — repo creation, rulesets, bot identity  [BLOCKER]
│   │   ├── D-CI   CI system — must build, sign, attest for Binary Authorization              [BLOCKER]
│   │   ├── D-TPL  Scaffolding engine: portal scaffolder | CLI engine with 3-way updates | own service [BLOCKER]
│   │   ├── D-LANG Supported runtimes for golden paths (each = one template family)          [BLOCKER]
│   │   └── D-SEC  Secret delivery (from GitOps tree) — templates emit references only         [BLOCKER]
│   └── Principles
│       ├── T-1 Templates produce code + a Workload API request; they never provision directly [YOU, L]
│       ├── T-2 Every rendered output passes the policy rule catalog at the same policy SHA   [L]
│       ├── T-3 The scaffolder is a privileged system: sandboxed, least-privilege, reviewed   [C V-52]
│       ├── T-4 Templates change only via signed, reviewed commits and version tags          [C V-37]
│       ├── T-5 Every service records its template lineage (template ID, version, answers)  [C V-51]
│       └── T-6 Upgrades never overwrite customizations; conflicts go to humans             [C V-51]
│
├── 1. ARCHITECTURE
│   ├── Template repo(s): one family per runtime (D-LANG); semver tags
│   ├── Template registry: approved versions, status (ACTIVE | DEPRECATED | YANKED), owners
│   ├── Scaffolder: renders in a sandbox; no network except allowlisted Git host;
│   │   no cluster or cloud credentials at all                                         [L]
│   ├── Repo creation: via D-SCM with a narrowly scoped bot identity                   [C V-52]
│   ├── Handoff: scaffolder submits a Workload API request (idempotency key) → normal
│   │   validation, approval, provisioning, Argo CD sync                                [YOU]
│   └── Lineage store: service ↔ template ID ↔ version SHA ↔ answers
│
├── 2. GOLDEN PATH CONTRACT (every template must emit all of these)
│   ├── 2.1 Build & image
│   │   ├── Container runs non-root, no added capabilities, seccomp default     → POD-04/05/06/07
│   │   ├── readOnlyRootFilesystem with explicit writable mounts                 → POD-11
│   │   ├── Base image pinned by digest from approved registry                  → IMG-02/03
│   │   ├── CI pipeline (D-CI): build → test → scan → SBOM → sign → attest        → IMG-01
│   │   └── imagePullPolicy explicit                                            → IMG-04
│   ├── 2.2 Runtime manifests
│   │   ├── Argo Rollouts canary with analysis template                          → GIT-04
│   │   ├── CPU/memory requests (and limits per platform floor)                  → TEN-04
│   │   ├── Labels: owner, cost-center, env, request-ID                          → TEN-01
│   │   ├── No replicas in Git when an autoscaler is generated                   → GIT-03
│   │   ├── PDB allowing ≥1 disruption, AlwaysAllow unhealthy eviction          → AVL-01/02
│   │   ├── App-level NetworkPolicy allows on top of default-deny               → NET-01
│   │   ├── Mesh-ready; no EnvoyFilter / WasmPlugin                              → NET-02/03
│   │   ├── Probes (readiness, liveness, startup where needed)                   [L]
│   │   └── Stateful variants only: retention label + backup plan selector      → STO-02/03
│   ├── 2.3 Secrets & config
│   │   ├── Secret references only, per D-SEC; zero secret values or defaults   → SEC-01/02
│   │   ├── No subPath mounts for rotating secrets                               → SEC-03
│   │   └── Config separated from code; no environment-specific values baked in [L]
│   ├── 2.4 Operability
│   │   ├── Telemetry wired to your chosen backends (DEC-15 from v4)             [L]
│   │   ├── Default SLOs, alerts and dashboards as code, routed to owner        [L]
│   │   ├── Runbook skeleton linked from every generated alert                   [L]
│   │   └── Ownership metadata for catalog (owner group, tier, on-call)
│   └── 2.5 Repo governance
│       ├── Branch protection / rulesets: reviews, signed commits, required checks [C V-37, L]
│       ├── CODEOWNERS including platform for platform-owned paths            [L]
│       └── Lineage file committed and protected from manual edits            [C V-51]
│
├── 3. TEMPLATE PUBLISHING TIER [P0]
│   └── Trigger: signed tag on a template repo
│       ├── Check: Tag signed by an allowed platform key; change reviewed by platform + security?
│       │   ├── NO ──> [REJECT publish]
│       │   └── YES ──> continue
│       ├── Check: Secret scan on template repo clean?
│       │   ├── NO ──> [HARD REJECT]
│       │   └── YES ──> continue
│       ├── Check: Uses unsafe engine features (tasks, migrations, extensions, external reads)? [C V-51]
│       │   ├── YES and not on platform allowlist ──> [REJECT]
│       │   └── NO / allowlisted ──> continue
│       ├── Check: Symlinks or paths escaping the output root?                     [C V-52]
│       │   ├── YES ──> [REJECT]
│       │   └── NO ──> continue
│       ├── Action: Render with every fixture answer set (each variant: stateless, worker,
│       │   batch, stateful)
│       ├── Check: Rendered output passes full policy suite at current policy SHA?
│       │   ├── NO ──> [REJECT with failing rule IDs]
│       │   └── YES ──> continue
│       ├── Check: Rendered project builds, tests pass, image attests in sandbox CI?
│       │   ├── NO ──> [REJECT]
│       │   └── YES ──> continue
│       └── State: ACTIVE in registry (previous version → DEPRECATED per support policy)
│
├── 4. INSTANTIATION TIER [P0]
│   └── Trigger: owner requests a new service from template T@version
│       ├── 4.1 Authorization
│       │   ├── Check: Requester in a group allowed to run T?                         [C V-52]
│       │   ├── Check: Requester in the owner group named in the request?
│       │   └── ANY NO ──> [REJECT 403]
│       ├── 4.2 Parameter validation
│       │   ├── Schema-typed answers; strings against strict allowlist regex
│       │   ├── Names: DNS-label rules, ≤63, reserved-name list, uniqueness check   [C V-11]
│       │   ├── Missing variables fail rendering (strict undefined)                [C V-51]
│       │   └── ANY FAIL ──> [REJECT 422 with field errors]
│       ├── 4.3 Rendering (sandbox)
│       │   ├── No network beyond template fetch; no credentials mounted           [L]
│       │   ├── Unsafe features off unless allowlisted for T                       [C V-51]
│       │   ├── Output path containment + symlink check                            [C V-52]
│       │   └── Render error ──> [ABORT; no repo created]
│       ├── 4.4 Verification
│       │   ├── Policy suite on rendered output (same policy SHA as clusters)
│       │   ├── Build + unit tests in sandbox CI
│       │   └── FAIL ──> [ABORT; template defect ticket to platform]
│       ├── 4.5 Materialization
│       │   ├── Write intent to ledger (request ID)                               [L]
│       │   ├── Create repo with protections via scoped bot identity              [C V-52]
│       │   ├── Initial commit signed by bot key listed for the project           [C V-37]
│       │   ├── Record lineage: template ID, version SHA, answers (no credentials) [C V-51]
│       │   └── Repo creation fails ──> retry idempotently; never a half-created repo left untracked
│       ├── 4.6 Handoff
│       │   ├── Submit Workload API request (namespace per app per env, quotas, project)
│       │   ├── Normal approval by role                                           [YOU]
│       │   ├── Rejected ──> repo marked PENDING_ARCHIVE; owner notified
│       │   └── Approved ──> provisioning tree → Argo CD sync → first canary
│       └── State: SCAFFOLDED → FIRST_DEPLOY_READY (only when workload READY is observed)
│
├── 5. UPGRADE TIER [P1, P0 for security patches]
│   └── Trigger: T@v2 published
│       ├── Action: Query lineage for services on older versions
│       ├── Action: Per service, re-render old and new versions with stored answers and
│       │   apply as a three-way merge into the service repo                    [C V-51]
│       ├── Check: Conflicts?
│       │   ├── YES ──> PR marked NEEDS_MANUAL_RESOLUTION; never force-push      [C V-51]
│       │   └── NO ──> clean PR
│       ├── Check: PR passes CI + policy suite?
│       │   ├── NO ──> PR blocked; template defect or service divergence flagged
│       │   └── YES ──> owner review (signed bot commit, normal branch rules)   [C V-37]
│       ├── Classes
│       │   ├── Routine ──> owner merges within cfg.upgrade.routine_sla
│       │   ├── Security ──> deadline cfg.upgrade.security_sla; escalation; tracked as SEV per CVE
│       │   └── Policy-driven (rule moving to DENY) ──> template fix lands BEFORE policy promotion
│       └── Lifecycle: ACTIVE → DEPRECATED (warnings) → YANKED (no new instantiation; upgrade PRs forced)
│
├── 6. INVARIANTS
│   ├── P0-01 Zero insecure defaults
│   │   ├── No default credentials, tokens, keys, permissive network rules, debug endpoints
│   │   └── Enforce: secret scan + policy suite at publish (§3) and instantiation (§4.4)
│   ├── P0-02 Templates never bypass provisioning
│   │   ├── Scaffolder holds no cluster or cloud credentials                          [L]
│   │   └── Enforce: only output to clusters is a Workload API request
│   ├── P0-03 Scaffolder is least-privilege and sandboxed
│   │   ├── Template edits are a sensitive area: restricted + manually reviewed      [C V-52]
│   │   ├── Who may run which template is controlled by permissions                 [C V-52]
│   │   ├── SCM credentials scoped to minimum repo create/write                     [C V-52]
│   │   └── Rendering isolated from host filesystem (symlink/path traversal)        [C V-52]
│   ├── P0-04 Compliance by construction
│   │   └── Enforce: rendered output must pass policy at the same SHA clusters enforce
│   ├── P0-05 Lineage is recorded and trustworthy
│   │   ├── Template version and answers stored with the service                    [C V-51]
│   │   ├── Lineage file protected by CODEOWNERS; manual edits break updates        [C V-51]
│   │   └── No credentials in lineage (e.g. template source URLs with tokens)       [C V-51]
│   ├── P0-06 Upgrades preserve customization
│   │   └── Enforce: three-way merge; conflicts surfaced, never auto-resolved        [C V-51]
│   └── P0-07 Template change control
│       └── Signed tags, platform + security review, publish pipeline (§3)          [C V-37]
│
├── 7. EDGE CASES
│   ├── E-01 Shell-metacharacter injection in parameters → strict allowlist; no shell hooks with
│   │        interpolated input
│   ├── E-02 Template-engine injection (expressions in answers) → answers treated as data;
│   │        strict undefined; no user-controlled template content
│   ├── E-03 Symlink / path traversal in template or archive → containment check; reject    [C V-52]
│   ├── E-04 Name already used by another tenant → reject before repo creation           [C V-11]
│   ├── E-05 Repo created, Workload API rejects → PENDING_ARCHIVE; owner decides; no orphan
│   ├── E-06 Repo created, initial CI wiring fails → idempotent retry from ledger
│   ├── E-07 Template version yanked after instantiation → forced upgrade PR; deadline
│   ├── E-08 Service heavily diverged → every upgrade conflicts → flag as off-path; owner
│   │        owns maintenance; reported in drift metrics
│   ├── E-09 Lineage file edited by hand → update fails or misapplies → CODEOWNERS block   [C V-51]
│   ├── E-10 Template source URL with embedded credentials stored in lineage → reject at publish [C V-51]
│   ├── E-11 Base image CVE → security-class upgrade across all lineage matches
│   ├── E-12 Policy rule promoted to DENY breaks generated services → template fixed and rolled
│   │        out first; policy promotion gated on upgrade adoption
│   ├── E-13 Runtime version reaches end of life → template DEPRECATED; migration PRs
│   ├── E-14 Two templates composed (service + stateful add-on) → each owns distinct paths;
│   │        overlap rejected at publish
│   ├── E-15 Bot commits unsigned → Argo CD refuses sync → bot key per project             [C V-37]
│   └── E-16 Scaffolder platform CVE → patch window; restrict execution until patched      [C V-52]
│
├── 8. FAILURE MODES
│   ├── F-01 Template ships insecure default [SECURITY] → publish gates → secret/policy scan alerts
│   ├── F-02 Scaffolder compromised [SECURITY] → sandbox, no cluster creds, scoped SCM creds
│   │        → audit of repo-creation events
│   ├── F-03 Upgrade wipes customization [DEGRADE] → three-way merge only → PR diff review
│   ├── F-04 Fleet stuck on vulnerable versions [SECURITY] → security SLA + escalation
│   │        → versions-behind metric
│   ├── F-05 Orphaned repos [COST | COMPLIANCE] → ledger + PENDING_ARCHIVE → orphan report
│   └── F-06 Template and policy drift apart [OUTAGE at promotion] → T-2 + E-12 gating
│        → nightly render-and-check of all ACTIVE templates
│
├── 9. PRODUCTION OPERATIONS
│   ├── SLIs (targets cfg.*)
│   │   ├── Scaffold success rate; time from request to FIRST_DEPLOY_READY
│   │   ├── Template conformance: ACTIVE templates passing nightly policy check = 100%
│   │   ├── Versions-behind distribution per template family
│   │   ├── Security-patch propagation time (publish → merged in all services)
│   │   ├── Upgrade PR merge latency; conflict rate per template
│   │   └── Off-path service count (E-08)
│   ├── Alerts → runbooks
│   │   ├── Nightly conformance failure → TICKET platform → fix template, re-publish
│   │   ├── Scaffold failures above cfg.alert.scaffold_failures → PAGE platform → RB: inspect
│   │   │   sandbox, SCM API, policy SHA mismatch
│   │   ├── Security upgrade past deadline → escalate to owner lead + security
│   │   └── Unexpected repo creation outside scaffolder identity → PAGE security
│   ├── Cadence
│   │   ├── Weekly: versions-behind review; stale upgrade PRs
│   │   ├── Monthly: base image refresh; template dependency updates
│   │   ├── Per policy change: render-and-check all ACTIVE templates before promotion
│   │   └── Quarterly: deprecation review; scaffolder access review
│   └── Ownership
│       ├── Platform: templates, scaffolder, upgrade bot, registry
│       ├── Security: publish review, base images, security-class upgrades
│       └── Service owners: merging upgrades, resolving conflicts
│
├── 10. ROADMAP (exit gates, no dates)
│   ├── PHASE 0 One golden path per D-LANG runtime; publish pipeline (§3); sandboxed scaffolder;
│   │   Workload API handoff
│   │   └── EXIT: generated service reaches canary in prod with zero policy exceptions
│   ├── PHASE 1 Registry with lifecycle states; lineage store; nightly conformance
│   │   └── EXIT: every service traceable to template version
│   ├── PHASE 2 Upgrade bot with three-way merge; security SLAs
│   │   └── EXIT: simulated base-image CVE patched fleet-wide within cfg.upgrade.security_sla
│   └── PHASE 3 Add-on templates (stateful, batch) with path ownership
│       └── EXIT: composed templates pass publish gates without overlap
│
└── 11. GO-LIVE GATES
    ├── D-SCM, D-CI, D-TPL, D-LANG, D-SEC answered
    ├── Scaffolder holds no cluster/cloud credentials (verified)
    ├── Every ACTIVE template passes policy suite at current policy SHA
    ├── Injection, traversal and unsigned-bot-commit tests pass
    └── Upgrade drill: conflicting and clean cases both handled without force-push
```

**New sources**

- **V-51 Three-way template updates (Copier as the reference engine):**
  - [Certain] Projects record the template version they came from in their answers file. An update re-renders the template with the same answers and applies the difference as a three-way merge. Running tasks and migrations requires explicitly trusting the template.
  - [Likely] One template's guidance warns that manually editing the answers file can break the update algorithm. It also says to prefer clean Git remote URLs so credentials don't end up stored in the answers file.
  - [Certain] Copier gates unsafe features, including tasks, migrations, Jinja extensions and external data reads outside the destination, and missing variables can be made to error rather than render blank.
  - [Likely] Conflicts on update surface as Git merge conflicts, or as `.rej` files that must be resolved manually.
- **V-52 Scaffolder security (Backstage as the reference portal):**
  - [Certain] Backstage's threat model treats scaffolder templates as a sensitive area. It recommends controlling access to templates and manually reviewing changes, after a vulnerability where write access to a template enabled remote code execution on the backend.
  - [Certain] The scaffolder integrates with Backstage's permission framework to control access to parameters, steps and actions based on the user running the template.
  - [Certain] A 2026 advisory describes an authorization bypass letting authenticated users get around configured action restrictions. The recommended mitigations are restricting template access and limiting source-control credentials to minimum scopes.
  - [Certain] Another advisory covers symlink-based path traversal in scaffolder actions. The mitigations are limiting who can create and execute templates and running Backstage with restricted filesystem access.
