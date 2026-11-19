# End-User Developer Usage Guide: Kubernetes Workload Provisioning & Canary Delivery

This guide provides platform engineers, developers, and operators with end-to-end instructions for provisioning container workloads, operating progressive canary deployments, inspecting declarative infrastructure artifacts, and managing security policy exceptions.

---

## 1. System Architecture & Consumption Model

The platform operates on a **Single-Writer GitOps Boundary**:
1. **Developer / CI Pipeline**: Submits desired workload specifications to the **Workload Provisioning API** (`POST /api/v1/workloads`).
2. **Workload Provisioning Engine**:
   - Executes in-process **CEL Admission Policy Engine** verification (<1ms).
   - Validates namespace **ResourceQuota headroom**.
   - Orchestrates the **Provisioning DAG** to synthesize declarative Kubernetes manifests (`Rollout`, `NetworkPolicy`, `PDB`), Cloud Service Mesh routing (`VirtualService`, `DestinationRule`), and Google Config Connector cloud resources (`IAMPolicyMember`, `SQLDatabase`).
3. **GitOps Committer**: Calculates deterministic Merkle tree roots, cryptographically signs commits, and pushes them to Git.
4. **Argo CD**: Reconciles signed Git manifests onto target clusters. Argo Rollouts manages dynamic in-cluster traffic shifting.

---

## 2. Provisioning a Workload (REST API Workflow)

### 2.1 Inbound Request Format & Headers

All requests to the Workload API must include standard multi-tenant and distributed tracing headers:

| Header | Type | Description | Example |
|---|---|---|---|
| `Authorization` | String | Bearer JWT with tenant role claims | `Bearer eyJhbGci...` |
| `X-Tenant-ID` | UUID | Tenant boundary identifier | `3fa85f64-5717-4562-b3fc-2c963f66afa6` |
| `traceparent` | W3C String | Distributed tracing context (W3C standard) | `00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01` |
| `Content-Type` | String | Payload MIME type | `application/json` |

### 2.2 Concrete Request Payload (`POST /api/v1/workloads`)

```bash
curl -X POST https://platform.internal.company.com/api/v1/workloads \
  -H "Authorization: Bearer $PLATFORM_TOKEN" \
  -H "X-Tenant-ID: 3fa85f64-5717-4562-b3fc-2c963f66afa6" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "wkld-payments-checkout-01",
    "tenantId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "serviceId": "svc-payments-api",
    "namespace": "payments-production",
    "workloadName": "checkout-service",
    "image": "gcr.io/corp-prod/payments/checkout@sha256:4f355bd3a39e728ec65147be15e1fa6dc9e20dd4a218f02934ffea210087799e",
    "cpuRequest": "500m",
    "cpuLimit": "2000m",
    "memoryRequest": "1Gi",
    "memoryLimit": "4Gi",
    "replicas": 4,
    "strategy": "Canary",
    "idempotencyKey": "release-2026-v2.4.1"
  }'
```

### 2.3 Response Structure (`HTTP 201 Created`)

```json
{
  "provision": {
    "id": "wkld-payments-checkout-01",
    "tenantId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "serviceId": "svc-payments-api",
    "namespace": "payments-production",
    "workloadName": "checkout-service",
    "cpuRequest": "500m",
    "cpuLimit": "2000m",
    "memoryRequest": "1Gi",
    "memoryLimit": "4Gi",
    "replicas": 4,
    "strategy": "Canary",
    "status": "OBSERVED_READY",
    "idempotencyKey": "release-2026-v2.4.1",
    "version": 2,
    "createdAt": "2026-11-19T10:00:00Z",
    "updatedAt": "2026-11-19T10:00:05Z"
  },
  "injectedSecurityContext": true,
  "injectedProbes": true,
  "pdbCreated": true,
  "admission": {
    "admitted": true,
    "violations": null,
    "exceptionApplied": false
  },
  "manifests": {
    "namespaceYaml": "apiVersion: v1\nkind: Namespace\n...",
    "rolloutYaml": "apiVersion: argoproj.io/v1alpha1\nkind: Rollout\n...",
    "analysisTemplateYaml": "apiVersion: argoproj.io/v1alpha1\nkind: AnalysisTemplate\n...",
    "virtualServiceYaml": "apiVersion: networking.istio.io/v1alpha3\nkind: VirtualService\n...",
    "destinationRuleYaml": "apiVersion: networking.istio.io/v1alpha3\nkind: DestinationRule\n...",
    "workloadIdentityYaml": "apiVersion: iam.cnrm.cloud.google.com/v1beta1\nkind: IAMPolicyMember\n...",
    "databaseYaml": "apiVersion: sql.cnrm.cloud.google.com/v1beta1\nkind: SQLDatabase\n..."
  },
  "message": "workload provisioned and admitted successfully"
}
```

---

## 3. What Gets Generated: Manifest Bundle Breakdown

When you provision a workload, the engine synthesizes a self-contained multi-tier configuration bundle:

### 3.1 Argo Rollout (`RolloutYAML`)
- Replaces vanilla Kubernetes `Deployment`.
- Implements a **4-stage progressive canary schedule**:
  - `Step 1`: Shift 10% traffic to Canary pod. Pause for 5 minutes.
  - `Step 2`: Shift 25% traffic. Pause for 10 minutes.
  - `Step 3`: Shift 50% traffic. Pause for 15 minutes.
  - `Step 4`: Promote to 100% traffic.
- References companion `AnalysisTemplate` for automated health gates.

### 3.2 Cloud Service Mesh Routing (`VirtualServiceYAML` & `DestinationRuleYAML`)
- **Single-Field Ownership**: Argo Rollouts is the sole field owner of `spec.http[0].route` weights in the `VirtualService`.
- **mTLS STRICT**: The generated `DestinationRule` strictly enforces `trafficPolicy.tls.mode: "ISTIO_MUTUAL"` to guarantee non-plaintext traffic per CIS benchmarks.

### 3.3 Google Config Connector Resources (`WorkloadIdentityYAML` & `DatabaseYAML`)
- **Workload Identity**: Generates declarative `IAMPolicyMember` binding the Kubernetes Service Account (`KSA`) to a dedicated Google Service Account (`GSA`) under the project, granting least-privilege cloud roles (e.g. `roles/cloudsql.client`).
- **Cloud SQL**: Generates `SQLDatabase` with `deletionProtection: true` and IAM database authentication enabled (`CLOUD_IAM_SERVICE_ACCOUNT`).

### 3.4 GKE Gateway API & Ingress Routing (`HTTPRouteYAML`)
- **Multi-Tenant Edge Attachment**: Workload routes attach to the central platform gateway (`gateway-system/gke-external-gateway`) using `parentRefs`.
- **Path Routing & Matchers**: Routes default to `PathPrefix: /` forwarding traffic to the application `Service` on port `8080` with 100% initial weight.
- **Dedicated Hostnames**: Generates tenant-isolated hostnames formatted as `<workload>.<tenant>.example.com`.

### 3.5 Cloud Armor WAF & Rate Limiting (`BackendPolicyYAML`)
- **GCPBackendPolicy Attachment**: Binds the workload backend `Service` directly to the enterprise Cloud Armor security policy (`platform-edge-security-policy`).
- **Resilience Controls**: Enforces backend timeouts (`30s`), graceful connection draining (`60s`), and full telemetry access logging.
- **Layer-7 Filtering**: Workloads inherit DDoS mitigation, client IP rate limiting (1000 req/min threshold), and OWASP Core Rule Set evaluations at the edge.

### 3.6 Argo CD GitOps Ignore Rules
To prevent Argo CD from detecting canary traffic weight changes as out-of-band Git drift and force-reverting them back to 0% [V-12], the following ignore rules are automatically generated:
```json
[
  {
    "group": "networking.istio.io",
    "kind": "VirtualService",
    "jsonPointers": ["/spec/http/0/route"]
  },
  {
    "group": "argoproj.io",
    "kind": "Rollout",
    "jsonPointers": ["/status"]
  }
]
```

---

## 4. Operating & Observing Canary Deployments

Developers and operators monitor rollout progress using the native Argo Rollouts kubectl plugin.

### 4.1 Live Visual Rollout Inspection

```bash
# Watch the rollout progression in real time:
kubectl argo rollouts get rollout checkout-service -n payments-production --watch
```

**Terminal ASCII Dashboard Output:**
```
Name:            checkout-service
Namespace:       payments-production
Status:          ॥ Paused
Strategy:        Canary
  Step:          1/4
  SetWeight:     10%
  ActualWeight:  10%
Images:          gcr.io/corp-prod/payments/checkout:v2.4.0 (stable)
                 gcr.io/corp-prod/payments/checkout:v2.4.1 (canary)
Replicas:
  Desired:       4
  Current:       4
  Updated:       1
  Stable:        3
AnalysisRuns:
  ✔ checkout-service-metrics-gate-1  Pass (error_rate=0.0001, latency_p99=142ms)
```

### 4.2 Manual Promotion & Abort Controls

If human validation or emergency intervention is required:

```bash
# Manually promote to the next canary step (skipping the remaining pause time):
kubectl argo rollouts promote checkout-service -n payments-production

# Instantly abort the rollout (reverts 100% traffic back to stable pods immediately):
kubectl argo rollouts abort checkout-service -n payments-production

# Retry an aborted or failed rollout after addressing an issue:
kubectl argo rollouts retry rollout checkout-service -n payments-production
```

### 4.3 Fail-Closed Metric Analysis Gates

The generated `AnalysisTemplate` checks two live Prometheus telemetry streams every 30 seconds:
1. **Canary Error Rate Delta**: `(canary_5xx_rate / canary_total_rate) - (stable_5xx_rate / stable_total_rate) <= 0.001` (0.1%).
2. **Canary P99 Latency**: `histogram_quantile(0.99, ...) <= 200ms`.

**Fail-Closed Behavior**:
- If either metric breaches the threshold, or if Prometheus is unreachable (`consecutiveErrorLimit: 1`), Argo Rollouts **instantly aborts the canary**, restores 100% traffic to stable pods, and marks the deployment as failed.

---

## 5. Security Policy Exception Lifecycle

All workloads must satisfy the 6 baseline government constraints (Restricted Pod Security, Cloud Service Mesh injection, immutable SHA256 image digest, mandatory labels, root user ban, and dropped capabilities).

If an existing stateful or legacy daemon requires temporary exemptions (e.g. running with elevated capabilities during a migration), developers must follow the **Policy Exception Lifecycle**.

### 5.1 Requesting a Policy Exception

1. Create a structured exception record in the `policy-as-code` repository or submit an exception request:
   - Must specify: `tenant_id`, `workload_id`, `namespace`, `policy_id`, `justification` (>= 10 chars), and `expires_at`.
2. **Security Review & Approval**:
   - SecOps team validates justification and approves the exception in Postgres (`status = 'ACTIVE'`).
3. **Admission Bypass Evaluation**:
   - When the Workload API evaluates admission for `checkout-service`:
   - It queries `policyRepo.GetActiveExceptionsForWorkload(ctx, tenantID, namespace, workloadName)`.
   - If an unexpired, active exception is found, the admission violation is waived.
   - The response includes:
     ```json
     "admission": {
       "admitted": true,
       "violations": ["Bans privileged containers across workloads"],
       "exceptionApplied": true,
       "exceptionId": "EXC-LEGACY-ROOT-001",
       "approverId": "secops-lead@company.internal"
     }
     ```

---

## 6. Common Errors & Troubleshooting Runbooks

### Error 1: `HTTP 409 CONFLICT: insufficient quota headroom`
- **Cause**: Requested CPU or Memory exceeds the namespace `ResourceQuota`.
- **Resolution**: Inspect current namespace usage with `kubectl get resourcequota -n <namespace>`. Either downscale unused workloads or submit a quota headroom increase request.

### Error 2: `HTTP 400 VALIDATION_FAILED: admission violation`
- **Cause**: The container image is using a floating tag (e.g. `:latest`) or attempting to run as root (`runAsUser: 0`).
- **Resolution**: Use immutable SHA256 digests (`image@sha256:...`) and configure non-root execution (`runAsUser: 10001`). If root execution is temporarily unavoidable, request a formal policy exception.

### Error 3: Rollout Aborted during Canary (`Degraded`)
- **Cause**: Metric analysis detected a 5xx error rate spike (> 0.1%) or p99 latency exceeding 200ms.
- **Resolution**: Check pod logs for unhandled panics or downstream errors:
  ```bash
  kubectl logs -l app.kubernetes.io/name=checkout-service -c checkout-service -n payments-production --tail=100
  ```
  Once resolved, publish a new immutable image digest and re-submit the provisioning request.
