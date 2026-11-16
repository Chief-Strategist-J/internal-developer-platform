# Failure Modes & Mitigation: Kubernetes Workload Provisioning

This document outlines the failure modes, severity levels, detection mechanisms, and automated mitigation runbooks for the Workload Provisioning engine and its platform runtime chassis.

---

## 1. Failure Modes Matrix

| Failure Scenario | Severity | Impact | Detection Mechanism | Automated Mitigation / Handling |
|---|---|---|---|---|
| **Silent Quota Rejection** | OUTAGE | K8s accepts Deployment, but pods remain in `Pending` due to ResourceQuota exhaustion. | Prometheus metric: `kube_pod_status_phase{phase="Pending"}` > 2m. | Workload API queries namespace quota headroom before apply; emits actionable quota increase request. |
| **Non-Baseline Pod Reaches Prod** | SECURITY | Workload running as root or with elevated capabilities. | Binary Authorization continuous validation violation logs + Falco runtime alert. | Validating admission webhook fails closed; pod terminated immediately. |
| **KMS Key Loss or Disablement** | LOSS / OUTAGE | Kubernetes Secrets encrypted with CMEK become unreadable; pods crash on restart. | Cloud KMS audit logs alert on key state disablement / deletion. | CMEK key deletion protection enabled; key custody isolated in dedicated security project. |
| **Argo CD Sync Conflict with HPA** | DEGRADE | Argo CD sync overwrites HPA-managed replica count, causing thrashing. | Argo CD health state `OutOfSync`; event frequency alert. | `ignoreDifferences` configured on `.spec.replicas` for HPA-managed Deployments. |
| **Stuck Namespace Deletion** | OUTAGE | Decommission blocked by lingering finalizers (e.g. cloud load balancer). | Namespace stuck in `Terminating` > 5m alert. | Automated finalizer sweep script releases external cloud dependencies in strict reverse order. |
| **Public TLS Certificate Expiration** | OUTAGE | Ingress connections rejected by client browsers. | cert-manager alert: `certmanager_certificate_expiration_timestamp_seconds` < 14 days. | Dual-issuer automated renewal via Let's Encrypt / Google Managed Certificates. |
| **Telemetry Collector Unreachable** | DEGRADE | OpenTelemetry Collector endpoint (port 4318) down; traces cannot be exported over network. | OTel SDK export drop counter `otelcol_exporter_enqueue_failed_spans` > 0. | Non-blocking in-memory ring buffer drops oldest spans; fallback structured logging continues to stdout with zero latency impact on user requests. |
| **Database Pool Starvation Under Burst** | OUTAGE | High-concurrency traffic exhausts `max_open_conns` (25); query latencies spike. | Metric: `go_sql_db_wait_count` increasing rapidly; connection acquire timeout > 2s. | Chassis sheds load via HTTP 429 Too Many Requests; read requests fall back to memory caching adapters. |
| **Downstream Peer Startup Coupling Deadlock** | DEGRADE | Kubernetes API or Argo CD server unreachable during platform service startup. | Chassis startup log: `database ping failed; soft dependency fallback active`. | Downstream dependencies marked `CriticalitySoft`; service boots successfully in read-only degraded mode and serving `/healthz`. |
| **Chassis Graceful Drain Timeout** | DEGRADE | Lingering in-flight HTTP connections prevent clean server shutdown within `shutdown_deadline_seconds` (15s). | Chassis shutdown log: `public server graceful drain failed`. | Context deadline triggers forceful socket closure; writes exit code `EXIT_TIMEOUT` (6) and dumps thread trace. |

---

## 2. Telemetry & Observability Failure Runbook

When trace collection or structured logging indicates degraded state:
1. **Loki / Tempo Log Drill-Down**: Use the `X-Trace-Id` returned in HTTP response headers to locate the corresponding trace in Grafana Tempo (`trace_id: <32hex>`).
2. **Span Status Verification**: Verify whether the span status is marked `ERROR` and inspect the `error.type`, `error.message`, and `http.status_code` span attributes.
3. **Collector Reconnection**: If the OpenTelemetry collector container restarts, the Go runtime automatically reconnects over HTTP/gRPC without process restarts.

---

## 3. Emergency Breakglass Runbook

When the automated GitOps pipeline or Argo CD itself is experiencing an outage and a hotfix must be deployed to mitigate an active production incident:
1. SRE requests JIT emergency role credentials with automatic 1-hour expiration.
2. Breakglass deploy applied directly via `kubectl apply -f manifest-bundle.yaml`.
3. Cloud Logging Data Access audit logs capture the breakglass event with `X-Request-ID`.
4. Automated post-incident reconciliation forces Git repository alignment to prevent drift once Argo CD recovers.
