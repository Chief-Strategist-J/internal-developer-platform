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

---

## ADR-007: Three-API Platform Topology and Canonical Infrastructure Layout
- **Status**: Accepted
- **Context**: Government platform standards mandate strict isolation of duties. Merging cluster lifecycles, cloud resource bindings, and container deployments creates massive blast radiuses and violates principle of least privilege.
- **Decision**: Segregate the platform into three strictly ordered, decoupled APIs:
  1. `Cluster API (§4.1)`: Lifecycles hardened GKE clusters enforcing CIS benchmarks, private endpoints, shielded nodes, and 9-point posture validation.
  2. `Cloud Resource API (§4.2)`: Declaratively provisions Workload Identity IAM bindings, private DNS records, and Cloud SQL databases via Kubernetes Config Connector (KCC).
  3. `Workload API (§4.3)`: Orchestrates application namespaces, quotas, in-process CEL admission, and rollouts.
  All technical outbound adapters are consolidated strictly under `src/infra/clients/` behind typed Result envelopes and zero-comment contracts per `0-package-folder-writing-rule.md`.
- **Consequences**: Workloads cannot mutate clusters; cloud resource dependencies are strictly validated before workload deployment; code layout remains 100% compliant with canonical package rules.

---

## ADR-008: Declarative Google Config Connector (KCC) Manifest Generation & Resilient Ledger
- **Status**: Accepted
- **Context**: Imperative cloud provider SDK calls bypass version control, create unmanaged state drift, and leave partial failures unrecorded during cloud outages.
- **Decision**: The Cloud Resource client compiles typed provisioning specs into declarative Google Config Connector (KCC) manifests (`IAMPolicyMember`, `DNSRecordSet`, `SQLDatabase`, `SQLUser`), tracks lifecycle states in a resilient in-memory ledger, enforces cloud-side deletion protection checks, and exposes automated sweep reconciliation to detect orphaned resources.
- **Consequences**: Cloud infrastructure is versioned in Git and reconciled by Argo CD; orphaned resources are deterministically identified and torn down; accidental deletion of protected databases is blocked.

---

## ADR-009: Downstream Resilient Client Architecture & Integer Milli-Token Rate Limiter
- **Status**: Accepted
- **Context**: Transient network partitions, rate limits, and slow upstream dependencies can cause cascading thread exhaustion. Additionally, standard floating-point rate-limiting algorithms suffer from IEEE 754 precision drift over long runtimes.
- **Decision**: Implement a unified resilience executor (`src/infra/clients/resilience/`) combining:
  1. Concurrency Bulkhead with channel-based slot acquisition.
  2. Circuit Breaker with Consecutive Failure Threshold and half-open probes.
  3. Exponential backoff retry budget with full jitter.
  4. Token bucket rate limiter calculating capacity exclusively in integer milli-tokens.
- **Consequences**: Downstream calls fail fast without cascading thread pool exhaustion; token bucket arithmetic remains mathematically exact over millions of operations without float rounding anomalies.

---

## ADR-010: Synthesis of Argo Rollouts, Mesh Routing, and Cloud Resources in the Provisioning DAG
- **Status**: Accepted
- **Context**: Disjoint synthesis of application containers, canary traffic shifting, and cloud infrastructure (Workload Identity, managed databases) leads to partial deployments and out-of-order reconciliation where workloads start before IAM bindings or routing mesh definitions exist.
- **Decision**: Orchestrate manifest synthesis as a unified directed acyclic graph (DAG) within `ExecuteProvisionDAG` under `domain/workflows/`. The DAG coordinates:
  1. `StepValidateSpec` & `StepApplyMutations`: Hardware limits, security context, and probe injection.
  2. `StepVerifyQuota`: Headroom evaluation.
  3. `StepGenerateK8sSpec`: Core Kubernetes manifests (Namespace, Quota, LimitRange, NetworkPolicy, Deployment, PDB).
  4. `StepSynthesizeCanaryRollout`: Declarative `argoproj.io/v1alpha1` `Rollout` manifests with 4-step progressive canary progression (`10% -> 25% -> 50% -> 100%`) and companion `AnalysisTemplate` metric gates.
  5. `StepSynthesizeMeshRouting`: Cloud Service Mesh `VirtualService` single-field traffic weight shifting and `DestinationRule` enforcing mTLS `STRICT` mode.
  6. `StepSynthesizeCloudResources`: Google Config Connector (KCC) manifests for Workload Identity (`IAMPolicyMember`) and Cloud SQL (`SQLDatabase`).
- **Consequences**: Every admitted workload is guaranteed a complete, self-contained multi-tier manifest bundle committed atomically to GitOps. Partial infrastructure states are structurally prevented.

---

## ADR-011: Automated Argo CD ignoreDifferences Generation to Prevent Traffic Routing Sync Fights [V-12]
- **Status**: Accepted
- **Context**: When Argo Rollouts shifts live traffic weights in an Istio `VirtualService`, Argo CD detects the modified `weight` fields as out-of-band Git drift and immediately force-reverts the route back to 0% canary, causing violent traffic fluttering, deployment stalling, and false rollback alarms.
- **Decision**: The Rollout generator automatically emits companion Argo CD `ignoreDifferences` rules:
  1. Target `networking.istio.io/v1alpha3` `VirtualService` with JSON pointer `/spec/http/0/route`.
  2. Target `argoproj.io/v1alpha1` `Rollout` with JSON pointer `/status`.
- **Consequences**: Argo CD yields field ownership over traffic route weights to Argo Rollouts while maintaining ownership over all other spec attributes (hosts, gateways, timeouts). Traffic shifting proceeds without GitOps reconciliation conflicts.

---

## ADR-012: Fail-Closed Metric Analysis Gates in Canary Rollouts
- **Status**: Accepted
- **Context**: If a telemetry pipeline or Prometheus instance drops connection or if a service produces zero metrics due to a configuration bug, optimistic analysis engines treat absent metrics as a passing signal, silently promoting defective code to 100% production traffic.
- **Decision**: The `AnalysisTemplate` generator configures strict fail-closed thresholds:
  1. `failureLimit: 1`: Any single metric threshold violation immediately aborts the canary rollout.
  2. `consecutiveErrorLimit: 1`: Any transport error, timeout, or missing metric stream from Prometheus is classified as an instant failure.
  3. Strict evaluation gates:
     - `canary-error-rate-delta`: `result[0] <= 0.001` (canary 5xx error rate cannot exceed stable baseline by > 0.1%).
     - `canary-p99-latency`: `result[0] <= 200` (canary p99 latency must not exceed 200ms).
- **Consequences**: Zero defective releases can pass silently during observability outages; canary promotions require positive, unbroken telemetry signals to proceed.

---

## ADR-013: Multi-Tenant Ingress Boundary Partitioning via GKE Gateway API
- **Status**: Accepted
- **Context**: In multi-tenant Kubernetes clusters, allowing tenant workload repositories to synthesize root `Gateway` resources risks route collisions, unauthorized listener modifications, and cluster-wide edge downtime.
- **Decision**: Partition ingress ownership between platform infrastructure and tenant workloads:
  1. Platform operators own root `Gateway` resources (`gateway.networking.k8s.io/v1`) in the `gateway-system` namespace.
  2. The workload provisioning engine synthesizes strictly scoped `HTTPRoute` resources in the tenant namespace, anchoring to the shared gateway via cross-namespace `parentRefs` (`group: gateway.networking.k8s.io`, `kind: Gateway`, `namespace: gateway-system`, `name: gke-external-gateway`).
  3. Route matches default to `PathPrefix: /` targeting the workload service with weight 100 and port 8080.
- **Consequences**: Multi-tenant route isolation is enforced at the admission and GitOps boundary. Tenants can configure custom hostnames and path routing without access to cluster-wide edge ingress listeners.

---

## ADR-014: Decoupled Cloud Armor WAF and DDoS Protection via GCPBackendPolicy
- **Status**: Accepted
- **Context**: Directly embedding Cloud Armor security policies inside tenant workload manifests risks exposing or allowing tenants to modify platform-wide DDoS protection rules, rate limits, and OWASP filter profiles.
- **Decision**: Decouple edge WAF policy definitions from tenant application definitions:
  1. Platform security policies (`ComputeSecurityPolicy` under `compute.cnrm.cloud.google.com/v1beta1`) are defined with standard pre-configured OWASP ModSecurity rules and client IP rate limiting.
  2. Tenant workloads attach to the platform security policy via declarative `networking.gke.io/v1` `GCPBackendPolicy` CRDs referencing the tenant's backend `Service` via `targetRef`.
  3. Workload generator configures default backend timeouts (`30s`), connection draining (`60s`), and full telemetry access logging (`sampleRate: 1.0`).
- **Consequences**: Platform security controls remain centralized and immutable, while tenant workloads automatically inherit enterprise-grade Layer-7 WAF and DDoS protection upon provisioning.

---

## ADR-015: Higher-Order Tracing Primitives and Uniform Function Decomposition
- **Status**: Accepted
- **Context**: Repeating manual OpenTelemetry span creation (`tracer.StartSpan`), error recording (`RecordError`), status setting (`codes.Error`/`codes.Ok`), and `span.End()` across dozens of methods created duplicate ceremony, leading to functions ballooning over the 30-line limit mandated by institutional rules.
- **Decision**: Standardize all tracing across the platform on reusable higher-order primitives in `src/infra/observability/tracer.go`:
  1. `observability.WithSpan(ctx, spanName, fn, attrs...) error`
  2. `observability.RunTraced[T](ctx, spanName, fn, attrs...) (T, error)`
  3. Decompose all domain workflows, adapters, and services into single-responsibility functions strictly $\le 30$ lines, moving multiline manifest templates to package constants.
- **Consequences**: Tracing lifecycle management is centralized in one place. Monolithic functions across domain workflows, adapters, and services are reduced from 280+ lines to under 30 lines each with zero mid-function comments.

---

## ADR-016: Cryptographic Spec Hashing & Role-Based Approval Binding
- **Status**: Accepted
- **Context**: In high-assurance environments, approving a release in an external management portal (e.g. Jira or ServiceNow) introduces a vulnerability window where malicious or accidental changes to the workload spec occur between approval time and the GitOps admission trigger.
- **Decision**: 
  1. Compute a deterministic SHA-256 canonical digest across core spec attributes (`TenantID`, `ServiceID`, `Namespace`, `WorkloadName`, `Image`, `CPURequest`, `CPULimit`, `MemoryRequest`, `MemoryLimit`, `Replicas`, `Strategy`).
  2. In production namespaces, mandate non-empty `ApprovedSpecHash` and `ApproverRole` (`platform-admin`, `secops-lead`, etc.), validating that `ApprovedSpecHash == ComputeWorkloadSpecHash(cmd)`.
  3. Reject any spec modification with HTTP 400 Validation Error.
- **Consequences**: Out-of-band modifications to workload specifications during approval workflows are structurally blocked at the Workload API boundary.

---

## ADR-017: Image Supply Chain Floor & Immutable Digest Mandate
- **Status**: Accepted
- **Context**: Mutable container image tags (e.g., `:latest`, `:v1.0.0`) allow downstream registry poisoning, tag re-push exploits, and non-reproducible deployments. Additionally, deploying images from public, uncurated registries introduces untracked vulnerabilities.
- **Decision**:
  1. Restrict all container images to a verified registry floor (`IsApprovedRegistry`) supporting `gcr.io`, `*.pkg.dev`, `ghcr.io`, `quay.io`, and internal mirror endpoints.
  2. In production namespaces, strictly enforce immutable digest pinning via `@sha256:` and reject mutable tags (`:latest`).
  3. Synthesize explicit container `imagePullPolicy` (`IfNotPresent` for immutable digests; `Always` for mutable tags).
  4. Provide an audited emergency Breakglass protocol (`IMG-BRK-001`) requiring documented justifications stored in the database audit log.
- **Consequences**: Containers cannot be tampered with post-build; production workloads are 100% reproducible and tamper-evident.

---

## ADR-018: Application-Layer KMS Envelope Encryption & Key Custody Separation [V-34]
- **Status**: Accepted
- **Context**: Relying solely on default Kubernetes secret encryption leaves sensitive tenant credentials vulnerable to cluster-admin scraping or compromised etcd storage. Furthermore, storing KMS keys in the same cloud project as the workload violates tenant key custody separation requirements (B-1).
- **Decision**:
  1. Implement application-layer KMS envelope encryption (`AES-256-GCM-KMS-ENVELOPE`) via `src/infra/clients/kms` where data encryption keys (DEKs) are dynamically generated, encrypted under a Cloud KMS key, and stored alongside AES-256-GCM ciphertexts.
  2. Enforce strict key custody admission rules (`ValidateKMSKeyCustody`): KMS keys must reside in a dedicated security project rather than the tenant workload project (`projects/<tenant>/` is rejected).
  3. Populate KMS annotations (`kms.platform.idp.io/key-uri`, `kms.platform.idp.io/algorithm`) on synthesized Kubernetes Secret manifests.
- **Consequences**: Cluster administrators and compromised cluster nodes cannot access raw secret payloads without separate IAM permissions on the dedicated KMS key project.

---

## ADR-019: Admission Guard Prohibiting subPath Mounts for Rotating Secret Material [V-15]
- **Status**: Accepted
- **Context**: When Kubernetes mounts a Secret or ConfigMap into a container volume using `subPath`, Linux bind mounts the file directly. As a consequence, atomic updates to the Secret symbolic link do not propagate to the container, leaving workloads permanently running on stale credentials following rotation.
- **Decision**:
  1. Add an admission validation guard in `rules.ValidateSecretVolumeMounts`.
  2. Reject any workload mounting secrets with a non-empty `SubPath` field, emitting violation `SEC-SUBPATH-001`.
  3. Mandate mounting secret volumes at directory level to guarantee atomic propagation of rotated credentials.
- **Consequences**: Prevents credentials from becoming silently frozen after rotation; guarantees all secret consumers receive atomic credential updates without requiring pod restarts.

---

## ADR-020: Automated Cert-Manager Certificate Lifecycle & Expiry Alerting [V-17, V-18]
- **Status**: Accepted
- **Context**: Public and mesh ingress certificates are subject to shrinking maximum validity lifetimes (e.g. 90-day Let's Encrypt certificates). Manual renewal or silent ACME issuer failures cause unexpected production outages.
- **Decision**:
  1. Automatically synthesize `cert-manager.io/v1` `Certificate` manifests in `adapters.buildCertificateYAML` with 90-day duration (`2160h`) and 30-day renewBefore (`720h`) thresholds.
  2. Provide continuous certificate expiration tracking via `rules.EvaluateCertificateExpiry`, categorizing remaining lifetime into `HEALTHY`, `WARNING_EXPIRING_SOON` (<= 30 days), and `CRITICAL_EXPIRED` (<= 0 days).
  3. Wire certificate manifests into the GitOps bundle for automated reconciliation alongside ingress HTTPRoutes.
- **Consequences**: Eliminates unexpected certificate expiration outages; guarantees timely automated renewals 30 days prior to expiry with proactive alert classification.

