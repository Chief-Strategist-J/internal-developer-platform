# Architecture Decision Records (ADRs): Policy as Code

## ADR-001: In-Process CEL (`cel.dev/cel-go`) Admission Policy Engine & AST Pre-Compilation
- **Status**: Accepted
- **Context**: Relying exclusively on external validating/mutating webhook servers introduces synchronous network latency, TLS certificate rotation overhead, and availability bottlenecks into the critical provisioning path.
- **Decision**: Integrate `cel.dev/cel-go` directly into the platform runtime (`src/infra/clients/admission/`). Parse and compile CEL expression ASTs into immutable `cel.Program` evaluators during service initialization.
- **Consequences**: Policy validation executes in-process in <1ms without network I/O; syntax and type errors in policies are caught at boot time rather than at request evaluation time.

---

## ADR-002: Six-Point Baseline Government Constraint Enforcement & Fail-Closed Evaluation
- **Status**: Accepted
- **Context**: Government platform workloads must comply with strict security baselines before manifests reach GitOps or Kubernetes admission. Any ambiguity in policy evaluation must not allow insecure workloads to proceed.
- **Decision**: Enforce six mandatory baseline policies:
  1. `GOV_SEC_001_POD_SECURITY_RESTRICTED`: Pod security level must be strictly `restricted`.
  2. `GOV_NET_002_MESH_INJECTION_MANDATORY`: Managed Cloud Service Mesh sidecar injection must be `true`.
  3. `GOV_NET_003_DEFAULT_DENY_NETPOL`: Namespace must enforce default-deny network policy.
  4. `GOV_IMG_004_IMMUTABLE_IMAGE_DIGEST`: Container image must pin an immutable `@sha256:` digest.
  5. `GOV_GOV_005_MANDATORY_GOVERNANCE_LABELS`: Workload must carry `platform.gov/request-id`, `platform.gov/owner`, and `platform.gov/cost-center`.
  6. `GOV_SEC_006_PRIVILEGED_CONTAINER_DISALLOWED`: Privileged containers are strictly prohibited.
  Evaluation strictly follows fail-closed semantics: any runtime evaluation failure or missing context variable instantly records a violation and rejects admission.
- **Consequences**: Non-compliant workloads cannot breach platform admission; every rejection provides structured, actionable failure messages.

---

## ADR-003: Strict Point-of-Origin Error Telemetry Stamping on Admission Child Spans
- **Status**: Accepted
- **Context**: Collapsing admission checks into coarse parent spans or omitting error records on child spans obscures which specific policy failed during distributed trace analysis in Grafana Tempo.
- **Decision**: Strictly uphold negative invariants 9 & 10 of `10-tracing-writing.md`:
  1. Admission evaluation runs under a dedicated child span: `kubernetes.workload.admission.cel_evaluate`.
  2. Whenever a policy evaluation generates an error or violation, the error is immediately stamped at the point of origin via `observability.RecordError(span, err)` before bubbling to the caller.
- **Consequences**: Zero orphaned errors in distributed traces; SREs and platform auditors can isolate the exact failing constraint within 60 seconds.

