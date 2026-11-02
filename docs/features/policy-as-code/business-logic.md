# Policy-as-Code — Business Logic Decision Trees

## 1. Architectural Overview & Policy Gate Decision Tree

```
POLICY-AS-CODE ENGINE (COMPLIANCE & GUARDRAIL DECISION TREE)
│
├── 1. ADMISSION HOOK & CHANGE INTERCEPTION TIER [P0]
│   └── Real-World: Developer pushes Kubernetes manifest or Terraform PR requesting public S3 bucket or root container
│       └── Trigger: Pre-Commit Hook / CI Pull Request / K8s Admission Webhook (ValidatingAdmissionWebhook)
│           ├── Check: Is Request originating from Exempt System Namespace (kube-system, platform-core)?
│           │   ├── YES ──> Check: Is System Namespace mutation explicitly protected against user tampering?
│           │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Non-platform actor cannot mutate system namespace]
│           │   │   └── YES ──> [BYPASS: Allow platform internal control loop]
│           │   └── NO ──> Proceed to Policy Evaluation
│           │
├── 2. REGISTRATION & POLICY REGISTRY RESOLUTION TIER [P0]
│   └── Real-World: Enterprise security pushes CIS Benchmark & SOC2 policies across all clusters
│       └── Evaluation: Fetch Active Policies for Tenant, Target Environment, and Resource Kind
│           ├── Action: Compile OPA Rego / Kyverno / Cedar policies from GitOps Policy Repo
│           ├── Check: Are all compiled policy bundles cryptographically signed by SecOps Cosign key?
│           │   ├── NO ──> [HALT: Unsigned policy detected — Refuse to evaluate; alert SecOps]
│           │   └── YES ──> Policy Set Loaded into In-Memory Policy Decision Point (PDP)
│           │
├── 3. RULE EVALUATION & SEVERITY CLASSIFICATION TIER [P0 / P1]
│   └── Real-World: Manifest evaluated against security rules (No root user, mandatory CPU limits, no `latest` tag)
│       └── Evaluation: Policy Match Rules
│           ├── Rule 1: Privileged Containers (`securityContext.privileged == true`)
│           │   ├── MATCH ──> Severity: CRITICAL [ENFORCE: HARD BLOCK]
│           │   └── PASS ──> Continue
│           ├── Rule 2: Resource Quotas & Limits (`resources.limits.cpu` & `memory` present)
│           │   ├── MISSING ──> Severity: HIGH [ENFORCE: HARD BLOCK]
│           │   └── PASS ──> Continue
│           ├── Rule 3: Mutable Image Tags (`image: app:latest`)
│           │   ├── MATCH ──> Severity: HIGH [ENFORCE: HARD BLOCK — Immutable digest or semver required]
│           │   └── PASS ──> Continue
│           ├── Rule 4: Mandatory Compliance Labels (`cost-center`, `owner-email`, `tier`)
│           │   ├── MISSING ──> Check: Enforcement Mode (ENFORCE vs AUDIT / WARN)?
│           │   │   ├── ENFORCE ──> [HARD BLOCK: Deployment Rejected]
│           │   │   └── WARN / AUDIT ──> [PERMIT WITH WARNING: Emit Linter Warning in CI / Webhook Response]
│           │   └── PASS ──> Continue
│           │
├── 4. MUTATING ADMISSION & AUTOMATED REMEDIATION TIER [P1]
│   └── Real-World: Developer forgets to inject sidecar or standard service mesh proxy labels
│       └── Evaluation: Can missing configuration be safely mutated automatically?
│           ├── Check: Does Mutation Policy define an idempotent default patch?
│           │   ├── YES
│           │   │   ├── Action: Generate JSONPatch (e.g. inject mesh sidecar, default drop capabilities)
│           │   │   └── Action: Record Mutation Diff in Audit Log
│           │   └── NO ──> Retain original manifest
│           │
└── 5. DRIFT DETECTION & CONTINUOUS AUDIT TIER [P1 / P2]
    └── Real-World: Rogue operator edits live cluster resource directly using emergency admin credentials
        └── Trigger: Scheduled Continuous Audit Scanner (Every 15 mins)
            ├── Action: Scan all running cluster workloads against active policy bundle
            ├── Check: Is any running resource non-compliant with current policy?
            │   ├── NO ──> Cluster State Compliant
            │   └── YES ──> Action: Check Remediation Policy
            │       ├── AUTO_REMEDIATE ──> Action: Overwrite cluster state with GitOps source of truth
            │       └── NOTIFY_ONLY ──> Action: Create PagerDuty / Jira Incident Ticket with Resource & Policy Diff
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Fail-Closed Admission on Policy Engine Timeout
- Real-Life Incident: A policy webhook became slow under high cluster load; Kubernetes admission timed out and defaulted to permissive pass, allowing an unvetted crypto-miner container to run.
- Rationale: For security-critical policies, failure or timeout of the policy engine must fail closed (deny admission) rather than fail open.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: FAIL-CLOSED POLICY (P0-01)
Trigger: Admission Webhook Invocation
├── Check: Does Policy Engine respond within 2000ms deadline?
│   ├── YES ──> Respect Policy Verdict (Permit / Deny)
│   └── NO (Timeout / HTTP 500)
│       ├── Check: Is Policy Scope marked CRITICAL_SECURITY?
│       │   ├── YES ──> [FAIL CLOSED: Reject Admission — HTTP 500 Admission Webhook Timeout]
│       │   └── NO (Advisory Policy) ──> [FAIL OPEN: Allow with Warning Header & Alert Ops]
```

### Invariant P0-02: Cryptographic Policy Bundle Signing
- Real-Life Incident: An attacker committed a malicious Rego file directly to a branch that auto-synced to cluster admission controllers, disabling all container privilege checks.
- Rationale: Policies enforce security; therefore, policy bundles must be signed cryptographically via Cosign / Sigstore and verified prior to in-memory compilation.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: POLICY SIGNATURE (P0-02)
Trigger: Policy Bundle Pull from Registry
├── Verify: Cosign signature against Enterprise Public Key
├── Check: Signature Valid & Transparency Log entry verified?
│   ├── YES ──> Load and activate policy in PDP
│   └── NO ──> [HALT: Reject bundle; remain on last known healthy policy version]
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Breaking Policy Update Blocks Emergency Production Patch
- Real-Life Incident: SecOps rolled out a strict new rule requiring 5 new labels; a developer attempting to push an emergency Sev-1 bugfix was blocked by the admission webhook.
- Decision Tree:
```
EDGE CASE 01: EMERGENCY POLICY OVERRIDE
Trigger: High-Priority Hotfix Blocked by Policy
├── Check: Does Request carry an Active Incident Override Token signed by Incident Commander?
│   ├── YES
│   │   ├── Check: Is Policy Overridden marked as NON-OVERRIDABLE (e.g. root privilege, host network)?
│   │   │   ├── YES ──> [HARD BLOCK: Inviolable Core Security Boundary]
│   │   │   └── NO ──> [ALLOW OVERRIDE: Permit deployment, set 24h remediation SLA, alert SecOps]
│   └── NO ──> [STANDARD REJECT: Must fulfill policy requirements]
```

### Case 02: Mutating Admission Webhook Infinite Patch Loop
- Real-Life Incident: Two competing mutating webhooks (Mesh Injector vs Policy Auto-Fixer) kept modifying the same container spec back and forth, exhausting Kubernetes API etcd storage.
- Decision Tree:
```
EDGE CASE 02: MUTATION LOOP DETECTION
Trigger: Mutating Admission Webhook invoked on Resource
├── Check: Has Resource Generation / Mutation Count exceeded MAX_MUTATIONS (5)?
│   ├── YES ──> [HALT: Detect Loop — Reject admission and log conflicting webhook names]
│   └── NO ──> Apply Patch with idempotency check
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
POLICY-AS-CODE ROADMAP
│
├── PHASE 0: ADMISSION GATEWAY & CORE SECURITY RULES (P0 - WEEKS 1-3)
│   ├── ValidatingAdmissionWebhook / OPA Gatekeeper integration
│   ├── P0 security rules: Block privileged containers, root users, hostPath mounts
│   ├── Signed policy distribution via OCI artifacts
│   └── Deliverable: Hardened cluster admission rejecting high-risk workloads
│
├── PHASE 1: GOVERNANCE, QUOTAS & LINTER INTEGRATION (P1 - WEEKS 4-6)
│   ├── Mandatory resource requests/limits and compliance labels enforcement
│   ├── Shift-left CLI / PR linter (`conftest` / `opa test`) for developers
│   ├── Automated warning mode for phased policy rollouts
│   └── Deliverable: Developer PR feedback loop prior to cluster deployment
│
├── PHASE 2: MUTATING ADMISSION & DRIFT REMEDIATION (P2 - WEEKS 7-9)
│   ├── Mutating webhooks for automatic sidecar and label injection
│   ├── Background drift scanner reporting non-compliant running workloads
│   └── Deliverable: Self-healing policy enforcement with automated remediation
│
└── PHASE 3: MULTI-CLOUD & CROSS-INFRASTRUCTURE UNIFICATION (P3 - WEEKS 10-12)
    ├── Unified policy engine across Kubernetes, Terraform, and Cloud IAM
    ├── Automated policy impact simulator across historical deployment manifests
    └── Deliverable: Single-pane policy compliance across multi-cloud infrastructure
```
