# Service Templates — Business Logic Decision Trees

## 1. Architectural Overview & Template Instantiation Decision Tree

```
SERVICE TEMPLATES ENGINE (SCAFFOLDING & VERSIONED SKELETON DECISION TREE)
│
├── 1. TEMPLATE REGISTRATION & SECURITY AUDIT TIER [P0]
│   └── Real-World: Platform engineer publishes new standard 'Node.js Fastify Enterprise Template v2.0'
│       └── Trigger: Inbound Template Registration Request / Git Tag Event
│           ├── Action: Ingest Template Skeleton and `template.yaml` Manifest
│           ├── Check: Does Template satisfy Golden Path Security Baseline?
│           │   ├── Rule 1: Non-root Dockerfile with distroless base image?
│           │   ├── Rule 2: Pre-wired OpenTelemetry tracing & Prometheus metrics?
│           │   ├── Rule 3: Automated CI pipeline with SonarQube & Trivy security gates?
│           │   └── ANY RULE FAILED ──> [REJECT: Template does not meet Platform Golden Path Standard]
│           │   └── ALL RULES PASSED ──> Template Registered: APPROVED_GOLDEN_PATH
│           │
├── 2. PARAMETER RESOLUTION & VARIABLE INTERPOLATION TIER [P0]
│   └── Real-World: Developer instantiates template with `{ name: "fraud-detector", port: 8080 }`
│       └── Evaluation: Parameter Replacement Engine (Cookiecutter / Jinja2 / Nunjucks)
│           ├── Action: Sanitize Input Variables against Regex Whitelist (Prevent Path Traversal / Injection)
│           ├── Action: Interpolate Variables into Directory Names, File Names, and Code Files
│           ├── Check: Did Template Engine produce valid, syntax-clean project files?
│           │   ├── NO ──> [ABORT: Template Generation Error — Syntax/Parsing Bug]
│           │   └── YES ──> Action: Execute Headless Pre-Commit Linter & Build Test
│           │       ├── Check: Does Scaffolded Code compile cleanly in isolated container?
│           │       │   ├── NO ──> [HALT: Template produces broken code — Fail-safe abort]
│           │       │   └── YES ──> Skeleton Validated
│           │
├── 3. SKELETON MATERIALIZATION & SECURE VAULT WIRING TIER [P0]
│   └── Real-World: Creating target Git repo and committing the templated microservice files
│       └── State: MATERIALIZATION_IN_PROGRESS
│           ├── Step 1: Create Destination Git Repository with Standard Branch Protections
│           ├── Step 2: Commit Scaffolded Code as Initial Commit (`feat: initial service scaffold`)
│           ├── Step 3: Wire Default Webhooks, Slack Channel, and Datadog Dashboards
│           └── State: SERVICE_SCAFFOLDED
│
└── 4. DRIFT TRACKING & TEMPLATE UPGRADE ENGINE TIER [P1]
    └── Real-World: Security updates base template to patch OpenSSL vulnerability; 80 services must upgrade
        └── Trigger: New Template Patch Released (v2.0 -> v2.1)
            ├── Action: Identify all microservices instantiated from Template v2.0
            ├── Action: Generate Automated Upgrade Pull Request (Git 3-Way Merge between template revisions)
            ├── Action: Run PR CI/CD Integration Test
            └── State: UPGRADE_PR_DISPATCHED (Developers merge PR with 1 click)
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Insecure Defaults in Platform Templates
- Real-Life Incident: A developer used a community template that included a default hardcoded JWT secret; when deployed to production, attackers used the known secret to forge admin tokens.
- Rationale: Templates embody organizational security posture. Templates must never contain default credentials, plaintext secrets, or permissive default network configurations.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: CLEAN SKELETON (P0-01)
Trigger: Template Publishing
├── Action: Execute Deep Secret Scanner (TruffleHog / Gitleaks) on Template Repo
├── Check: Any hardcoded secret, token, or insecure default detected?
│   ├── YES ──> [HARD REJECT: Publishing Blocked]
│   └── NO ──> Permit Publishing
```

### Invariant P1-02: Deterministic 3-Way Merge Upgradeability
- Real-Life Incident: A platform team pushed template updates by blindly overwriting repos, wiping out developers' custom business logic and causing outrage.
- Rationale: Template upgrades must preserve downstream customizations using clean 3-way Git merge strategies against a dedicated template branch.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: SAFE TEMPLATE MERGE (P1-02)
Trigger: Automated Upgrade PR Generation
├── Action: Perform 3-way merge: (Base Template v1 vs Custom Service Repo vs Base Template v2)
├── Check: Merge Conflict Detected?
│   ├── YES ──> Flag PR as REQUIRING_MANUAL_CONFLICT_RESOLUTION (Do not force push)
│   └── NO ──> Clean automated PR created
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Parameter Injection in Scaffolded Script
- Real-Life Incident: A user supplied `name: "app; rm -rf /"` in the template form; an unescaped shell command in the scaffolding hook executed in the runner container.
- Decision Tree:
```
EDGE CASE 01: SCAFFOLDING INJECTION PREVENTION
Trigger: Template Parameter Ingestion
├── Action: Strict Alpha-Numeric Whitelist Validation on all parameters
├── Check: Any special character (`;`, `&`, `|`, `$`, `` ` ``) present in string parameters?
│   ├── YES ──> [REJECT: HTTP 422 — Illegal characters in parameter]
│   └── NO ──> Safe to interpolate
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
SERVICE TEMPLATES ROADMAP
│
├── PHASE 0: GOLDEN PATH SKELETONS & SECURE SCAFFOLDER (P0 - WEEKS 1-3)
│   ├── Production-grade templates for Go, Node.js, Python, and Java Spring
│   ├── Automated injection of security linters, Dockerfile distroless, and CI/CD
│   └── Deliverable: Ready-to-deploy compliant services generated in 30 seconds
│
├── PHASE 1: TEMPLATE VERSIONING & REGISTRY (P1 - WEEKS 4-6)
│   ├── Centralized template catalog with semantic versioning
│   ├── Automated pre-commit verification and compilation testing
│   └── Deliverable: Verified, tamper-proof organizational template registry
│
├── PHASE 2: AUTOMATED DRIFT DETECTION & UPGRADE BOT (P2 - WEEKS 7-9)
│   ├── Template lineage tracking (Linking services to exact template commit SHA)
│   ├── Automated Dependabot-style upgrade pull requests for template updates
│   └── Deliverable: Frictionless fleet-wide standardization and security patching
│
└── PHASE 3: COMPLIANCE INTEGRATION & CUSTOM SCAFFOLDING HOOKS (P3 - WEEKS 10-12)
    ├── Dynamic regulatory compliance overlays (PCI-DSS, HIPAA templates)
    └── Deliverable: Turnkey compliant architecture skeletons for enterprise workloads
```
