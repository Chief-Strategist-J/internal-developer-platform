# Architecture Decision Records (ADRs): Kubernetes Workload Provisioning

## ADR-001: In-Process Admission Validation vs External Webhook Engine
- **Status**: Accepted
- **Context**: Government platform standards mandate strict admission verification (non-root execution, dropped capabilities, read-only root filesystems, PDB creation) before any resource manifest reaches Git or the Kubernetes API Server.
- **Decision**: We enforce a dual-layer defense:
  1. In-process validation inside `rules.ValidateAdmission` during the Workload API request cycle, rejecting invalid configurations before persisting them to Postgres or Git.
  2. In-cluster CEL validation and admission controllers (Kyverno / PSS) as the final cluster enforcement barrier.
- **Consequences**: Developers receive instant sub-second feedback via the REST API without waiting for asynchronous GitOps sync failures, while the cluster remains protected even against direct out-of-band submissions.

---

## ADR-002: Restricted Pod Security Standards as Mandatory Floor
- **Status**: Accepted
- **Context**: CIS GKE Benchmark requires hardened container boundaries to eliminate privilege escalation, container escapes, and host compromises.
- **Decision**: Every generated workload manifest hardcodes:
  - `runAsNonRoot: true`
  - `runAsUser: 10001`
  - `runAsGroup: 10001`
  - `readOnlyRootFilesystem: true`
  - `allowPrivilegeEscalation: false`
  - `capabilities.drop: ["ALL"]`
  - `seccompProfile: { type: "RuntimeDefault" }`
- **Consequences**: Workloads cannot write to the root filesystem (must use explicit emptyDir memory volumes for ephemeral temp files) and cannot run as root.

---

## ADR-003: Automated PodDisruptionBudget for Multi-Replica Workloads
- **Status**: Accepted
- **Context**: Cluster node pool upgrades and autoscaler scale-down operations can cause brief outages if all replicas of a service are terminated simultaneously.
- **Decision**: Any workload with `replicas >= 2` automatically triggers generation and binding of a `PodDisruptionBudget` with `minAvailable: 1`.
- **Consequences**: Guarantees high availability during routine maintenance; single-replica workloads (`replicas == 1`) are exempt to prevent blocking node maintenance.

---

## ADR-004: Platform Chassis Two-Tree Architecture & Lifecycle Isolation
- **Status**: Accepted
- **Context**: Monolithic `main()` entrypoints couple service-specific business wiring with generic platform runtime primitives (configuration loading, signal trapping, server drain, telemetry registration). This results in lifecycle divergence across microservices.
- **Decision**: Strictly separate execution into two trees per `main-file.md`:
  1. **Tree 1 (`src/main.go`)**: Only writes `describeService()` declaring identity, dependencies, contracts, and router bindings.
  2. **Tree 2 (`src/infra/chassis`)**: Standardized platform runtime executing preflight, configuration validation, composition, dual-server wiring, and graceful multi-stage shutdown.
- **Consequences**: Eliminates boilerplate across services; guarantees consistent signal handling (`SIGTERM`/`SIGINT`), uniform port allocation, and standardized exit codes (`EXIT_USAGE`, `EXIT_CONFIG`, `EXIT_DEPENDENCY`, `EXIT_TIMEOUT`).

---

## ADR-005: Universal W3C Distributed Tracing and Structured JSON Log Invariant
- **Status**: Accepted
- **Context**: Unstructured stdout prints (`log.Println`, `fmt.Printf`) and untraced HTTP handlers make cross-service failure correlation in Grafana Loki and Tempo impossible during high-severity production incidents.
- **Decision**: Enforce zero raw standard prints. All logging must serialize through `src/infra/observability` into canonical JSON (`timestamp`, `level`, `service`, `message`, `trace_id`, `span_id`, `tenant_id`, `attributes`). All ingress endpoints extract W3C `traceparent`/`tracestate` and mirror `X-Trace-Id` and `X-Span-Id` on responses.
- **Consequences**: Every incoming request produces a traceable span and correlated log lines; 4xx errors are logged as `WARN` and 5xx as `ERROR`, strictly upholding the three-step failure invariant.

---

## ADR-006: Soft-Dependency Startup & Fleet Connection Pool Budgeting
- **Status**: Accepted
- **Context**: Hard-blocking service startup on external peer availability (e.g. Argo CD, Kubernetes API Server, Postgres) causes cascading boot deadlocks across the fleet during cold cluster restarts. Additionally, unbounded database connection pools exhaust Postgres backend capacity.
- **Decision**: 
  1. Mark external clients (`kubernetes-api`, `argocd-server`) as soft startup dependencies (`CriticalitySoft`). Services must boot and serve health checks even if downstream peers are initializing.
  2. Database connection pools must be explicitly budgeted via declarative configuration (`max_open_conns: 25`, `max_idle_conns: 10`, `conn_max_lifetime_minutes: 5`) with non-blocking ping fallbacks.
- **Consequences**: Prevents fleet-wide restart deadlocks; shields database servers from connection exhaustion during horizontal autoscaling.
