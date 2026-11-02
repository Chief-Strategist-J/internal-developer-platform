# Health Checks — Business Logic Decision Trees

## 1. Architectural Overview & Health Probing Decision Tree

```
HEALTH CHECKS ENGINE (MULTI-TIER PROBING & TRAFFIC GATING DECISION TREE)
│
├── 1. PROBE INGESTION & CONFIGURATION TIER [P0]
│   └── Real-World: Microservice boots up; Kubelet & Service Mesh probe `/healthz/startup`, `/healthz/ready`, `/healthz/live`
│       └── Evaluation: Tri-Partite Probe Architecture
│           ├── PROBE TYPE 1: STARTUP PROBE (`/healthz/startup`)
│           │   ├── Purpose: Protect slow-starting apps (JVM, large cache hydration) from premature death
│           │   ├── Parameters: `initialDelaySeconds: 5`, `periodSeconds: 10`, `failureThreshold: 30` (Allows up to 300s to boot)
│           │   └── Check: Does Startup Probe return HTTP 200 within threshold?
│           │       ├── NO ──> [KILL & RESTART: Container failed to initialize within 300s deadline]
│           │       └── YES ──> Container Booted ──> Deactivate Startup Probe, Enable Liveness & Readiness
│           ├── PROBE TYPE 2: READINESS PROBE (`/healthz/ready`)
│           │   ├── Purpose: Gate incoming network traffic (Service endpoints)
│           │   ├── Evaluation: Shallow vs Deep Dependency Verification (Tier 2)
│           │   └── Outcome:
│           │       ├── PASS (HTTP 200) ──> [ATTACH: Pod IP registered in Service Endpoints & Mesh Routing]
│           │       └── FAIL (HTTP 503) ──> [DETACH: Pod IP severed from Service Endpoints; Pod keeps running]
│           └── PROBE TYPE 3: LIVENESS PROBE (`/healthz/live`)
│               ├── Purpose: Detect deadlocks, infinite loops, and unrecoverable thread pool exhaustion
│               ├── Evaluation: Lightweight Internal Heartbeat (NO external network calls)
│               └── Outcome:
│                   ├── PASS (HTTP 200) ──> Container Healthy
│                   └── FAIL (HTTP 500) for `failureThreshold: 3` consecutive times
│                       └── [KILL & RESTART: Kubelet executes container SIGKILL and crash restart]
│
├── 2. READINESS DEPENDENCY CIRCUIT BREAKING TIER [P0 / P1]
│   └── Real-World: Downstream database experiences momentary 10-second lag; app readiness probes react
│       └── Evaluation: Deep Dependency Probing Strategy
│           ├── Check: Does Readiness Probe query external downstream dependencies (DB, Redis, Third-Party)?
│           │   ├── YES (Dangerous Anti-Pattern)
│           │   │   ├── Check: Downstream DB Slow / Unreachable?
│           │   │   │   ├── YES ──> [CASCADE DANGER: All 50 pods fail readiness simultaneously -> 100% Traffic Outage]
│           │   │   │   └── Safe Pattern: Readiness must test LOCAL service state, queue capacity, and memory buffers
│           │   └── NO (Safe Local Probe)
│           │       ├── Check: Is Local App overloaded (active connections at thread limit)?
│           │       │   ├── YES ──> Return HTTP 503 (Temporarily shedding traffic until queue clears)
│           │       │   └── NO ──> Return HTTP 200 (Ready for traffic)
│           │
├── 3. GRACEFUL DRAIN & SHUTDOWN TIER [P0]
│   └── Real-World: Deployment triggers pod eviction; pod must finish in-flight HTTP requests before exit
│       └── Trigger: Pod Receives SIGTERM from Kubelet
│           ├── Step 1: Readiness Probe instantly flips to HTTP 503
│           ├── Step 2: Kubelet / Ingress removes Pod IP from endpoints (takes 5-15s to propagate)
│           ├── Step 3: Container `preStop` hook executes `sleep 15` (Allows in-flight packets to finish arriving)
│           ├── Step 4: Application server stops accepting new connections; drains existing connections
│           ├── Check: Have all in-flight connections drained within `terminationGracePeriodSeconds` (default 60s)?
│           │   ├── YES ──> Clean exit with code 0
│           │   └── NO (Timeout Reached) ──> Kubelet sends SIGKILL (Force terminate remaining hung threads)
│           └── State: POD_TERMINATED_CLEANLY
│
└── 4. SYNTHETIC CANARY & EXTERNAL ENDPOINT MONITORING TIER [P1]
    └── Real-World: Blackbox exporter runs synthetic HTTP login journeys from external internet regions
        └── Evaluation: External Availability & SSL Health
            ├── Action: Dispatch Synthetic Probe from 5 global edge locations every 30s
            ├── Check: Probe Latency > 1500ms OR SSL Certificate Expires in < 14 Days?
            │   ├── YES ──> Raise Alert to Network Operations / SecOps
            │   └── NO ──> Edge Connectivity Verified
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Liveness Probes Must Never Call External Dependencies
- Real-Life Incident: A developer coded the liveness probe to execute `SELECT 1 FROM postgres`; when Postgres restarted, all 200 microservice pods failed liveness and entered CrashLoopBackOff simultaneously, causing complete company outage.
- Rationale: Liveness determines whether a process is deadlocked or alive. Checking downstream networks causes cascading cluster death. Liveness probes must strictly be local in-memory checks.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: ISOLATED LIVENESS (P0-01)
Trigger: Workload Manifest Admission
├── Inspect: Liveness Probe Handler Endpoint (`/healthz/live`)
├── Static Rule: Liveness probe code must never establish network socket or DB connection
├── Check: Does Liveness probe timeout or fail due to network partition?
│   ├── YES ──> [DESIGN VIOLATION: Flag Policy Violation — Decouple Liveness from external systems]
│   └── NO ──> Compliant Local In-Memory Heartbeat
```

### Invariant P0-02: Mandatory PreStop Sleep for Zero-Downtime Rolling Restarts
- Real-Life Incident: During rolling deployment, pods terminated instantly on SIGTERM; the Ingress controller took 4 seconds to remove their IPs from iptables, causing hundreds of HTTP 502 errors to clients.
- Rationale: In Kubernetes, endpoint removal is asynchronous. A pod must wait (via a preStop hook sleep) before closing its listening socket.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: PRESTOP DRAIN DELAY (P0-02)
Trigger: Workload Manifest Generation
├── Check: Does Pod Spec include `lifecycle.preStop.exec`?
│   ├── NO ──> [MUTATE: Automatically inject `exec: command: ["/bin/sh", "-c", "sleep 15"]`]
│   └── YES ──> Ensure sleep duration >= 10 seconds
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Slow Cold-Start Hydration During High-Scale Deployment
- Real-Life Incident: A Spring Boot service took 90 seconds to load caches on startup; Kubernetes liveness probe killed it every 30 seconds, creating an endless restart loop.
- Decision Tree:
```
EDGE CASE 01: COLD-START RESTART LOOP
Trigger: Pod Restart Count Increments during Initial Deployment
├── Check: Is Startup Probe configured on container?
│   ├── NO ──> [MUTATE & FIX: Inject Startup Probe with generous threshold (30 * 10s = 300s)]
│   └── YES ──> Check if failure is unhandled crash vs slow boot
```

### Case 02: Flapping Readiness Probes Causing Endpoint Throttling
- Real-Life Incident: A busy garbage collection pause caused readiness to fail for 1 second every 10 seconds; the Ingress controller was overwhelmed recalculating endpoint routes.
- Decision Tree:
```
EDGE CASE 02: READINESS FLAPPING
Trigger: Readiness State Flips > 3 times in 60 seconds
├── Action: Apply Damping Filter
├── Action: Increase `readinessProbe.failureThreshold = 3` and `periodSeconds = 5`
└── Result: Temporary GC pauses do not trigger endpoint churn
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
HEALTH CHECKS ROADMAP
│
├── PHASE 0: TRI-PARTITE PROBE STANDARD & ADMISSION ENFORCEMENT (P0 - WEEKS 1-3)
│   ├── Automated injection of Startup, Liveness, and Readiness probes
│   ├── Mandatory preStop drain hook enforcement for zero 502 errors
│   ├── Platform guardrail: Strict ban on external network calls in Liveness probes
│   └── Deliverable: Bulletproof zero-downtime deployments with zero client dropped connections
│
├── PHASE 1: BLACKBOX & SYNTHETIC EDGE PROBING (P1 - WEEKS 4-6)
│   ├── Prometheus Blackbox Exporter deployment across multiple edge zones
│   ├── Automated SSL/TLS certificate expiry alerting (30/14/7-day thresholds)
│   └── Deliverable: External customer-perspective health monitoring
│
├── PHASE 2: ADAPTIVE PROBE DECOUPLING & TRAFFIC SHEDDING (P2 - WEEKS 7-9)
│   ├── Local queue-depth and concurrency-based readiness shedding
│   ├── Integration with Envoy / Istio outlier detection
│   └── Deliverable: Self-protecting microservices that shed overload without restarting
│
└── PHASE 3: ML-POWERED ANOMALOUS LATENCY FORECASTING (P3 - WEEKS 10-12)
    ├── Continuous probe latency regression analysis
    ├── Predictive health degradation alerting before hard probe failures
    └── Deliverable: Autonomous self-healing infrastructure remediating degradations
```
