# Failure Modes & Mitigation: Policy as Code & Governance Engine

This document outlines the failure modes, severity classes, detection mechanisms, and automated mitigation runbooks for the Policy as Code engine, webhook admission boundaries, continuous audit scanners, and relational governance ledgers.

---

## 1. Failure Modes Matrix (F-01 through F-09)

| Failure Code | Failure Scenario | Severity | Root Cause | Detection Mechanism | Automated Mitigation & Operational Runbook |
|---|---|---|---|---|---|
| **F-01** | **Admission Webhook Outage** | SEV-1 (OUTAGE) | Policy Controller / Kyverno pods crash or become unreachable while `failurePolicy: Fail` is enforced. All cluster writes are blocked. | Metric: `apiserver_admission_webhook_admission_duration_seconds` spike; Alert: `AL-02` (PAGE). | **RB-02**: HA deployment across 3 failure domains, PodDisruptionBudgets (`minAvailable: 2`), and mandatory namespace exclusions (`kube-system`, `gatekeeper-system`). Emergency breakglass allows two-person authenticated webhook deregistration while in-process PSA/CEL gates continue blocking critical violations. |
| **F-02** | **Webhook Left at Fail-Open (`Ignore`)** | SEV-2 (SECURITY / COMPLIANCE) | Cluster operator or configuration drift toggles `failurePolicy` from `Fail` to `Ignore`, allowing unvetted workloads during network latency. | Posture scan audit; Alert: `AL-03` (PAGE); Admission telemetry flag. | **RB-03**: Cluster READY gate rejects any cluster with `failurePolicy: Ignore`. Argo CD GitOps reconciler automatically reverts configuration drift. Audit scanner queries all workloads admitted during the drift window for non-compliance. |
| **F-03** | **Premature Rule Promotion (`DENY` False Positives)** | SEV-2 (OUTAGE) | A new or tightened rule is promoted to `DENY` without passing through `AUDIT` and `WARN` soak cycles, breaking production deployments. | Metric: Deny count spike post-sync; Alert: `AL-04` (PAGE). | **RB-04**: Strict relational FSM guard in `PolicyAsCodeService` blocks transition from `WARN` to `DENY` if active violations > 0 in the target ring. Automated canary rollback reverts rule state to `WARN` upon false positive detection ($SLI-7$ breach). |
| **F-04** | **Policy Repository Compromise** | SEV-1 (SECURITY) | Unauthorized or malicious commit pushed to policy repository loosening admission constraints. | Signature verification failure; Alert: `AL-09` (PAGE); Git commit signature mismatch. | **RB-08**: Argo CD refuses synchronization of unsigned commits or unknown GPG signers. CI and admission gates require two-person security review. Automated freeze blocks webhook updates until branch integrity is verified. |
| **F-05** | **Missing Detection Pair** | SEV-3 (COMPLIANCE) | An enforcing prevention rule is introduced without a matching background continuous audit scan or validation check. | Invariant `P0-04` CI verification gate; Alert: Compliance gap event. | CI rejects PR immediately. Periodic evidence sampling detects unmapped prevention rules and prevents rule graduation past `DRAFT`. |
| **F-06** | **Cloud Org Policy Hierarchy Override** | SEV-1 (SECURITY) | Organization Policy guardrails (e.g. private cluster endpoints, shield nodes) are removed or inherited exceptions granted higher in GCP hierarchy. | Continuous Validation (CV) posture scan; Alert: `AL-10` (TICKET). | Posture scanner flags hierarchy mismatch. In-process admission CEL checks maintain localized container constraints regardless of cloud boundary status. |
| **F-07** | **Continuous Audit Scanner Stalled** | SEV-3 (COMPLIANCE) | Background scanner crashes or becomes CPU-starved; violation table becomes stale past SLA. | Metric: `gatekeeper_audit_last_run_end_time` staleness > 4h; Alert: `AL-05` (TICKET). | **RB-05**: Audit scanner health probe restarts stalled scan pod. Platform dashboard flags compliance status as degraded. Deny promotions blocked across all rings until convergence is restored. |
| **F-08** | **Compliance Evidence Sink Broken** | SEV-1 (COMPLIANCE) | Audit event pipeline to locked storage (WORM bucket) fails due to IAM permissions, quota, or network partitions. | Log export drop counter; Alert: `AL-13` (PAGE). | **RB-12**: Platform buffers audit scan reports in PostgreSQL `policy_violations` relational tables. Exponential backoff retry against secondary log bucket. Compliance officers alerted to quantify evidence capture window. |
| **F-09** | **Active Expired Exception** | SEV-2 (SECURITY) | An approved exception passes its `expires_at` timestamp but continues to admit non-compliant workloads. | Reconciliation sweep; Alert: `AL-12` (TICKET). | **RB-11**: Domain rules evaluate expiration deterministically at runtime (`now > ExpiresAt`). Automated background reaper sweeps PostgreSQL and updates status to `EXPIRED`, immediately revoking admission exemption. |

---

## 2. Edge Case Handling & Resolution Matrix (E-01 through E-15)

- **E-01: Deployment Accepted, Pod Denied by PSA**: Workload controller creates Deployment successfully, but child Pods fail Pod Security Standards validation (`ReplicaFailure`).
  - *Resolution*: Shift-left CI evaluates rendered pod templates against PSA rules before merge, preventing silent replica failures.
- **E-02: Existing Running Pod Violates Tightened Rule**: A rule is promoted from `AUDIT` to `WARN` while existing pods in production violate it.
  - *Resolution*: Invariant `G-5`: Tightening does not evict running pods; violation is recorded in `policy_violations`, ticket dispatched to owner, and promotion to `DENY` is locked by the FSM guard until running count reaches zero.
- **E-03: Tenant Self-Assigns Exemption Label**: Tenant attempts to bypass admission by adding `policy.internal/exempt=true` to pod labels.
  - *Resolution*: Invariant `P0-03` & rule `TEN-06`: In-process CEL and label-protection webhooks reject tenant labels. Exemptions are strictly loaded from the relational PostgreSQL registry via signed GitOps PRs.
- **E-04: Emergency Breakglass Image Deploy**: Sev-1 production incident requires deploying an un-attested hotfix container image.
  - *Resolution*: Binary Authorization breakglass flag logged; admitted workload emits continuous validation alert `AL-08` requiring mandatory retrospective security review within 24 hours.
- **E-05: Concurrent Mutation Collision**: Mutating webhook and in-process CEL mutation both modify pod security context.
  - *Resolution*: Strict execution ordering: CEL mutating runs first (in-process), followed by webhook mutation with idempotency validation.
- **E-06: Recovery Namespace Interception Lockout**: Webhook tries to validate resources inside `gatekeeper-system` or `kube-system`.
  - *Resolution*: Invariant `P0-02`: Anti-lockout exclusion hardcodes recovery namespaces out of webhook scope entirely.
- **E-07: Cloud Org Policy Propagation Delay**: GKE cluster created before new Org Policy reaches eventual consistency.
  - *Resolution*: Pre-flight readiness gate executes automated posture scan before admitting tenant traffic.
- **E-08: Pre-1.36 Kubernetes Cluster Without CEL Mutation**: Older clusters cannot run in-process CEL mutations.
  - *Resolution*: Platform chassis activates fallback mutating webhook adapter with explicit metric tagging.
- **E-09: Referential Sync Memory Exhaustion**: Policy Controller runs out of memory tracking large numbers of ingress hostnames or GVK kinds.
  - *Resolution*: Referential rules restricted to non-critical severity classes; watched GVK sets strictly throttled and bounded.
- **E-10: Policy Bundle Upgrade Introduces New Constraints**: Upstream NIST/CIS benchmark bundle update adds unexpected checks.
  - *Resolution*: Upgrades treated as Class C1 changes; new constraints forced to start in `AUDIT` state regardless of upstream defaults.
- **E-11: CI / Admission Version Divergence**: CI runs a newer policy ruleset than the cluster admission controller.
  - *Resolution*: CI and cluster engines pin the identical policy repository Git commit SHA; version mismatch fails CI pipeline.
- **E-12: Emergency Fix Blocked by New Rule**: Incident patch blocked by recently tightened non-critical policy.
  - *Resolution*: Standard expedited exception path with 24-hour maximum lease and incident commander sign-off.
- **E-13: Webhook Latency Under Deploy Storm**: Bulk deployment waves cause webhook latency to breach API server timeout (10s).
  - *Resolution*: Webhook horizontal autoscaling (HPA) targets 50% CPU/memory; heavy expression logic shifted into in-process CEL.
- **E-14: CEL Evaluation On Missing Optional Fields**: Manifest omitting optional spec fields causes CEL expression nil-pointer evaluation errors.
  - *Resolution*: Domain rule `ValidateCELRuleExpression` verifies delimiter balance and defensive field navigation (`has(x.spec.y)`).
- **E-15: Policy Controller Licensing Block**: GKE Enterprise licensing unavailable in self-hosted or standard environments.
  - *Resolution*: Standardized adapter abstraction allows seamless swap to Kyverno without altering the underlying relational governance ledger or API contracts.

---

## 3. Anti-Lockout Breakglass Runbook (RB-02)

When admission webhook pods are degraded and cluster operations are frozen:
1. **Identification**: Alert `AL-02` pages Platform SRE. API server latency spikes on mutating/validating calls.
2. **Exclusion Verification**: Confirm whether the outage affects control plane components. Verify that `kube-system` and GitOps namespaces remain operational.
3. **Emergency Webhook Deregistration**:
   - Two authorized platform engineers authenticate via MFA.
   - Delete failing `ValidatingWebhookConfiguration` object directly:
     ```bash
     kubectl delete validatingwebhookconfiguration internal-policy-webhook --ignore-not-found
     ```
4. **Baseline Containment**: In-process PSA restricted enforcement and CEL validating policies remain active in the apiserver, ensuring core security boundaries are not compromised.
5. **Restoration & Audit**:
   - Re-deploy Policy Controller pods in isolated staging ring.
   - Re-enable webhook configuration via Argo CD sync.
   - Execute continuous audit scan (`/api/v1/policies/audit-scan`) to verify all resources deployed during the bypass window.
   - Log breakglass window duration and affected resources for auditor evidence trace.
