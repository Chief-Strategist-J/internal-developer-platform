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

---

## ADR-021: Cloud Service Mesh Managed Control Plane & PeerAuthentication STRICT Enforcement [V-30]
- **Status**: Accepted
- **Context**: Government zero-trust security architecture mandates mutual TLS (mTLS) for all inter-service communications in production namespaces. Relying on default mesh settings leaves traffic in `PERMISSIVE` mode, allowing unauthenticated and unencrypted plaintext communication.
- **Decision**:
  1. Mandate mTLS `STRICT` mode for all production namespaces in `rules.ValidateMeshSecurity` (`MESH-MTLS-001`). Reject any `PERMISSIVE` or `DISABLE` configuration at admission time.
  2. Synthesize declarative `security.istio.io/v1beta1` `PeerAuthentication` manifests in `adapters.buildMeshManifests` targeting workload labels with `spec.mtls.mode: STRICT`.
  3. Include `PeerAuthentication` manifests in the GitOps delivery bundle reconciled by Argo CD.
- **Consequences**: Guarantees zero plaintext inter-service traffic in production; non-mTLS connections are rejected at the Envoy proxy layer.

---

## ADR-022: Admission Prohibition of In-Cluster OSS Istio and Unsupported EnvoyFilter/WasmPlugin CRDs [V-30]
- **Status**: Accepted
- **Context**: Managed Google Cloud Service Mesh does not support in-cluster `istiod` control planes and explicitly does not support custom `EnvoyFilter` or `WasmPlugin` CRDs. Applying these resources breaks automated mesh upgrades, corrupts proxy configurations, and causes cluster-wide network outages.
- **Decision**:
  1. Add admission guard `rules.ValidateMeshCRDCompatibility` checking tenant workload resource lists.
  2. Reject any workload manifest attempting to declare or reference `EnvoyFilter`, `WasmPlugin`, `istiod`, or `in-cluster-istio` with violation `MESH-OSS-001`.
- **Consequences**: Protects managed mesh stability; prevents tenant teams from deploying unmaintainable or incompatible Layer-7 proxy filters that break fleet upgrade rings.

---

## ADR-023: Tenant Egress Perimeter Control via Declarative NetworkPolicy & ServiceEntry Boundaries
- **Status**: Accepted
- **Context**: Workload namespaces enforce default-deny egress (allowing only DNS on port 53). Workloads requiring external third-party API access (e.g. Stripe, AWS S3, OAuth providers) fail connection attempts unless explicit egress permissions are provisioned. However, wildcard egress violates zero-trust perimeters.
- **Decision**:
  1. Introduce structured `contracts.EgressRule` (`Host`, `Port`, `Protocol`) in `WorkloadProvisionCommand`.
  2. Synthesize declarative `networking.istio.io/v1beta1` `ServiceEntry` resources (`MESH_EXTERNAL`, DNS resolution) for allowlisted third-party hostnames and ports.
  3. Commit `ServiceEntry` manifests as part of the workload GitOps delivery bundle.
- **Consequences**: Workloads can securely access designated external endpoints with full DNS resolution while maintaining strict perimeter isolation and preventing unauthorized data exfiltration.

---

## ADR-024: Mandatory Data Access Audit Logging and Dedicated Locked-Retention Log Sinks [V-32]
- **Status**: Accepted
- **Context**: In Google Cloud, Data Access audit logs are disabled by default. If left in the `_Default` log bucket, audit entries are pruned after 30 days. For government workloads requiring multi-year compliance retention (B-1), 30-day retention violates compliance and destroys forensics evidence.
- **Decision**:
  1. Mandate `enableDataAccessLogs: true` for all production namespaces in `rules.ValidateAuditLogging` (`AUD-DATA-001`).
  2. Reject audit configurations targeting `_Default` or empty buckets (`AUD-SINK-001`). Sinks must route to dedicated user-defined storage buckets (`LoggingLogSink`).
  3. Enforce a minimum retention period of 365 days with immutable bucket locking (`isLocked: true`, `AUD-LOCK-001`) to prevent administrative deletion or tampering.
- **Consequences**: Audit logs are immutably preserved for compliance regimes; retention is legally locked and protected from administrative tampering.

---

## ADR-025: Continuous Off-Node Log Forwarding to Preempt Pod Eviction Log Loss [V-23]
- **Context**: Node resource exhaustion, GKE auto-repair, and spot preemptions cause pod evictions. Upon eviction, local node-level container logs are pruned; `kubectl logs` only displays output from the newest container instance, wiping historical crash diagnostics.
- **Decision**:
  1. Add admission guard requiring `offNodeLogCollection: true` for all production workloads (`AUD-COLL-001`).
  2. Synthesize Google Config Connector `LoggingLogSink` capturing `resource.type="k8s_container"` continuously off-node into Cloud Storage.
- **Consequences**: Full stdout/stderr streams are preserved in immutable storage even if worker nodes are destroyed or evicted during critical cluster incidents.

---

## ADR-026: Pre-Storage PII Scrubbing and CMEK-Encrypted Audit Sinks
- **Status**: Accepted
- **Context**: Government data protection regulations mandate that Personally Identifiable Information (PII) such as national IDs, payment tokens, and authentication credentials must not be stored in plain text in centralized audit buckets. Furthermore, audit buckets must be encrypted using customer-managed encryption keys (CMEK).
- **Decision**:
  1. Enforce `piiScrubbingEnabled: true` at admission time for production workloads (`AUD-COLL-001`).
  2. Synthesize declarative Google Config Connector `StorageBucket` resources configured with `encryption.defaultKmsKeyName` referencing dedicated security CMEK keys.
- **Consequences**: Audit logs are protected by customer-held cryptographic keys and scrubbed of sensitive user data prior to long-term compliance archival.

---

## ADR-027: Just-In-Time (JIT) Access Grants and Hard Session Duration Ceilings
- **Status**: Accepted
- **Context**: Standing privileged access in production environments presents extreme security exposure and violates compliance regimes (B-1). Developers and operators require temporary access for active incident debugging, but naked Kubernetes RoleBindings lack native expiration or revocation mechanics.
- **Decision**:
  1. Enforce a zero standing production access baseline.
  2. Require all production access via Just-In-Time (JIT) `access_grants` with mandatory business reason and approved role authorization (`PlatformAdmin` or `SecurityLead`).
  3. Enforce a hard security ceiling of 4 hours (14,400 seconds) on any JIT session (`JIT-DUR-001`).
  4. Automatically invalidate and revoke expired grants via asynchronous eviction and verification guards.
- **Consequences**: Prevents privilege accumulation; ensures every production access session is time-bounded, justified, and cryptographically revocable.

---

## ADR-028: Absolute Prohibition of `nodes/proxy` and Kubelet Exec Privileges [V-25]
- **Status**: Accepted
- **Context**: In Kubernetes, granting `nodes/proxy` or `nodes/exec` permissions allows callers to interact directly with the Kubelet API on worker nodes, bypassing Pod Security Standards, admission webhooks, network policies, and audit logging sinks.
- **Decision**:
  1. Add admission guard `ValidateRoleSecurityPosture` prohibiting `nodes/proxy`, `nodes/exec`, and direct kubelet subresource permissions across all human and monitoring roles (`SEC-PROXY-001`).
  2. Enforce database-level check constraint `chk_prohibit_nodes_proxy` on `access_grants` table to guarantee that no JIT grant can request or bind node proxy privileges.
- **Consequences**: Eliminates node-level privilege escalation pathways; guarantees all cluster interactions remain constrained within tenant namespace boundaries.

---

## ADR-029: Ephemeral Debug Container Supply Chain and Continuous Auditing [V-24, V-26, V-32]
- **Status**: Accepted
- **Context**: Ephemeral debug containers (`kubectl debug`) injected into running production pods run with elevated debugging tools. If unvetted public images are attached mid-incident, malicious binaries or unverified diagnostic tools could compromise pod memory and secrets.
- **Decision**:
  1. Enforce `ValidateEphemeralDebugContainer`: ephemeral debug images must originate strictly from approved internal container registries (`V-26`).
  2. Require immutable digest references (`@sha256:`) for ephemeral containers in production namespaces.
  3. Emit structured audit events (`V-32`) for every ephemeral container attachment and pod exec session, capturing caller identity and target container PID.
- **Consequences**: Incident debugging remains responsive while ensuring forensic integrity and protecting workload memory from untrusted tooling.

---

## ADR-030: Dynamic Database & Config Platform Regime Governance (B-1 to B-4)
- **Status**: Accepted
- **Context**: Government platform baselines cannot hardcode compliance parameters (audit log retention, data residency, KMS key custody mode, RTO/RPO SLAs, GKE cluster topology, and tenancy isolation) into immutable binaries or single static values. Different jurisdictions (e.g. FedRAMP, DoD IL5, EU GDPR) mandate varying compliance floors that must be dynamically configurable per tenant while strictly preventing relaxation below baseline platform floors.
- **Decision**:
  1. **Dual-Layer Resolution**: Provide declarative platform baseline floors via `config/regime.yaml` with runtime overrides stored in PostgreSQL table `platform_regime_policies`.
  2. **Database Invariants & Immutability**: Enforce 4 reference lookup tables (`regime_policy_statuses`, `kms_custody_modes`, `cluster_modes`, `tenancy_modes`), named constraints (`chk_platform_regime_audit_retention_min_floor >= 30`, positive RTO/RPO), and database triggers (`trg_platform_regime_immutability`) preventing in-place mutations of historical records.
  3. **Admission-Time Verification**: Inject `PlatformRegimeRepositoryPort` into `WorkloadProvisioningService` to dynamically evaluate all incoming workload commands against active tenant regimes. Workloads violating regime floors (e.g. audit retention < floor, unapproved residency regions, missing KMS custody) are rejected immediately at admission.
  4. **OpenAPI & REST Ingress**: Expose dedicated endpoints `POST /api/v1/regimes` and `GET /api/v1/regimes/active` with W3C distributed tracing and role-based access control.
- **Consequences**: Eliminates blockers B-1, B-2, B-3, and B-4 from the provisioning tree; enables multi-regime government multi-tenancy while guaranteeing no tenant can circumvent platform security and compliance baselines.

---

## ADR-031: Cluster API & Cloud Resource Offerings Ingress Architecture
- **Status**: Accepted
- **Context**: The Kubernetes Workload Provisioning Business Tree (v4, Section 3) specifies that the platform must expose three distinct offerings behind dedicated APIs: Cluster API, Cloud Resource API, and Workload API. Previously, only the Workload API had exposed HTTP handlers, leaving Cluster and Cloud Resource provisioning uninvokable via REST.
- **Decision**:
  1. Implement `ClusterHandler` (`POST /api/v1/clusters`) in `src/api/rest/v1/handlers/cluster.handler.go` utilizing `GKEClusterClient` to synthesize declarative, hardened GKE cluster manifests (Shielded Nodes, Private Nodes/Endpoint, Application-Layer KMS Secrets Encryption, Binary Authorization, Backup for GKE).
  2. Implement `CloudResourceHandler` (`POST /api/v1/cloud-resources`) in `src/api/rest/v1/handlers/cloudresource.handler.go` utilizing `CloudResourceClient` to generate declarative Google Config Connector Workload Identity bindings (`IAMServiceAccount` and `IAMPolicyMember`) mapped to Kubernetes Service Accounts.
  3. Register both offering handlers under `OfferingsRouter` in `src/api/rest/v1/routers/offerings.router.go`, protected by RBAC middleware requiring `infrastructure:write` permissions.
  4. Enforce strict typecasting with zero inline comments in handler bodies, returning standardized `types.ApiResponse` envelopes.
- **Consequences**: Fulfills the three-offerings architecture mandated by the business tree, providing auditable and declarative generation of cluster and cloud resources directly through the platform REST API.

---

## ADR-032: Relational Incident Timeline Persistence Engine
- **Status**: Accepted
- **Context**: Workload lifecycle state transitions, canary rollouts, and incident rollbacks produce critical operational events that must be durably recorded for auditability, post-mortem analysis, and SRE triage. Table `incident_timeline_events` was defined in migration `0012_add_oncall_and_incident_fields.sql`, but lacked application repository ports and REST ingress routes.
- **Decision**:
  1. Define `IncidentTimelineEvent` domain entity and extend `WorkloadRepositoryPort` with `RecordTimelineEvent` and `ListTimelineEvents`.
  2. Implement SQL query statements (`FLOW_INSERT_INCIDENT_TIMELINE_EVENT`, `FLOW_LIST_INCIDENT_TIMELINE_EVENTS`) and PostgreSQL repository adapter methods in `src/features/kubernetes-workload-provisioning/infrastructure/adapters/kubernetes-workload-provisioning.postgres.repository.adapter.go`.
  3. Implement in-memory fallback adapter in `kubernetes-workload-provisioning.memory.repository.adapter.go` for isolated unit tests.
  4. Expose REST ingress endpoints `POST /api/v1/workloads/timeline` and `GET /api/v1/workloads/timeline` in `WorkloadProvisioningHandler` and `WorkloadRouter`.
- **Consequences**: Every operational incident, canary failure, or automated rollback is permanently recorded with actor attribution and metadata in PostgreSQL, providing an immutable audit timeline for incident response.






