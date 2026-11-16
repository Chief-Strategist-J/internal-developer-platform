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
