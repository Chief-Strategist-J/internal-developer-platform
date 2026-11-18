# Architecture Decision Records (ADRs): Policy as Code & Governance Engine

## ADR-001: Defense-in-Depth Layered Enforcement Architecture
- **Status**: Accepted
- **Context**: Government security frameworks (NIST SP 800-53, CIS Benchmarks) mandate that no single point of failure in an admission controller can permit unauthorized or non-compliant workloads to execute. External admission webhooks can fail open or introduce severe latency under load.
- **Decision**: Implement a 5-layer enforcement topology:
  1. `Layer 0 (Cloud)`: GCP Organization Policy custom constraints on Cluster and NodePool creation/updates.
  2. `Layer 1 (Shift-Left CI)`: In-pipeline policy evaluation running against rendered manifests pinned to the identical policy Git SHA.
  3. `Layer 2 (GitOps Boundary)`: Argo CD `AppProject` source repository, destination namespace, and cluster-scoped kind restrictions.
  4. `Layer 3 (In-Process Admission Gate)`: Kubernetes Pod Security Standards (PSS `restricted` profile) and in-process Common Expression Language (`CEL`) validating admission policies.
  5. `Layer 4 (Cluster Webhook Engine)`: Policy Controller (or Kyverno) running in High Availability with `failurePolicy: Fail` as an outer barrier.
  6. `Layer 5 (Continuous Detection)`: Background audit scans and Continuous Validation (CV) paired with each prevention rule.
- **Consequences**: Critical rules cannot be bypassed even if external webhook pods are unreachable or crashing.

---

## ADR-002: In-Process Admission for CRITICAL Policies (Fail-Open Prohibition)
- **Status**: Accepted
- **Context**: Standard Kubernetes admission webhooks default to `failurePolicy: Ignore` (fail-open) to avoid breaking deployments during control-plane outages. Relying on fail-open paths for security controls enables complete compliance bypass.
- **Decision**: Non-negotiable invariant `P0-01`: All `CRITICAL` severity policies (including root execution bans, host namespace isolation, privilege escalation, and binary attestation) MUST be enforced exclusively through in-process mechanisms (`PSA`, in-process `CEL`, `Binary Authorization`, or `Org Policy`). Webhook engines are used only for complex referential constraints, and must be configured with `failurePolicy: Fail`.
- **Consequences**: Ensures cluster security posture cannot be compromised by webhook unavailability, network partitions, or webhook certificate expiry.

---

## ADR-003: Anti-Lockout Infrastructure & Recovery Exclusion Scope
- **Status**: Accepted
- **Context**: When admission controllers enforce `failurePolicy: Fail`, a failure in webhook pods can prevent the scheduler or operator from deploying the very pods needed to fix the problem, creating a circular fatal lockout.
- **Decision**: Invariant `P0-02`: The policy engine MUST exclude critical system and recovery namespaces from webhook interception:
  - `kube-system`
  - `gatekeeper-system` / `kyverno`
  - Service mesh control plane (`istio-system`)
  - GitOps agent (`argocd`)
  Furthermore, webhooks MUST NEVER intercept `Node` or `ValidatingWebhookConfiguration` objects. A two-person audited break-glass procedure (`RB-02`) is established for emergency webhook removal.
- **Consequences**: Recovery paths remain functional during catastrophic policy engine failures; platform operators can restore cluster stability without out-of-band master node access.

---

## ADR-004: Git-Managed Exemption Registry & Label Tamper-Proofing
- **Status**: Accepted
- **Context**: In multi-tenant environments, tenants can attempt to bypass admission policies by attaching arbitrary labels (e.g., `policy.security/exempt=true`) directly to their pod manifests.
- **Decision**: Invariant `P0-03`: Exemptions can NEVER be granted via resource labels. All policy exemptions must be requested via signed Pull Requests in the policy repository, verified against the `exceptable` flag, reviewed by security leads, and rendered into declarative CEL binding parameters or Policy Controller constraints. A dedicated mutating/validating label-protection webhook running with `failurePolicy: Fail` rejects any tenant attempt to self-assign exemption labels.
- **Consequences**: Prevents privilege escalation and policy evasion by untrusted workloads. All exemptions are centrally audited with explicit expiration timestamps.

---

## ADR-005: Staged Policy Rollout Lifecycle & Zero-Downtime Rule Evolution
- **Status**: Accepted
- **Context**: Immediate deployment of new security policies in blocking mode (`DENY`) risks disrupting live production workloads due to unforeseen configuration edge cases and false positives.
- **Decision**: Invariant `P0-05`: Every policy rule must progress through a deterministic, phased lifecycle:
  `DRAFT` → `CI_ONLY` → `AUDIT` → `WARN` → `DENY`
  - `DRAFT`: Local expression testing.
  - `CI_ONLY`: Fails PRs with advisory comments without blocking cluster deployment.
  - `AUDIT`: Violations recorded in metrics and audit logs; zero workload blocking.
  - `WARN`: Admission warning headers returned to developers.
  - `DENY`: Enforced blocking at admission gates.
  Promotion across rings (dev → staging → prod) requires running violations in the ring to be zero and deny precision ($SLI-7$) to meet the error budget.
- **Consequences**: Eliminates unexpected production outages caused by policy tightening; gives application teams explicit soak windows to remediate non-compliant workloads.

---

## ADR-006: Relational Policy & Exemption Persistence with 05-DB Compliance
- **Status**: Accepted
- **Context**: Storing policies solely as flat YAML files lacks centralized queryability, historical auditability, and relational integrity with workload metadata. Unstructured tables lead to data anomalies and cross-tenant leaks.
- **Decision**: Implement PostgreSQL persistence adhering strictly to `05-db-writing-rules.md`:
  1. Five mandatory identifier roles per entity: `id` (UUIDv7 PK), `public_id` (prefixed string `pol_...`, `pex_...`), `tenant_id` (partition/isolation key), `source` + `external_id` (natural key), and `correlation_id` (causation trace).
  2. Lifecycle states enforced via reference lookup tables (`policy_lifecycle_states`, `policy_exception_status`) and a relational transition matrix (`policy_state_transitions`).
  3. Strict composite foreign keys (`FOREIGN KEY (tenant_id, policy_id) REFERENCES policies (tenant_id, id) ON DELETE RESTRICT`).
  4. Optimistic locking via `version BIGINT NOT NULL DEFAULT 1` and trigger-maintained `updated_at`.
- **Consequences**: Guarantees zero cross-tenant policy leakage, prevents concurrent update conflicts, and establishes an immutable, auditable policy ledger.

---

## ADR-007: Mandatory Detection-Prevention Pairing & Continuous Validation
- **Status**: Accepted
- **Context**: Admission controls only inspect resources at the moment of creation or update (`CREATE` / `UPDATE`). They do not detect configuration drift, post-admission tampering, or rule tightening applied to existing running pods.
- **Decision**: Invariant `P0-04`: Every enforcing prevention rule MUST have an automated background detection counterpart:
  - Pod Security Admission is paired with continuous Policy Controller audit scans.
  - Binary Authorization image admission is paired with Google Cloud Continuous Validation (`CV`).
  - Cloud Organization Policies are paired with Security Command Center posture scans.
  CI blocks any rule commit that lacks a registered detection pair mapping.
- **Consequences**: Eliminates blind spots for long-running workloads deployed prior to policy updates; ensures compliance auditors receive continuous, provable posture evidence.
