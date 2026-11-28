# Architecture Decision Records (ADRs) — Deployment Strategies

## ADR-001: Hexagonal Rollout Engine & PostgreSQL Rollout Execution Ledger
- **Status:** Accepted & Mechanized
- **Context:**
  Progressive delivery engines (such as Argo Rollouts or Flagger) often keep rollout state transiently inside Kubernetes CRD status fields (`status.canaryStepIndex`, `status.phase`). Under controller restarts, etcd compaction, or network partitioning between clusters and control planes, this leads to split-brain rollouts, untracked traffic weights, and un-audited canary promotions.
- **Decision:**
  Enforce a centralized PostgreSQL ledger in schema `deployment_strategies`:
  1. `rollout_plans`: Stores declarative strategy templates, step progressions, and analysis SLO thresholds.
  2. `rollout_executions`: Durable state machine ledger storing active step index, current weight, lease tokens, and execution status (`INITIALIZING`, `ROUTING_STEP`, `ANALYZING`, `PAUSED`, `PROMOTING`, `COMPLETED`, `ABORTING`, `ABORTED`).
  3. `rollout_analysis_runs`: Append-only time-series ledger of individual Prometheus metric queries, observed values, and pass/fail verdicts.
  All tables strictly enforce Data Contract v1.2: surrogate UUIDv7 primary keys, composite uniqueness `(tenant_id, id)`, immutable public IDs (`dplan_...`, `dexec_...`), and Row-Level Security tenant isolation.
- **Consequences:**
  - Auditable, durable history of every traffic split and metric evaluation.
  - Zero state loss during controller crashes or cluster failovers.

---

## ADR-002: Cloud Service Mesh Native Traffic Shifting & Decoupled Port Architecture
- **Status:** Accepted & Mechanized
- **Context:**
  Contract v3.0 Invariant A mandates 1 file per adapter, zero vendor leakage into domain rules, and clean hexagonal port boundaries. Workloads must support traffic shifting across both Cloud Service Mesh (Istio Envoy) and native Kubernetes service selectors.
- **Decision:**
  Define discrete hexagonal ports in `internal/infrastructure/ports/`:
  - `TrafficPort` (`traffic.port.go`): Abstract interface defining `SetTrafficWeight(ctx, workload, weightPct) (Result, error)` and `GetTrafficWeight(ctx, workload) (int, error)`.
  - `OrchestratorPort` (`orchestrator.port.go`): Abstract interface for managing pod replica counts and rollout revisions.
  - `AnalysisPort` (`analysis.port.go`): Abstract interface for querying real-time telemetry metrics.
  - `RepositoryPort` (`repository.port.go`): Interface for durable PostgreSQL ledger operations.
  Implement concrete adapters in `internal/infrastructure/adapters/`:
  - `mesh_traffic.adapter.go`: Updates Istio `VirtualService` HTTP route destination weights via `k8s.io/client-go`.
  - `argo_rollouts.adapter.go`: Manages Argo Rollouts custom resources via `argoproj/argo-rollouts`.
  - `prometheus_analysis.adapter.go`: Evaluates PromQL queries via `prometheus/client_golang` API.
  - `postgres_repository.adapter.go`: Executes transactional ledger updates using centralized SQL queries in `queries/`.
- **Consequences:**
  - Complete decoupling of domain progressive delivery logic from specific proxy or mesh implementations.

---

## ADR-003: Prometheus Continuous Analysis Gates & Statistical Failure Thresholds
- **Status:** Accepted & Mechanized
- **Context:**
  Canary deployments require automated health evaluation to prevent user-facing outages caused by buggy code. Flaky or transient network blips can cause false-positive rollbacks if evaluated naively on single data points.
- **Decision:**
  Implement continuous analysis gates with configurable consecutive failure damping:
  1. Prometheus queries execute at fixed intervals (`cfg.analysis.interval`, e.g. every 10s) during each step's pause window.
  2. A step is marked failed ONLY when the failure condition is satisfied for `cfg.analysis.consecutive_failures` consecutive samples.
  3. Minimum sample size clamp: if the request rate is below `cfg.analysis.min_samples`, the analysis window is extended up to `cfg.analysis.max_extension` before an inconclusive verdict is rendered.
- **Consequences:**
  - High noise immunity while maintaining rapid sub-minute abort response times for genuine outages.

---

## ADR-004: Invariant P0-03 Pre-Pod Traffic Shifting Prohibition & Circuit Breaking
- **Status:** Accepted & Mechanized
- **Context:**
  Directing production traffic to unready pods causes immediate 502/503 HTTP drops.
- **Decision:**
  Strict domain rule enforcement (`deployment-strategies.rules.go`):
  Traffic shifting to canary pods is prohibited until:
  - 100% of canary target pods report `Ready = True`.
  - Envoy proxy sidecars confirm active mTLS STRICT session readiness.
  - EndpointSlice controller has populated healthy IP endpoints.
  Failure to meet these conditions within `cfg.pod_ready_timeout` aborts the rollout without shifting a single request.
- **Consequences:**
  - Guarantees zero dropped traffic during progressive canary ramps.

---

## ADR-005: Expand/Contract Database Migration Coupling Isolation
- **Status:** Accepted & Mechanized
- **Context:**
  Progressive rollouts of applications sharing a database can cause fatal errors if a new application version requires a new database column before the previous version is decommissioned.
- **Decision:**
  The rollout engine checks database migration state before initializing canary progression. Workloads undergoing database schema migrations must prove dual-write / expand-contract backward compatibility in `context.yml` before progressive traffic shifting is permitted.
- **Consequences:**
  - Prevents fatal column-missing or schema mismatch errors during multi-version coexistence.
