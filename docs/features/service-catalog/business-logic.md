# Service Catalog — Business Logic Decision Trees

## 1. Architectural Overview & Catalog Registration Decision Tree

```
SERVICE CATALOG ENGINE (DISCOVERY, DEPENDENCY GRAPH & REGISTRATION DECISION TREE)
│
├── 1. ENTITY INGESTION & METADATA HARVESTING TIER [P0]
│   └── Real-World: Developer commits `catalog-info.yaml` to their microservice repository
│       └── Trigger: Inbound Git Webhook or Scheduled Periodic Catalog Crawler
│           ├── Action: Fetch `catalog-info.yaml` from Repository Root
│           ├── Check: Is Entity Descriptor valid against CNCF / Backstage Catalog Schema v1alpha1?
│           │   ├── NO ──> [REJECT: HTTP 422 Unprocessable Entity — Schema Validation Error]
│           │   └── YES ──> Check: Is Entity Name Globally Unique within Tenant Domain?
│           │       ├── COLLISION ──> [REJECT: HTTP 409 Conflict — Entity Name Already Registered]
│           │       └── UNIQUE ──> Ingest Entity (Component, API, Resource, System, Domain)
│           │
├── 2. OWNERSHIP & TEAM VALIDATION TIER [P0]
│   └── Real-World: Validating that `owner: team-payments` maps to an active team in the identity provider
│       └── Evaluation: Ownership Verification
│           ├── Action: Query Identity Store for `spec.owner`
│           ├── Check: Does Specified Team / User exist and have active members?
│           │   ├── NO ──> [REJECT: Orphaned Service Prohibited — Owner must be valid enterprise team]
│           │   └── YES ──> Owner Bound: Attach PagerDuty on-call escalation policy
│           │
├── 3. DEPENDENCY GRAPH & TOPOLOGY RESOLUTION TIER [P0 / P1]
│   └── Real-World: Resolving dependencies (`dependsOn: [service:auth, database:postgres-billing]`)
│       └── Evaluation: Directed Acyclic Graph (DAG) Compilation
│           ├── Action: Resolve Downstream Services & Resources
│           ├── Action: Recompute Global Service Dependency Graph
│           ├── Check: Does Dependency Link reference an Unknown or Deleted Entity?
│           │   ├── YES ──> Mark Dependency as UNRESOLVED (Flag in Health Scorecard)
│           │   └── NO ──> Dependency Graph Node Verified
│           └── State: ENTITY_COMMITTED_TO_CATALOG
│
└── 4. API SPECIFICATION & CONTRACT PUBLISHING TIER [P1]
    └── Real-World: Microservice provides OpenAPI / gRPC Protobuf definition; catalog compiles interactive docs
        └── Evaluation: API Contract Parsing
            ├── Action: Ingest OpenAPI / AsyncAPI / Protobuf definition
            ├── Check: Is Contract Syntax Valid and Lint-Clean (Spectral Linter)?
            │   ├── NO ──> [WARN: Publish with Contract Linting Warnings]
            │   └── YES ──> Action: Compile Interactive Swagger / Redoc Explorer UI
            └── State: API_EXPLORER_LIVE
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Orphaned Services (Mandatory Valid Ownership)
- Real-Life Incident: A payment webhook service broke during production; alerts fired to an empty Slack channel because the service had no recorded owner, delaying incident response by 4 hours.
- Rationale: No service may exist in the catalog or run in production without a verified owning team and an active PagerDuty escalation policy.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ZERO ORPHANED SERVICES (P0-01)
Trigger: Service Catalog Registration / Nightly Audit
├── Extract: `spec.owner`
├── Check: Does Owner exist in Identity Provider AND have at least 1 active member?
│   ├── YES ──> Ownership Verified
│   └── NO ──> [HALT: Flag Service ORPHANED — Page Platform Ops to reassign owner immediately]
```

### Invariant P1-02: Cryptographic Entity Source Verification
- Real-Life Incident: An attacker submitted an unvetted catalog descriptor spoofing the core authentication service, injecting malicious API documentation into the portal.
- Rationale: Catalog metadata must originate from verified corporate Git repositories with verified commit signatures.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: SOURCE REPO VERIFICATION (P1-02)
Trigger: Entity Registration via URL
├── Check: Does URL belong to Whitelisted Enterprise Git Provider Organization?
│   ├── NO ──> [HARD REJECT: Untrusted Git Host]
│   └── YES ──> Allow Registration
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Service Renamed in Git Causing Ghost Duplicates
- Real-Life Incident: A team renamed their service from `order-srv` to `orders-api` in Git; the catalog created a new entry while retaining the old dead entry as a zombie.
- Decision Tree:
```
EDGE CASE 01: GHOST CATALOG ENTITY
Trigger: Crawler scans repository and discovers Entity Name changed
├── Action: Match Repository URL (`metadata.annotations['github.com/project-slug']`)
├── Check: Does an existing Catalog entity map to identical Repo URL?
│   ├── YES ──> Action: Execute Atomic Entity Rename (Update ID, preserve history, prune old slug)
│   └── NO ──> Register as fresh entity
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
SERVICE CATALOG ROADMAP
│
├── PHASE 0: CORE ENTITY REGISTRY & SCHEMA LINTER (P0 - WEEKS 1-3)
│   ├── Backstage-compliant `catalog-info.yaml` parser and database storage
│   ├── Mandatory ownership validation and team mapping engine
│   └── Deliverable: Single authoritative registry of all software components
│
├── PHASE 1: DEPENDENCY TOPOLOGY & API CONTRACTS (P1 - WEEKS 4-6)
│   ├── Automated dependency graph compilation and blast radius visualizer
│   ├── OpenAPI / AsyncAPI interactive contract documentation viewer
│   └── Deliverable: Comprehensive service architecture topology and API discovery
│
├── PHASE 2: HEALTH SCORECARDS & PRODUCTION READINESS (P2 - WEEKS 7-9)
│   ├── Automated scoring on test coverage, security scans, SLOs, and docs
│   ├── Gamified tier certification (Bronze, Silver, Gold readiness tiers)
│   └── Deliverable: Quantifiable engineering standard governance across all teams
│
└── PHASE 3: RUNTIME DISCOVERY & AUTOMATED TOPOLOGY SYNCHRONIZATION (P3 - WEEKS 10-12)
    ├── eBPF / Service Mesh runtime network traffic observation
    ├── Automated detection of undeclared microservice dependencies
    └── Deliverable: 100% self-healing, real-time dynamic architecture mapping
```
