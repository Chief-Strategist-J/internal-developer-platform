# Developer Portal — Business Logic Decision Trees

## 1. Architectural Overview & Portal Gateway Decision Tree

```
DEVELOPER PORTAL ENGINE (UI/CLI/API ORCHESTRATION DECISION TREE)
│
├── 1. USER AUTHENTICATION & CONTEXT INITIALIZATION TIER [P0]
│   └── Real-World: Developer opens internal portal dashboard or runs `idp-cli login`
│       └── Trigger: Ingress UI Session / CLI Ingress Request
│           ├── Check: Is User authenticated via Corporate SSO / OIDC?
│           │   ├── NO ──> [REDIRECT: OIDC Authorization Code Flow + PKCE]
│           │   └── YES ──> Resolve Active Profile (User ID, Assigned Teams, Tenant Roles)
│           │       ├── Action: Fetch Bookmarked Services, Active Deployments, and Open Incident Alerts
│           │       └── State: CONTEXT_HYDRATED (Render Personalized Developer Home Screen)
│           │
├── 2. PLUGIN REGISTRY & COMPONENT RESOLUTION TIER [P1]
│   └── Real-World: Rendering Backstage / custom portal cards (Cost, CI/CD, Kubernetes Pods, PagerDuty)
│       └── Evaluation: Plugin Ingestion Pipeline
│           ├── Check: Are requested plugins permitted under Tenant Security Policy?
│           │   ├── NO ──> [SUPPRESS: Hide unapproved plugin widgets from layout]
│           │   └── YES ──> Action: Parallel Asynchronous Backend Aggregation:
│           │       ├── Sub-Request 1: Query Service Catalog API (Metadata, Ownership, API Docs)
│           │       ├── Sub-Request 2: Query Kubernetes Workload API (Pod health, Replica status)
│           │       ├── Sub-Request 3: Query CI/CD Engine (Latest Pipeline status, Commit SHA)
│           │       └── Sub-Request 4: Query FinOps Engine (Monthly Cloud Cost Burn Rate)
│           │       └── Merge: Hydrate Unified Entity View within 250ms SLA
│           │
├── 3. SELF-SERVICE ACTION SUBMISSION TIER [P0]
│   └── Real-World: Developer triggers 'Restart Pods' or 'Execute Ad-Hoc Database Backup' from UI
│       └── Trigger: User clicks Action Button in Portal Component
│           ├── Check: Does Subject have explicit RBAC permission for this Action on this Resource?
│           │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Insufficient RBAC Permissions]
│           │   └── YES ──> Check: Is Action classified as DESTRUCTIVE / SENSITIVE?
│           │       ├── SENSITIVE ──> Require Step-Up Re-Authentication (WebAuthn / YubiKey touch)
│           │       └── STANDARD ──> Action Dispatched
│           ├── Action: Construct Idempotent Command Envelope -> Submit to Platform Workflow Bus
│           └── State: ACTION_DISPATCHED (Display real-time streaming progress bar via WebSocket)
│
└── 4. REAL-TIME EVENT STREAMING & NOTIFICATION TIER [P1]
    └── Real-World: Long-running deployment updates progress bar from 10% to 100% live on portal screen
        └── Evaluation: Push Notification Bus
            ├── Action: Establish Client WebSocket / Server-Sent Events (SSE) Channel
            ├── Filter: Stream only events subscribed by User or owned by User's Teams
            └── Heartbeat: Ping every 30s; reconnect with exponential backoff on network drop
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Portal Gateway Never Bypasses Underlying Service RBAC
- Real-Life Incident: A developer used browser DevTools to edit a hidden button in the portal UI, triggering a production deployment that the UI had disabled but the backend forgot to authorize.
- Rationale: The Developer Portal is an aggregation presentation layer. Every action submitted must be independently re-authenticated and authorized at the API gateway layer.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: BACKEND AUTHORIZATION (P0-01)
Trigger: Portal Action Submission
├── Action: Extract Forwarded User Identity Token (JWT)
├── Action: Transmit User Identity Token downstream to core microservice
├── Enforce: Downstream microservice PDP MUST evaluate RBAC independently
└── Rule: Portal backend service account NEVER impersonates root or bypasses user context
```

### Invariant P1-02: Graceful Degradation on Backend Plugin Failure
- Real-Life Incident: The JIRA plugin backend crashed; the entire developer portal threw an unhandled React error boundary white screen, blocking all engineers from accessing Kubernetes logs.
- Rationale: A failure in an ancillary plugin (cost, tickets, documentation) must never crash the portal. Plugins must isolate errors and render local fallback states.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: PLUGIN FAULT ISOLATION (P1-02)
Trigger: Backend Plugin Ingestion
├── Check: Did Plugin API respond within 1500ms timeout?
│   ├── YES ──> Render Widget Content
│   └── NO (Timeout or HTTP 5xx)
│       └── [LOCAL FALLBACK: Render widget placeholder with 'Temporarily Unavailable' retry button]
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: High-Frequency WebSocket Churn During Incidents
- Real-Life Incident: 5,000 developers opened the portal during a major outage to watch live deployment status, exhausting portal WebSocket connections and crashing the frontend server.
- Decision Tree:
```
EDGE CASE 01: WEBSOCKET THUNDERING HERD
Trigger: Active Portal WebSocket Connections > 80% Capacity
├── Action: Activate Ingress Edge Fan-Out via Redis Pub/Sub / CDN Edge
├── Action: Downsample UI broadcast frequency from 100ms to 2000ms
└── Output: Stable real-time broadcast without server collapse
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
DEVELOPER PORTAL ROADMAP
│
├── PHASE 0: CORE CATALOG UI & SSO LOGIN (P0 - WEEKS 1-3)
│   ├── Enterprise SSO integration with unified OIDC auth
│   ├── Core Service Catalog browser and API documentation viewer
│   ├── Backend proxy enforcing end-to-end token forwarding
│   └── Deliverable: Centralized single-pane portal for service discovery
│
├── PHASE 1: SELF-SERVICE ACTIONS & WORKFLOW RUNNER (P1 - WEEKS 4-6)
│   ├── Interactive service scaffolding forms with dynamic parameter validation
│   ├── Real-time workflow execution progress streaming (SSE / WebSockets)
│   └── Deliverable: Frictionless self-service creation of services from the UI
│
├── PHASE 2: OBSERVABILITY & KUBERNETES PLUGINS (P2 - WEEKS 7-9)
│   ├── In-portal Kubernetes pod status, logs, and metrics viewer
│   ├── Distributed tracing and error rate graphs embedded in service pages
│   └── Deliverable: Contextual day-2 debugging directly from the service dashboard
│
└── PHASE 3: COMPLIANCE SCORECARDS & FINOPS INSIGHTS (P3 - WEEKS 10-12)
    ├── Automated production readiness scorecard per service
    ├── Real-time cloud cost allocation and resource waste recommendations
    └── Deliverable: Comprehensive engineering health and efficiency governance
```
