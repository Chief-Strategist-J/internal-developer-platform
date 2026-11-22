The template's own central invariant, forwarding the user's JWT to every downstream service, is a security problem. A token valid for many audiences can be replayed against any of them.

The real escalation paths in portals in 2026 aren't hidden buttons edited in DevTools. They are:

- **Ownership taken from the catalog.** Whoever can edit a catalog file can claim ownership, and ownership drives authorization.
- **Shared plugin credentials.** Every portal user inherits the proxy's static token, so the proxy acts on their behalf with its own privileges (a confused deputy).
- **Documentation builds.** TechDocs had code-execution, SSRF and file-read advisories this year.

I disagree with mirroring the template because it treats the portal as a UI problem when it's really a privileged aggregator. Here's what I'd do instead:

- The portal holds **no control-plane power**. Every mutation goes through your Workload, Cluster and Cloud APIs.
- Tokens are **exchanged per audience**, not forwarded.
- **Catalog integrity** is a security control.
- **Doc builds run sandboxed**, outside the portal.

The risk in the template's approach is a single compromised plugin or catalog file reaching every tenant.

The template's numbers (250 ms, 1500 ms, 30 s, 5,000 users, 80%) are replaced with `cfg.*` values, as before.

```
DEVELOPER PORTAL — SINGLE COMPLETE TREE (internal platform, government)
Legend   : [YOU] decision   [GS] gold-standard default   BLOCKER   [C V-xx] verified (V-72..V-79 below;
           earlier V-ids from this thread)   [L] inference — verify   cfg.* configurable + owner
           "If Backstage" = applies only if D-PORTAL selects Backstage
│
├── 0. DECISIONS
│   ├── Settled (from earlier trees)
│   │   ├── Mutations only via Cluster / Cloud Resource / Workload APIs; Argo CD single writer [YOU]
│   │   ├── Role-based approvals; JIT human access; 24/7 paging; all logs retained          [YOU]
│   │   └── Portal itself is an image under the image-security tree (attested, scanned)
│   └── BLOCKERS
│       ├── D-PORTAL Engine: Backstage OSS | commercial Backstage | own portal
│       ├── D-TPL    Scaffolder engine (from templates tree)
│       ├── D-SCM    Git host (catalog sources, doc sources)
│       ├── D-IDP-ROLE Where roles live: IdP groups only, or IdP + catalog Groups (ownership)
│       └── B-1      Residency/retention for audit logs and portal data
│
├── 1. PRINCIPLES
│   ├── PP-1 Portal = presentation + orchestration; never a control plane
│   ├── PP-2 Authorization decided by the owning API (PDP), never by UI visibility
│   ├── PP-3 Tokens are audience-restricted; exchanged per downstream, never broadcast   [C V-77, L]
│   ├── PP-4 No shared static credential grants a user more than their own rights (no confused deputy) [C V-73]
│   ├── PP-5 Catalog data that feeds authorization is a security-controlled input       [C V-75]
│   ├── PP-6 Untrusted content (docs, catalog files, templates) is processed in sandboxes [C V-78]
│   ├── PP-7 Deny by default: no allow-all permission policy, no unauthenticated endpoints [C V-72, V-74]
│   └── PP-8 Portal outage never blocks deploys, rollbacks or incident response
│
├── 2. ARCHITECTURE
│   ├── Frontend (SPA): same-origin plugins; strict CSP; no third-party scripts           [L]
│   ├── Auth service: separate service + DB for high-security deployments               [C V-75]
│   ├── Portal backend plugins: read aggregation + action dispatch only
│   ├── Action broker: converts portal actions into Workload API calls (idempotency key, audit)
│   ├── Catalog: org data (User/Group) only from IdP provider; components from registered repos
│   ├── Doc pipeline: built in CI sandbox; published to storage; served with authz      [C V-78]
│   ├── Event stream (SSE/WebSocket): authenticated, origin-checked, per-message authz   [C V-79]
│   └── Audit sink: every action + auth event → locked audit bucket (retention per B-1)  [C V-32]
│
├── 3. TIERS
│   │
│   ├── 3.1 AUTHENTICATION
│   │   ├── OIDC authorization code + PKCE; implicit grant never used                    [C V-77]
│   │   ├── Guest provider removed (create-app ships with it)                           [C V-74]
│   │   ├── Exactly one sign-in resolver per provider; domain allowlist on email resolvers [C V-74]
│   │   ├── Sign-in only for users present in catalog (never "allow without user")      [C V-74]
│   │   ├── Session: short-lived, HttpOnly, Secure, SameSite cookies; idle + absolute timeouts [L]
│   │   ├── Step-up (WebAuthn) for SENSITIVE actions; freshness ≤ cfg.stepup.max_age     [L]
│   │   └── CLI: device/PKCE flow; tokens in OS keychain; short-lived                    [L]
│   │
│   ├── 3.2 AUTHORIZATION
│   │   ├── Portal permission policy deny-by-default; allow-all module removed           [C V-72]
│   │   ├── Every action re-authorized by the owning API with user identity (exchanged token)
│   │   ├── Ownership claims resolved consistently across all plugins                   [C V-75]
│   │   ├── Read authorization filtered per entity (catalog, docs, k8s, logs, cost)      [C V-78]
│   │   └── Admin/superuser list minimal, IdP-group based, reviewed                      [L]
│   │
│   ├── 3.3 CATALOG INTEGRITY
│   │   ├── Kinds allowlisted; default ingestion is Component, API, Location only        [C V-75]
│   │   ├── User/Group entities only from the IdP org provider, never from tenant repos
│   │   ├── spec.owner changes require approval by both old and new owner (CI check on catalog file) [L]
│   │   ├── Locations registered only via platform (Workload API), not ad hoc by users
│   │   ├── Backend reading allowlist limited to trusted hosts; placeholders can't read beyond
│   │   │   what the registering user may read                                       [C V-75]
│   │   └── Orphaned / conflicting entities quarantined, never auto-trusted
│   │
│   ├── 3.4 PLUGINS & PROXY
│   │   ├── Plugin allowlist; each plugin package attested + pinned; npm supply-chain scan [C V-53, L]
│   │   ├── Proxy endpoints: credentials "require"; never "dangerously-allow-unauthenticated" [C V-73]
│   │   ├── Default auth policy never disabled (disabling makes every proxy endpoint open) [C V-73]
│   │   ├── Static target tokens in proxies forbidden for data scoped per tenant → use per-user
│   │   │   token exchange or a backend that enforces entity permissions               [C V-73, L]
│   │   ├── Third-party backend plugins checked for their own unauthenticated defaults    [C V-73]
│   │   └── Plugin failure isolated: timeout cfg.plugin.timeout, local fallback, no cascade
│   │
│   ├── 3.5 SELF-SERVICE ACTIONS
│   │   ├── Action catalog: each action → one Workload/Cluster/Cloud API operation
│   │   ├── Classification: READ | STANDARD | SENSITIVE | DESTRUCTIVE
│   │   │   ├── SENSITIVE/DESTRUCTIVE → step-up + role approval per your approval model [YOU]
│   │   │   └── DESTRUCTIVE in prod → typed confirmation of resource name + reason      [L]
│   │   ├── Envelope: idempotency key, actor, exchanged token, reason, ticket/incident link
│   │   ├── GitOps-aware: actions that change desired state create a signed commit/PR, not
│   │   │   a live patch (Argo CD would revert live patches)                          [C V-38, L]
│   │   ├── Restart / scale through Rollout-aware API, not raw pod deletion             [L]
│   │   └── Result: 202 + operation ID; status from the owning API, never from the portal
│   │
│   ├── 3.6 DAY-2 VIEWS (k8s, logs, metrics, traces, cost)
│   │   ├── Read via per-tenant scoped identities or permission-filtered backend        [L]
│   │   ├── Logs: PII/secret redaction before display; exports audited                [L, B-1]
│   │   ├── Exec/debug from portal = JIT access flow (no standing credentials in portal) [YOU]
│   │   └── Cost data visible to owners and finance roles only
│   │
│   ├── 3.7 DOCUMENTATION (TechDocs or equivalent)
│   │   ├── Built in CI sandbox (container, no credentials, egress allowlist), never in portal backend [C V-78]
│   │   ├── Config allowlist (no hooks / plugins from untrusted repos)                    [C V-78]
│   │   ├── Symlinks rejected; output scanned for secrets
│   │   ├── Published to storage; served with per-entity authorization                    [C V-78]
│   │   └── Generated HTML sanitized; served from isolated origin or strict CSP          [L]
│   │
│   └── 3.8 REAL-TIME EVENTS
│       ├── Origin allowlist checked on every handshake                                [C V-79]
│       ├── Token-based auth (not cookies alone); re-validated during long connections [C V-79]
│       ├── Per-message authorization: only events for entities the user may read
│       ├── Limits: per-user connections, message size, rate                           [C V-79]
│       └── Overload: fan-out via pub/sub; degrade update frequency to cfg.stream.min_interval
│
├── 4. INVARIANTS (Statement │ Enforce │ Detect)
│   ├── A. IDENTITY
│   │   ├── PI-A01 No guest or anonymous sign-in in any environment reachable by staff │ config guard │ config scan [C V-74]
│   │   ├── PI-A02 One sign-in resolver per provider; domain allowlist │ config guard │ config diff [C V-74]
│   │   ├── PI-A03 Users must exist in catalog (from IdP) to sign in │ resolver config │ sign-in audit [C V-74]
│   │   ├── PI-A04 Implicit grant never used; PKCE always │ IdP client config │ IdP audit [C V-77]
│   │   ├── PI-A05 Step-up required and fresh for SENSITIVE/DESTRUCTIVE │ action broker │ action audit
│   │   └── PI-A06 Tokens audience-restricted per downstream │ token exchange │ audience mismatch alerts [C V-77]
│   ├── B. AUTHORIZATION
│   │   ├── PI-B01 No allow-all policy in any non-local environment │ deployment check │ config scan [C V-72]
│   │   ├── PI-B02 Owning API re-authorizes every action │ API PDP │ portal-vs-API deny diff
│   │   ├── PI-B03 Portal service identity can't perform any mutation without a user token │ IAM │ IAM scan
│   │   ├── PI-B04 Ownership resolution identical across plugins │ shared library │ conformance test [C V-75]
│   │   └── PI-B05 Every read surface filters by entity permission │ backend │ cross-tenant test
│   ├── C. CATALOG
│   │   ├── PI-C01 User/Group only from IdP provider │ catalog rules │ entity-source audit [C V-75]
│   │   ├── PI-C02 Ownership change needs old + new owner approval │ CI on catalog files │ owner-change alert
│   │   ├── PI-C03 Reading allowlist limited to trusted hosts │ config │ config diff [C V-75]
│   │   └── PI-C04 Locations only registered by platform │ permission policy │ location audit
│   ├── D. PLUGINS & CONTENT
│   │   ├── PI-D01 No unauthenticated proxy/backend endpoints │ config guard + tests │ endpoint scan [C V-73]
│   │   ├── PI-D02 Default auth policy never disabled │ config guard │ config scan [C V-73]
│   │   ├── PI-D03 No static shared token exposes tenant-scoped data │ review │ proxy inventory [C V-73]
│   │   ├── PI-D04 Plugins allowlisted, pinned, attested │ build │ SBOM diff
│   │   ├── PI-D05 Docs never built in portal runtime; sandbox without credentials │ pipeline │ build audit [C V-78]
│   │   └── PI-D06 Scaffolder/templates per templates tree (sandbox, no cluster creds) [C V-52]
│   ├── E. ACTIONS
│   │   ├── PI-E01 Every action maps to exactly one owning-API operation │ action catalog │ unmapped = blocked
│   │   ├── PI-E02 Desired-state changes go through Git, not live patches │ action broker │ Argo CD drift alerts
│   │   ├── PI-E03 Every action idempotent and audited with actor, reason, token audience │ broker │ audit completeness
│   │   └── PI-E04 UI state never authoritative for results │ status from API │ UI/API mismatch tests
│   ├── F. STREAMING
│   │   ├── PI-F01 Origin allowlist on every handshake │ server │ origin-reject metrics [C V-79]
│   │   └── PI-F02 Per-message authz + periodic re-validation │ server │ revoked-session-still-connected test [C V-79]
│   └── G. RESILIENCE & AUDIT
│       ├── PI-G01 Portal outage doesn't block deploy/rollback/incident paths │ independent CLI/API paths │ game day
│       ├── PI-G02 Plugin failure isolated │ timeouts + fallbacks │ error-boundary metrics
│       ├── PI-G03 All auth + action events in locked audit storage │ log sinks │ routing tests [C V-32]
│       └── PI-G04 Portal patched within security SLA (advisories for portal framework tracked) [C V-78]
│
├── 5. EDGE CASES
│   ├── 5.1 IDENTITY
│   │   ├── EC-P01 Guest provider left from scaffold → PI-A01 guard                     [C V-74]
│   │   ├── EC-P02 Second resolver added "for convenience" → account hijack path → PI-A02 [C V-74]
│   │   ├── EC-P03 Email local-part collision across domains → allowlist + exact-match resolver [C V-74]
│   │   ├── EC-P04 User removed from IdP but session alive → short sessions + revocation on IdP event
│   │   ├── EC-P05 Step-up done hours ago reused for destructive action → freshness check
│   │   ├── EC-P06 CLI token stolen from laptop → short TTL; device-bound where supported [L]
│   │   └── EC-P07 Portal token replayed against another API → audience restriction (PI-A06) [C V-77]
│   ├── 5.2 AUTHORIZATION & CATALOG
│   │   ├── EC-P08 Tenant edits spec.owner to claim another team's service → PI-C02
│   │   ├── EC-P09 Tenant repo defines a Group/User entity to gain membership → PI-C01  [C V-75]
│   │   ├── EC-P10 Catalog placeholder reads a private file the user can't access → PI-C03 [C V-75]
│   │   ├── EC-P11 Plugin uses different ownership logic → privilege mismatch → PI-B04   [C V-75]
│   │   ├── EC-P12 Hidden UI button invoked via API → owning API denies (PI-B02)
│   │   ├── EC-P13 Orphaned entity still grants ownership → quarantine; ownership revoked
│   │   └── EC-P14 Team reorg leaves services unowned → platform owns until reassigned
│   ├── 5.3 PLUGINS & PROXY
│   │   ├── EC-P15 Proxy with static vendor token exposes all tenants' data → PI-D03    [C V-73]
│   │   ├── EC-P16 Third-party plugin allows unauthenticated requests by default → PI-D01 [C V-73]
│   │   ├── EC-P17 Default auth policy disabled to "fix" a plugin → every proxy endpoint opens → PI-D02 [C V-73]
│   │   ├── EC-P18 Malicious npm update to a plugin → pinned + attested build (PI-D04)  [C V-53]
│   │   ├── EC-P19 Frontend plugin XSS → whole session exposed (same origin) → CSP, review [L]
│   │   └── EC-P20 Slow plugin cascades into page failure → timeout + fallback (PI-G02)
│   ├── 5.4 DOCUMENTATION
│   │   ├── EC-P21 Doc config hooks execute code on build host → sandboxed CI build (PI-D05) [C V-78]
│   │   ├── EC-P22 Symlink in docs embeds host files in HTML → reject symlinks         [C V-78]
│   │   ├── EC-P23 Doc build fetches internal URLs (SSRF) → egress allowlist          [C V-78]
│   │   ├── EC-P24 Crafted URL reads another entity's docs → patched version + authz tests [C V-78]
│   │   └── EC-P25 Building in CI with the same vulnerable package doesn't mitigate → patch + sandbox [C V-78]
│   ├── 5.5 ACTIONS
│   │   ├── EC-P26 Double-click submits action twice → idempotency key
│   │   ├── EC-P27 Live patch (restart annotation, scale) reverted by Argo CD self-heal → GitOps-aware
│   │   │         actions / Rollout-aware restart                                     [C V-38, L]
│   │   ├── EC-P28 Destructive action on wrong environment → typed confirmation + env banner [L]
│   │   ├── EC-P29 Action approved, then spec changed → approval bound to spec hash
│   │   ├── EC-P30 Action succeeds in API, portal shows failure (stream dropped) → status from API
│   │   └── EC-P31 Action during sync window freeze → API returns blocked; portal shows reason [C V-40]
│   ├── 5.6 STREAMING
│   │   ├── EC-P32 Malicious site opens socket with victim's cookies → origin allowlist (PI-F01) [C V-79]
│   │   ├── EC-P33 Origin checked by substring → bypass → exact match only             [C V-79]
│   │   ├── EC-P34 User loses access mid-stream → periodic re-validation (PI-F02)      [C V-79]
│   │   ├── EC-P35 Incident surge (thousands of viewers) → pub/sub fan-out + degrade frequency
│   │   └── EC-P36 Event leaks another team's deploy details → per-message authz
│   ├── 5.7 DATA & COMPLIANCE
│   │   ├── EC-P37 Logs shown in portal contain secrets/PII → redaction; restricted roles [L, B-1]
│   │   ├── EC-P38 Export of logs/cost data → audited; role-gated
│   │   └── EC-P39 Audit sink unavailable → actions blocked (fail closed) for SENSITIVE+ [L]
│   └── 5.8 RESILIENCE
│       ├── EC-P40 Portal down during incident → CLI/API break-glass paths (PI-G01)
│       ├── EC-P41 IdP down → portal unusable; independent break-glass identity for platform on-call [L]
│       └── EC-P42 Portal framework advisory (e.g. scaffolder/TechDocs) → patch SLA (PI-G04) [C V-52, V-78]
│
├── 6. PRODUCTION OPERATIONS
│   ├── SLIs (targets cfg.*)
│   │   ├── Page/view latency p95 per plugin; plugin error ratio
│   │   ├── Action success rate; time to status from owning API
│   │   ├── Authz denials at owning API for portal-allowed actions (should be ~0; non-zero = drift)
│   │   ├── Unauthenticated endpoint count (0); allow-all policy present (never)
│   │   ├── Catalog ownership changes without dual approval (0)
│   │   ├── Stream origin rejects; revoked sessions still connected (0)
│   │   └── Portal framework advisories open vs SLA
│   ├── Alerts
│   │   ├── PAGE security: auth config drift (guest, resolvers, allow-all, auth policy disabled),
│   │   │   unauthenticated endpoint found, User/Group from non-IdP source, ownership change
│   │   │   without approval, audit sink failure
│   │   ├── PAGE platform: action broker errors, auth service down
│   │   └── TICKET: plugin degradation, advisory approaching SLA
│   ├── Cadence: weekly config + endpoint scan; monthly permission review; per release: authz
│   │   conformance suite; quarterly pen test of portal + plugins
│   └── Ownership: platform (portal, broker, plugins), security (auth, policies, reviews),
│       teams (catalog accuracy for their entities)
│
├── 7. VERIFICATION (game days, per cfg.drill.interval)
│   ├── Call a hidden action via API without permission → owning API denies
│   ├── Tenant PR changes spec.owner / adds Group entity → blocked
│   ├── Cross-site page opens socket with cookies → rejected
│   ├── Revoke user in IdP mid-session → stream and API access end within cfg.revoke.max_lag
│   ├── Malicious doc config / symlink → sandbox build fails safely, no host access
│   ├── Proxy endpoint probed without credentials → 401
│   └── Portal taken down → deploy, rollback and incident paths still work
│
├── 8. ROADMAP (exit gates, no dates)
│   ├── PHASE 0 Identity + deny-by-default: OIDC/PKCE, single resolver, no guest, permission
│   │   policy, catalog rules, audit sink
│   │   └── EXIT: §7 identity and catalog tests pass
│   ├── PHASE 1 Read views with per-entity filtering; plugin allowlist; sandboxed docs
│   │   └── EXIT: cross-tenant read tests pass; doc sandbox tests pass
│   ├── PHASE 2 Action broker: Workload API actions, step-up, GitOps-aware changes
│   │   └── EXIT: every action audited, idempotent, re-authorized downstream
│   └── PHASE 3 Real-time streams, scorecards, cost views
│       └── EXIT: stream security tests pass; incident surge drill stable
│
└── 9. GO-LIVE GATES
    ├── D-PORTAL, D-TPL, D-SCM, D-IDP-ROLE, B-1 answered
    ├── PI-A01..A06, PI-B01..B05, PI-C01..C04, PI-D01..D03 proven by tests
    ├── Portal framework at a version with all current advisories fixed
    └── Break-glass paths tested with portal and IdP unavailable
```

**New sources (V-72 to V-79)**

- **V-72 Allow-all policy:** [Certain] Community RBAC setup instructions explicitly tell you to remove the allow-all permission policy module from the backend when installing a real policy. That confirms the allow-all module is the starting point you must replace.
- **V-73 Proxy credentials:**
  - [Certain] Since Backstage 1.28, proxy endpoints require Backstage user or service credentials by default, and those credentials aren't forwarded to the target.
  - [Certain] If the default auth policy is disabled, every proxy endpoint behaves as unauthenticated.
  - [Certain] Proxy examples inject a static vendor token as a header, which every authenticated caller then uses: the confused-deputy pattern.
  - [Certain] At least one third-party plugin allows unauthenticated requests by default unless configured otherwise.
- **V-74 Sign-in:**
  - [Certain] New projects come configured with a guest provider.
  - [Certain] Backstage advises a single sign-in resolver per provider, because more increase account-hijacking risk, and recommends `allowedDomains` with the email local-part resolver.
  - [Certain] Red Hat's documentation warns not to use the allow-without-catalog-user option in production.
- **V-75 Catalog and threat model:**
  - [Certain] By default the catalog only ingests Component, API and Location kinds, and adding a `catalog.rules` key replaces those defaults.
  - [Certain] The threat model notes that catalog placeholders can read sources the user can't access directly, and it scopes trusted reads via `backend.reading.allow`.
  - [Certain] It recommends running the auth backend as a separate service with its own database for high-security deployments, and stresses consistent ownership resolution.
- **V-76:** reserved (unused).
- **V-77 OAuth BCP:** [Certain] RFC 9700 deprecates the implicit grant over token leakage and replay. It covers audience-restricted and sender-constrained tokens, and it explicitly names eGovernment as a context needing stricter protection. [Likely] Per-audience token exchange (RFC 8693) is my recommended way to apply audience restriction; I didn't fetch that RFC here.
- **V-78 TechDocs 2026 advisories:** [Certain] In 2026, TechDocs had all of the following:
  - code execution through MkDocs hooks, where building in CI with the same package doesn't mitigate it;
  - arbitrary file read through symlinks;
  - an October code-execution flaw through doc configuration, worst when builds run with backend credentials;
  - SSRF from doc builds;
  - a cross-entity authorization bypass for static content.
- **V-79 WebSocket:** [Certain] OWASP notes that browsers send cookies on WebSocket handshakes, which enables cross-site hijacking. It recommends exact origin allowlists and token-based authentication. [Certain] MITRE CWE-1385 records a bypass where origin validation used only a substring match.

These five decisions block go-live: **D-PORTAL** (Backstage or not), **D-TPL** (scaffolder engine), **D-SCM** (Git host), **D-IDP-ROLE** (where roles live) and **B-1** (residency and retention). No codebase changes have been made.