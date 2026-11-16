# Failure Modes & Mitigation: Kubernetes Workload Provisioning

This document outlines the failure modes, severity levels, detection mechanisms, and automated mitigation runbooks for the Workload Provisioning engine.

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

---

## 2. Emergency Breakglass Runbook

When the automated GitOps pipeline or Argo CD itself is experiencing an outage and a hotfix must be deployed to mitigate an active production incident:
1. SRE requests JIT emergency role credentials with automatic 1-hour expiration.
2. Breakglass deploy applied directly via `kubectl apply -f manifest-bundle.yaml`.
3. Cloud Logging Data Access audit logs capture the breakglass event with `X-Request-ID`.
4. Automated post-incident reconciliation forces Git repository alignment to prevent drift once Argo CD recovers.
