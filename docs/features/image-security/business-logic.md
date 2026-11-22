Six open decisions sit inside this tree, and no amount of merging removes them: D-SLOT, D-VERIFY, D-CI, D-BASE, B-1 and B-3. They stay marked as blockers. Everything else from the last three versions (v2, the invariants and edge cases, and the deep-check corrections) is merged below into one tree with duplicates removed.

```
IMAGE SECURITY — SINGLE COMPLETE TREE (internal platform)
Legend   : [YOU] decision   [GS] gold-standard default   BLOCKER = open, blocks go-live
           [C V-xx] verified earlier in thread   [L] inference — verify in your clusters
           cfg.* configurable (floor/default/ceiling + named owner; no invented numbers)
           PRIO = priority class (§5), never raw CVSS   Rule IDs (IMG-, POD-) = policy-as-code tree
│
├── 0. DECISIONS
│   ├── Settled / defaulted
│   │   ├── Registry: Artifact Registry, private, per-tenant repos, immutable tags ON (default OFF) [GS, C V-66]
│   │   ├── Scanner of record: Artifact Analysis + build-time SBOM                       [GS, C V-62]
│   │   ├── Admission: Binary Authorization ENFORCE, required attestors                   [GS, C V-29]
│   │   ├── Continuous validation: monitoring only (Preview), never sole control         [C V-65]
│   │   ├── Node image: Container-Optimized OS (lower exposure, faster patches)          [GS, C V-67]
│   │   └── Untrusted / third-party code: GKE Sandbox node pools                         [GS, C V-67]
│   └── BLOCKERS
│       ├── D-SLOT   Cover initContainers / native sidecars / image volumes:
│       │            A) CEL digest-match  B) webhook verifier  C) forbid for tenants        [C V-70]
│       ├── D-VERIFY Who verifies provenance and signs the admission attestation          [C V-64]
│       ├── D-CI     CI system (Cloud Build enables built-in SLSA check; others need own verifier) [C V-64]
│       ├── D-BASE   Base image source (vendor-hardened catalog | own minimal builds)
│       ├── B-1      Residency: may vulnerability data be processed in the US (CV vuln check)? [C V-65]
│       └── B-3      Standard vs Autopilot (node patch ownership, observability)
│
├── 1. PRINCIPLES
│   ├── P-1 Trust = verified provenance (builder + exact source repo + signed commit), not scan result [C V-53, V-55]
│   ├── P-2 Every image slot in a Pod is verified — not only what Binary Authorization sees  [C V-70]
│   ├── P-3 Priority = exploitation evidence × exposure × applicability × severity          [C V-56, V-57]
│   ├── P-4 Vulnerability data is incomplete by design; unknown ≠ clean                    [C V-61]
│   ├── P-5 Every running digest has fresh scan + runtime inventory                        [C V-62]
│   ├── P-6 Node/runtime layer is part of image security                                   [C V-59, V-67]
│   ├── P-7 Security tooling is a target: pinned, credential-less, itself attested         [C V-53]
│   ├── P-8 Digests everywhere; tags are never a security boundary                         [C V-53, V-66]
│   └── P-9 No compliance claim rests only on a Preview feature                            [C V-65]
│
├── 2. THREAT MODEL
│   ├── T-01 Stolen publish credentials push malicious tag (Trivy, KICS, LiteLLM)        [C V-53..V-55]
│   ├── T-02 CI workflow abuse; force-pushed action tags; fork PRs with secrets          [C V-53]
│   ├── T-03 Lookalike source repo in provenance                                         [C V-55]
│   ├── T-04 Malicious package inside an otherwise clean image                          [C V-62]
│   ├── T-05 Exploitable runtime CVE (Java4Shell, nginx2root, PHP2Shell)                [C V-56..V-58]
│   ├── T-06 Exploitation before disclosure / before KEV                                [C V-58, V-68]
│   ├── T-07 Escape via runtime or kernel (runc, containerd, Fragnesia, DirtyFrag)     [C V-59, V-67]
│   ├── T-08 Embedded secrets in layers                                                 [C V-62]
│   ├── T-09 Stale scans; scanner blind to unmanaged binaries                          [C V-62]
│   ├── T-10 Vulnerability-feed gaps (NVD triage)                                       [C V-61]
│   ├── T-11 Suppression abuse: image-level VEX hides future versions                  [C V-69]
│   ├── T-12 Never-revoked exposed credentials                                          [C V-55]
│   ├── T-13 Unchecked Pod slots: initContainers, native sidecars, image volumes       [C V-70]
│   ├── T-14 Permissive BA config: ALWAYS_ALLOW default, dry-run, broad exemptions     [C V-70]
│   └── T-15 Cross-tenant use of cached private images on shared nodes                 [C V-71]
│
├── 3. PIPELINE
│   │
│   ├── 3.1 SOURCE & CI
│   │   ├── Actions/plugins pinned by commit SHA                                          [C V-53]
│   │   ├── Privileged PR triggers forbidden; forks never see secrets or push rights     [C V-53]
│   │   ├── Separation of duties: change ≠ approve ≠ release                             [C V-55]
│   │   ├── CI logs immutable and exported; deletion = alert                             [C V-55]
│   │   ├── Short-lived workload-identity credentials; no long-lived publish tokens       [L]
│   │   └── Scanners/verifiers digest-pinned, attested, no publish/cloud-admin creds     [C V-53]
│   │
│   ├── 3.2 BUILD
│   │   ├── Trigger-invoked builds only (manual builds get no provenance)               [C V-64]
│   │   ├── Base image from D-BASE, pinned by digest, base provenance verified         → IMG-02/03
│   │   ├── Runtimes via package manager or checksum-verified vendor artifact; unmanaged
│   │   │   binaries fail the build (scanner blind spot)                                [C V-62]
│   │   ├── Remote fetches checksum-pinned; no curl | sh; no unpinned ADD URL            [L]
│   │   ├── Build secrets mounted at build time only; layer-level secret scan           [L]
│   │   ├── Build caches separated by trust level                                       [L]
│   │   ├── Build-time SBOM (incl. vendored/static deps) attested, bound to digest
│   │   └── Provenance: builder, exact source repo, commit, build config                [C V-64]
│   │
│   ├── 3.3 REGISTRY
│   │   ├── Prod repos: immutable tags ON; setting change = alert                       [C V-66]
│   │   ├── Push only by CI identity; no human writers                                  [L]
│   │   ├── Per-tenant repos; tenants can't read others' repos                          [L]
│   │   ├── Scanning enabled on every prod repo (per-repo setting monitored)             [C V-66]
│   │   ├── On-push scan (digest-based; tag moves don't rescan)                         [C V-62]
│   │   ├── Malicious package → quarantine digest + PAGE security                       [C V-62]
│   │   ├── Remote/pull-through repos never a prod source; third-party mirrored by digest after
│   │   │   verification                                                               [C V-53]
│   │   ├── Cleanup never deletes running digests or forensic-retention digests          [C V-66]
│   │   └── Encrypted with your key; inside network perimeter                           [L, B-1]
│   │
│   ├── 3.4 VERIFY & ATTEST (D-VERIFY)
│   │   ├── Check: builder ∈ trusted builders                                          [C V-64]
│   │   ├── Check: source repo = exact allowlist entry (no wildcard above org path)     [C V-64]
│   │   ├── Check: commit on protected branch, signed                                   [C V-37]
│   │   ├── Check: scan complete; no open PRIO-0/1 without active exception
│   │   ├── Check: SBOM present; no unmanaged-runtime findings
│   │   ├── ALL PASS ──> sign per-environment attestation with KMS key usable only by verifier
│   │   └── ANY FAIL ──> no attestation; reason logged; owner notified
│   │
│   ├── 3.5 ADMISSION — EVERY POD SLOT
│   │   ├── containers + ephemeralContainers ──> Binary Authorization required attestors [C V-70]
│   │   ├── initContainers + native sidecars + image volumes ──> D-SLOT control         [C V-70]
│   │   ├── All slots: digest references only (CEL)                                  → IMG-02
│   │   ├── Missing/invalid attestation in any slot ──> [DENY] + audit log
│   │   ├── Breakglass: only break-glass group (CEL), logged, CV + inventory keep flagging [C V-29]
│   │   └── BA config guards: no ALWAYS_ALLOW, no prod dry-run, narrow reviewed exemptions,
│   │       global system-image trust documented                                        [C V-70]
│   │
│   ├── 3.6 RUNTIME
│   │   ├── Own rescan job: every running digest within cfg.scan.max_age
│   │   │   (closes the 30-day continuous-analysis window; independent of Preview CV)   [C V-62, V-65]
│   │   ├── Runtime inventory job: actual runtime versions per running digest           [L]
│   │   ├── CV platform policy (monitoring): attestation, freshness, SLSA if Cloud Build [C V-63, V-64]
│   │   ├── CV vulnerability check OFF until B-1 permits                                [C V-65]
│   │   └── Inventory: digest ↔ SBOM ↔ owner ↔ tenant ↔ exposure ↔ node pool CPU/kernel/OS
│   │
│   └── 3.7 NODES
│       ├── COS; release-channel auto-upgrade; bulletin feed subscribed                [C V-67]
│       ├── Node pools separated by trust tier and tenant; untrusted → Sandbox          [C V-67]
│       ├── Node service account reads only repos for workloads that pool serves        [C V-71, L]
│       ├── seccomp RuntimeDefault enforced; no NET_ADMIN/SYS_ADMIN for tenants        → POD-04/07 [C V-67]
│       └── Applicability metadata (CPU features, kernel, OS) re-evaluated on pool changes [C V-56]
│
├── 4. INVARIANTS (Statement │ Enforce │ Detect)
│   ├── A. TRUST ROOT & ADMISSION
│   │   ├── A01 Every image in every Pod slot is verifier-attested │ BA + D-SLOT │ inventory vs attestations
│   │   ├── A02 BA default rule never ALWAYS_ALLOW in prod │ platform-repo-managed BA policy │ daily diff
│   │   ├── A03 Prod BA mode is block-and-audit │ Cluster API baseline │ dry-run logs from prod = alert
│   │   ├── A04 Every prod cluster has BA enforcement │ Org Policy + posture scan │ posture scan
│   │   ├── A05 Exemptions narrow, platform-owned, security-reviewed │ guard rejects broad patterns │ diff alert
│   │   ├── A06 Global system-image trust is a documented decision │ review │ config diff
│   │   ├── A07 Breakglass only by break-glass group │ CEL │ audit event → PAGE
│   │   ├── A08 Attestations per environment │ separate attestors │ attestor usage audit
│   │   └── A09 Attestor keys non-exportable, verifier-only │ KMS IAM │ key-use audit → PAGE
│   ├── B. BUILD & PROVENANCE
│   │   ├── B01 Only trigger-invoked builds are deployable │ verifier │ provenance absence
│   │   ├── B02 Exact repo allowlist; protected branch; signed commit │ verifier │ verifier logs
│   │   ├── B03 Untrusted builds never get secrets/push │ CI policy │ CI config scan
│   │   ├── B04 Caches separated by trust level │ CI config │ CI config scan
│   │   ├── B05 No unmanaged runtimes │ build rule │ runtime inventory
│   │   ├── B06 Remote fetches checksum-pinned │ build lint │ build lint
│   │   ├── B07 No secrets in any layer │ build mounts + scan │ layer secret scan
│   │   ├── B08 SBOM attested per digest │ verifier │ attestation check
│   │   └── B09 CI actions SHA-pinned; separation of duties │ CI policy │ workflow change alert
│   ├── C. REGISTRY
│   │   ├── C01 Immutable tags on prod repos │ repo config │ setting-change alert
│   │   ├── C02 CI-only push │ IAM │ IAM drift alert
│   │   ├── C03 No remote repo as prod source │ admission allowlist │ inventory
│   │   ├── C04 Per-tenant repo isolation │ IAM │ IAM drift alert
│   │   ├── C05 Cleanup spares running + retained digests │ keep policies │ pull failures
│   │   ├── C06 CMEK + perimeter │ Org Policy │ posture scan
│   │   └── C07 Scanning on in every prod repo │ repo config │ config scan
│   ├── D. VULNERABILITY DATA
│   │   ├── D01 Scan + inventory fresher than cfg.scan.max_age │ jobs │ stale alert
│   │   ├── D02 No PRIO-0/1 admitted without mitigated exception │ verifier │ inventory
│   │   ├── D03 Malicious package = PRIO-0 │ registry quarantine │ scan occurrences
│   │   ├── D04 Unknown severity tracked with SLA │ triage queue │ PRIO-U age
│   │   ├── D05 VEX per digest only, security-approved │ CI rejects image-level VEX │ VEX audit
│   │   ├── D06 Exploitation trigger = vendor/CERT/threat intel OR KEV │ PRIO model │ intel feed
│   │   └── D07 Scan data location complies with B-1 │ CV vuln check gated │ config scan
│   ├── E. RUNTIME & NODES
│   │   ├── E01 COS unless approved exception │ Cluster API │ posture scan
│   │   ├── E02 Patched within bulletin SLA │ release channel │ version vs bulletin
│   │   ├── E03 Trust tiers never share pools │ scheduling policy │ inventory
│   │   ├── E04 Pods only run images their tenant may pull │ tenant repos + pools + node SA │ tenant mismatch → PAGE
│   │   ├── E05 Node SA least privilege │ IAM │ IAM drift alert
│   │   ├── E06 seccomp RuntimeDefault; no NET_ADMIN/SYS_ADMIN │ PSA + CEL │ PC audit
│   │   └── E07 Applicability metadata current │ pool-change hook │ inventory diff
│   ├── F. TOOLING & CREDENTIALS
│   │   ├── F01 Security tools hold no publish/cloud-admin creds │ IAM │ IAM scan
│   │   ├── F02 Exposed credentials revoked + verified within cfg.sla.cred_revoke │ runbook │ credential inventory
│   │   ├── F03 CI/verifier logs immutable │ log sinks + locks │ deletion alert
│   │   └── F04 Security tools are attested images (no exemption) │ A01 │ inventory
│   └── G. GOVERNANCE
│       ├── G01 Exceptions PRIO-1/2 only, mitigated, digest-bound, expiring, auto-revoked
│       ├── G02 No Preview-only control as sole evidence
│       ├── G03 Every invariant has owner, detection, alert, runbook
│       └── G04 Every invariant drilled within cfg.drill.interval
│
├── 5. PRIORITY MODEL
│   ├── Inputs: exploitation evidence (vendor, CERT, threat intel, KEV), exposure (internet,
│   │   pre-auth path, privileges, data class), applicability (config, CPU, kernel), vendor/CNA
│   │   severity with CVSS version recorded, fix availability
│   ├── PRIO-0 Malicious/backdoored/unverified provenance │ exploited pre-auth RCE on exposed
│   │          workload │ escape with exploit
│   ├── PRIO-1 Critical/high RCE or escape with public exploit or active attempts, fix available
│   ├── PRIO-2 High, no exploit evidence, fix available
│   ├── PRIO-3 Medium/low, or digest-level VEX "not affected"
│   ├── PRIO-U Unknown/unenriched → triage within cfg.triage.unknown_sla               [C V-61]
│   ├── KEV is a floor, never the trigger (PHP2Shell: ~9-day KEV lag)                 [C V-68]
│   └── SLAs: PRIO-0 contain cfg.sla.prio0_contain, fix cfg.sla.prio0_fix │ PRIO-1 cfg.sla.prio1 │
│       PRIO-2 cfg.sla.prio2 then block │ PRIO-U never silently passes
│
├── 6. EXCEPTIONS
│   ├── PRIO-1/2 only; never PRIO-0
│   ├── Named, tested mitigation with side effects (e.g. HTTP/2 off breaks gRPC)        [C V-57]
│   ├── Bound to digest; expiry ≤ cfg.exception.image_max_ttl; auto-revoke on fixed build
│   ├── VEX "not affected" reviewed like an exception                                   [C V-69]
│   └── CV and inventory keep reporting excepted digests                              [C V-63]
│
├── 7. EDGE CASES
│   ├── 7.1 POD SHAPE & ADMISSION
│   │   ├── EC-01 Unattested initContainer → D-SLOT denies (A01)                       [C V-70]
│   │   ├── EC-02 Unattested native sidecar → D-SLOT denies (A01)                      [L]
│   │   ├── EC-03 Image volume with unattested content → forbid or D-SLOT (A01)        [C V-70]
│   │   ├── EC-04 Ephemeral debug container → BA checks; debug image attested (A01)   [C V-70]
│   │   ├── EC-05 Mesh-injected sidecar → platform-attested or narrow digest exemption (A05)
│   │   ├── EC-06 Tag reference resolves differently per node → digest required (IMG-02)
│   │   ├── EC-07 Index digest vs per-platform digest → attest what Pods reference; test [L]
│   │   ├── EC-08 Digest copied to another repo path → only prod repos admitted         [L]
│   │   ├── EC-09 Nonprod attestation in prod → per-env attestors deny (A08)
│   │   ├── EC-10 Unauthorized breakglass → CEL deny + PAGE (A07)
│   │   ├── EC-11 Prod left in dry-run → guard + posture scan (A03)
│   │   ├── EC-12 Exemption widened "temporarily" → guard rejects (A05)
│   │   ├── EC-13 Global system-image trust disabled → system Pods fail; documented test (A06)
│   │   ├── EC-14 BA/attestation backend unavailable → behaviour measured in game day   [L]
│   │   └── EC-15 Cluster created without BA → Org Policy denies (A04)
│   ├── 7.2 ATTESTATION LIFECYCLE
│   │   ├── EC-16 Provenance later found bad → delete attestations; flag + replace running Pods [C V-63]
│   │   ├── EC-17 Attestor key compromised → re-key, re-attest verified digests, staged removal [L]
│   │   ├── EC-18 KMS outage → new digests blocked; attested digests still deploy
│   │   ├── EC-19 Verifier bug signs bad digest → verifier changes treated as C1 policy changes
│   │   └── EC-20 Attested digest later gets PRIO-0 → revoke attestation; replace running Pods
│   ├── 7.3 BUILD
│   │   ├── EC-21 Manual Cloud Build → no provenance → refused (B01)                  [C V-64]
│   │   ├── EC-22 Fork PR hits privileged workflow → forbidden (B03)                 [C V-53]
│   │   ├── EC-23 Shared cache poisoned → separated caches (B04)                       [L]
│   │   ├── EC-24 Base image compromised upstream → digest + base provenance (IMG-02)
│   │   ├── EC-25 Tarball JDK/nginx → scanner blind → build fails (B05)              [C V-62]
│   │   ├── EC-26 Static binaries / vendored deps → build-tool SBOM                    [L]
│   │   ├── EC-27 Distroless/scratch without package DB → unknown composition unless SBOM (D04)
│   │   ├── EC-28 Secret in intermediate layer → layer scan; build-time mounts (B07)
│   │   ├── EC-29 Non-reproducible digest from same commit → provenance binds; drift flagged [L]
│   │   └── EC-30 Scan digest A, deploy digest B via moved tag → digest-only (C01)
│   ├── 7.4 REGISTRY
│   │   ├── EC-31 Admin disables immutable tags → alert (C01)                        [C V-66]
│   │   ├── EC-32 Remote repo caches malicious upstream tag → never a prod source (C03) [C V-53]
│   │   ├── EC-33 Cleanup deletes running digest → keep policy for running (C05)      [C V-66]
│   │   ├── EC-34 Regional registry outage → multi-region mirror
│   │   ├── EC-35 Tenant SA granted writer → IAM drift alert (C02)
│   │   └── EC-36 Scanning disabled on one repo → config check (C07)                 [C V-66]
│   ├── 7.5 VULNERABILITY DATA
│   │   ├── EC-37 CVSS v3.1 vs v4 disagree → record version; PRIO decides             [C V-56]
│   │   ├── EC-38 Unenriched / "Not Scheduled" CVE → PRIO-U (D04)                     [C V-61]
│   │   ├── EC-39 Distro backport fixed, version looks vulnerable → distro feed wins   [L]
│   │   ├── EC-40 Fix only in breaking major → exception + migration plan
│   │   ├── EC-41 Vulnerable code unreachable → digest-level VEX, approved (D05)
│   │   ├── EC-42 Image-level VEX would hide future builds → rejected (D05)          [C V-69]
│   │   ├── EC-43 Exploited before disclosure / not in KEV → threat-intel trigger (D06) [C V-68]
│   │   ├── EC-44 Malicious package found after deploy → PRIO-0 replace + IOC hunt (D03)
│   │   ├── EC-45 Vendor disputes CVE → tracked; decision recorded with source
│   │   └── EC-46 Scan data residency conflict → CV vuln check off; own scanner path (D07) [C V-65]
│   ├── 7.6 RUNTIME & NODES
│   │   ├── EC-47 Long-running image never re-pulled → rescan + inventory jobs (D01)  [C V-62]
│   │   ├── EC-48 Tenant B uses tenant A's cached private image → pools + repos + node SA (E04) [C V-71]
│   │   ├── EC-49 Node SA reads all repos → scope it (E05)                            [L]
│   │   ├── EC-50 Autoscaler adds a CPU family affected by a CPU-specific CVE → re-evaluate (E07) [C V-56]
│   │   ├── EC-51 Ubuntu pool during kernel CVE → patches lag COS → migrate or isolate (E01) [C V-67]
│   │   ├── EC-52 Mixed node versions mid-upgrade → risky workloads pinned to patched pools
│   │   ├── EC-53 Untrusted code in shared pool → Sandbox only (E03)                  [C V-67]
│   │   └── EC-54 containerd/runc CVE → node track, not image track                   [C V-59, V-67]
│   ├── 7.7 TOOLING & CREDENTIALS
│   │   ├── EC-55 Scanner image compromised → attested, pinned, no creds (F01, F04)   [C V-53]
│   │   ├── EC-56 Credential exposed months ago → revocation verified (F02)           [C V-55]
│   │   ├── EC-57 CI logs deleted after malicious release → immutable export alert (F03) [C V-55]
│   │   └── EC-58 Partial rotation → atomic revoke-then-reissue                       [C V-53]
│   └── 7.8 PROCESS & SCALE
│       ├── EC-59 Several criticals at once → order by PRIO × exposure
│       ├── EC-60 Rebuild storm saturates CI → reserved capacity for security rebuilds
│       ├── EC-61 Owner team dissolved → platform owns remediation until reassigned
│       ├── EC-62 Critical CVE during freeze → security exception path through freeze
│       ├── EC-63 Exception outlives fix → auto-revoke (G01)
│       └── EC-64 Forensics need old images/logs → retention per B-1 (C05)
│
├── 8. RESPONSE PLAYBOOKS
│   ├── RB-I1 Compromised upstream tool/image
│   │   └── IOC digest sweep (registry, mirrors, caches) → assume reachable creds stolen →
│   │       atomic revoke-then-reissue → host compromised if socket/host access → pin last-good
│   │       digest → resume only on verified provenance                                [C V-53]
│   ├── RB-I2 New critical runtime CVE
│   │   └── SBOM + runtime inventory query → applicability (config/CPU/exposure) → PRIO →
│   │       mitigation exception → rebuild by digest → verify fix in image → canary rollout [C V-56, V-57]
│   ├── RB-I3 Node/runtime escape
│   │   └── Bulletin versions → upgrade pools (COS first available) → interim Sandbox/pause
│   │       untrusted → hunt on affected nodes                                          [C V-67]
│   ├── RB-I4 Stale scan/inventory → force rescan; PRIO-U until fresh
│   ├── RB-I5 Feed gap → manual triage from vendor/CNA advisory; record decision      [C V-61]
│   ├── RB-I6 Exposed credential → revoke → verify → search usage → close            [C V-55]
│   ├── RB-I7 Attestor key compromise → freeze new attestations → re-key → re-attest → staged
│   │   removal → hunt old-key-only digests                                             [L]
│   └── RB-I8 BA config drift (ALWAYS_ALLOW / dry-run / broad exemption) → revert via platform
│       repo → list everything admitted during drift → verify each against attestations [C V-70]
│
├── 9. PRODUCTION OPERATIONS
│   ├── SLIs (targets cfg.*)
│   │   ├── % running images (all slots) with verifier attestation — target 100%
│   │   ├── % running digests with fresh scan + inventory
│   │   ├── Open PRIO-0/1 on running digests vs SLA
│   │   ├── Node pools within bulletin SLA
│   │   ├── PRIO-U count and age
│   │   ├── Disclosure → inventory answer time; → mitigation; → patched rollout
│   │   ├── Exceptions expired-but-active (0); image-level VEX (0)
│   │   └── Tenant-image mismatch count (0)
│   ├── Alerts
│   │   ├── PAGE security: attestation/provenance failure, malicious package, unknown pusher,
│   │   │   CI log deletion, BA config drift, immutable-tags change, breakglass, attestor key misuse,
│   │   │   tenant-image mismatch
│   │   ├── PAGE owner + security: PRIO-0/1 on running digest
│   │   ├── PAGE platform: GKE escape bulletin affecting your versions
│   │   └── TICKET: stale scan/inventory, PRIO-U SLA, node SLA approaching
│   ├── Cadence
│   │   ├── Daily: advisories vs inventory; bulletin review; breakglass reviews
│   │   ├── Weekly: base image refresh + fleet rebuild; exception review
│   │   ├── Monthly: credential audit; IOC sweep drill; BA config review
│   │   └── Quarterly: RB-I1 and RB-I7 tabletops; auditor evidence dry run
│   └── Ownership
│       ├── Security: PRIO model, exceptions, VEX approval, attestor keys, BA policy content
│       ├── Platform: pipeline, verifier, registry, nodes, inventory jobs, alerts
│       └── Service owners: rebuilds and remediation within SLA
│
├── 10. VERIFICATION (game days, per cfg.drill.interval)
│   ├── Unattested image as initContainer, native sidecar, image volume → denied
│   ├── BA default set to ALWAYS_ALLOW in nonprod → drift alert fires
│   ├── Manual build and lookalike-repo provenance → verifier refuses
│   ├── Tenant B referencing tenant A's private image on shared node → denied / unschedulable
│   ├── Tarball JDK in image → build fails; inventory detects legacy image
│   ├── Attestor key rotation in nonprod → no mass deny; old-key-only digests found
│   ├── Immutable tags disabled in nonprod → alert fires
│   ├── BA backend unavailable (simulated) → measured behaviour recorded
│   └── Every run recorded against invariant IDs as audit evidence
│
├── 11. ROADMAP (exit gates, no dates)
│   ├── PHASE 0 Trust root: D-CI, D-VERIFY, D-SLOT decided; verifier attestor enforced on all slots;
│   │   BA config guards; COS; immutable tags; per-tenant repos and pools
│   │   └── EXIT: every §10 admission test denies correctly
│   ├── PHASE 1 Truthful data: build-time SBOM, runtime inventory, own rescan job, PRIO model
│   │   └── EXIT: zero stale running digests; tarball-runtime drill caught
│   ├── PHASE 2 Response: RB-I1..I8, digest-level VEX workflow, exception records
│   │   └── EXIT: disclosure-to-inventory answer within cfg.response.inventory_time in a drill
│   └── PHASE 3 Automation: advisory-driven rebuilds, auto-proposed mitigations
│       └── EXIT: simulated critical CVE patched fleet-wide within PRIO-1 SLA
│
├── 12. GO-LIVE GATES
│   ├── D-SLOT, D-VERIFY, D-CI, D-BASE, B-1, B-3 answered
│   ├── A01 proven by game day (all Pod slots)
│   ├── BA guards active: no ALWAYS_ALLOW, no prod dry-run, narrow exemptions
│   ├── Immutable tags + scanning on every prod repo
│   ├── Zero running digests without fresh scan + inventory
│   ├── Node pools on COS at current bulletin versions; tenant pools separated
│   └── RB-I1, RB-I6, RB-I7 tabletops completed
│
└── 13. ACT NOW (as of 2026-11-22)
    ├── Java4Shell (CVE-2026-114938): JDK 17.0.6+/21/25/27 in running images → vendor-fixed builds;
    │   check node pools for x86-64 AVX-512 VBMI; not in KEV as of Nov 20 but attempts observed [C V-56, V-68]
    ├── nginx2root (CVE-2026-123007/123633): nginx 1.9.5–1.31.8 with HTTP/2 in images, ingress,
    │   gateways → 1.30.7 / 1.31.9; HTTP/2-off only where no gRPC depends on it        [C V-57]
    └── PHP images: confirm PHP2Shell-fixed versions (KEV since Oct 26)                  [C V-58, V-68]
```
