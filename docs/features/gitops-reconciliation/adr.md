# Architecture Decision Records (ADRs): GitOps Reconciliation

## ADR-001: Argo CD as the Exclusive Single Writer for In-Cluster State
- **Status**: Accepted
- **Context**: Allowing multiple microservices or CI/CD pipelines to run `kubectl apply` directly against target Kubernetes clusters creates configuration divergence, unmanaged credentials, and state inconsistency during rollbacks.
- **Decision**: Designate Argo CD as the sole authoritative writer permitted to synchronize manifests to Kubernetes clusters (§0.17). All platform features publish declarative manifests to Git; Argo CD reconciles them continuously.
- **Consequences**: Direct cluster mutations are prohibited; cluster state is 100% reproducible from Git history; drift detection and automated self-healing are natively handled by the Argo CD Application Controller.

---

## ADR-002: Deterministic SHA-256 Merkle Tree Commit Signing for Git Manifest Publishers
- **Status**: Accepted
- **Context**: Git commits created by automated automation engines must guarantee cryptographic tamper-evidence, reproducibility, and conflict-free concurrent branch publication.
- **Decision**: Implement `src/infra/clients/gitops/committer.go` utilizing deterministic SHA-256 Merkle tree calculation across all file paths and payloads before creating signed Git commit objects.
- **Consequences**: Every manifest set is hashed deterministically; git commit trees remain verifiable against original spec inputs; audit trails satisfy strict government supply chain compliance.

---

## ADR-003: Resilient Argo CD REST Client with Exponential Backoff and State Machine Tracking
- **Status**: Accepted
- **Context**: Intermittent network latency or Argo CD server restarts during cluster maintenance can interrupt deployment status queries, resulting in false-positive deployment failures.
- **Decision**: Wrap the Argo CD REST client (`src/infra/clients/argocd/`) in the downstream resilience executor with retry budgets, circuit breakers, and explicit application sync triggering. Transition internal reconciliation records through a formal state machine (`SYNCING`, `HEALTHY`, `DEGRADED`, `UNKNOWN`).
- **Consequences**: Transient Argo CD restarts do not fail workload deployments; applications maintain clear observability into active synchronization phases.
