# Architecture & Operational Diagrams: Policy as Code

This document details the runtime architecture, compiled Common Expression Language (CEL) evaluation pipeline, fail-closed policy execution trees, and distributed tracing telemetry hierarchies for the Policy as Code engine.

---

## 1. CEL Policy Engine Runtime Execution ASCII Tree

The following ASCII tree illustrates the two-phase lifecycle of the CEL admission policy engine: from boot-time AST compilation down to sub-millisecond per-request evaluation:

```text
[Engine Boot Phase] : NewDefaultGovernmentCELEvaluator()
  │
  ├── 1. Initialize CEL Environment (cel.NewEnv)
  │     ├── Declare Variables with Strict Static Types:
  │     │     ├── VarPodSecurityLevel  : cel.StringType ("pod_security_level")
  │     │     ├── VarMeshInjection     : cel.BoolType   ("mesh_injection")
  │     │     ├── VarDefaultDenyNetPol : cel.BoolType   ("default_deny_netpol")
  │     │     ├── VarImage             : cel.StringType ("image")
  │     │     ├── VarPrivileged        : cel.BoolType   ("privileged")
  │     │     ├── VarLabels            : cel.MapType(cel.StringType, cel.StringType)
  │     │     └── VarAnnotations       : cel.MapType(cel.StringType, cel.StringType)
  │     └── ⚠ On Environment Creation Failure: Return Fatal Startup Error
  │
  └── 2. Immutable AST Compilation Loop
        ├── For each rule in DefaultGovernmentPolicies:
        │     ├── Compile Expression: env.Compile(rule.Expression)
        │     ├── Verify Syntax & Type Safety (iss.Err() == nil)
        │     ├── Generate Evaluation Program: env.Program(ast)
        │     └── Cache in Memory: compiledRule{rule, program}
        └── Pre-compiled Programs Ready for Zero-Allocation Runtime Ingestion

─────────────────────────────────────────────────────────────────────────────

[Per-Request Ingestion Phase] : CELEvaluator.Evaluate(ctx, input)
  │
  ├── 1. Distributed Tracing Initialization
  │     ├── Open OTel Child Span: "kubernetes.workload.admission.cel_evaluate"
  │     ├── Set Attributes: workload.name, namespace
  │     └── Defer span.End()
  │
  ├── 2. Evaluation Context Construction
  │     ├── Normalize Labels & Annotations (Prevent Nil Map Panics)
  │     └── Build Runtime Context Map:
  │           {
  │             "pod_security_level":  input.PodSecurityLevel,
  │             "mesh_injection":      input.MeshInjection,
  │             "default_deny_netpol": input.DefaultDenyNetPol,
  │             "image":               input.Image,
  │             "privileged":          input.Privileged,
  │             "labels":              input.Labels,
  │             "annotations":         input.Annotations
  │           }
  │
  ├── 3. Sequential Compiled Rule Evaluation
  │     │
  │     ├── 3.1 Rule: GOV_SEC_001_POD_SECURITY_RESTRICTED
  │     │     ├── Eval: pod_security_level == 'restricted'
  │     │     └── ↳ If False: Append ViolationDetail(SeverityBlocker)
  │     │
  │     ├── 3.2 Rule: GOV_NET_002_MESH_INJECTION_MANDATORY
  │     │     ├── Eval: mesh_injection == true
  │     │     └── ↳ If False: Append ViolationDetail(SeverityBlocker)
  │     │
  │     ├── 3.3 Rule: GOV_NET_003_DEFAULT_DENY_NETPOL
  │     │     ├── Eval: default_deny_netpol == true
  │     │     └── ↳ If False: Append ViolationDetail(SeverityBlocker)
  │     │
  │     ├── 3.4 Rule: GOV_IMG_004_IMMUTABLE_IMAGE_DIGEST
  │     │     ├── Eval: image.contains('@sha256:')
  │     │     └── ↳ If False: Append ViolationDetail(SeverityBlocker)
  │     │
  │     ├── 3.5 Rule: GOV_GOV_005_MANDATORY_GOVERNANCE_LABELS
  │     │     ├── Eval: 'platform.gov/request-id' in labels && 'platform.gov/owner' in labels && 'platform.gov/cost-center' in labels
  │     │     └── ↳ If False: Append ViolationDetail(SeverityCritical)
  │     │
  │     └── 3.6 Rule: GOV_SEC_006_PRIVILEGED_CONTAINER_DISALLOWED
  │           ├── Eval: !privileged
  │           └── ↳ If False: Append ViolationDetail(SeverityBlocker)
  │
  ├── 4. Fail-Closed Error Handling & Origin Telemetry Stamping
  │     ├── If Program.Eval() Returns Non-Nil Error:
  │     │     ├── Immediately Invoke: observability.RecordError(span, evalErr)
  │     │     └── Treat as Violation: Append ViolationDetail("evaluation error: ...")
  │     │
  │     └── If len(violations) > 0:
  │           ├── Mark Result: Allowed = false
  │           ├── Stamp Span Attribute: admission.allowed = false
  │           └── Stamp Span Attribute: admission.violations.count = N
  │
  └── 5. Result Envelope Return
        └── Return &AdmissionEvaluationResult{Allowed, Violations, EvaluatedCount: 6}
```

---

## 2. In-Process Admission Evaluation Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    participant Client as Workload Service
    participant Evaluator as CELEvaluator
    participant Tracer as OpenTelemetry Tracer
    participant Program as Pre-Compiled cel.Program
    participant Span as Child Span (cel_evaluate)

    Client->>Evaluator: Evaluate(ctx, input)
    Evaluator->>Tracer: StartSpan("kubernetes.workload.admission.cel_evaluate")
    Tracer-->>Evaluator: ctx, Span

    loop For each compiled policy (1..6)
        Evaluator->>Program: Eval(evalCtx)
        alt Evaluation Succeeds & Returns True
            Program-->>Evaluator: true (Compliant)
        else Evaluation Returns False (Violation)
            Program-->>Evaluator: false
            Evaluator->>Evaluator: Append ViolationDetail(rule.ID, rule.FailureMessage)
        else Evaluation Throws Error (Fail-Closed)
            Program-->>Evaluator: error
            Evaluator->>Span: RecordError(span, evalErr)
            Evaluator->>Evaluator: Append ViolationDetail(SeverityBlocker, "eval error")
        end
    end

    Evaluator->>Span: SetAttributes(admission.allowed, violations.count)
    Evaluator->>Span: End()
    Evaluator-->>Client: AdmissionEvaluationResult
```

---

## 3. Fail-Closed Policy Enforcement Partition

Every incoming workload spec is partitioned into compliant or rejected regions based on our 6-point government baseline:

```mermaid
flowchart TD
    SPEC["Workload Admission Input"] --> PSS{"1. Pod Security<br/>Restricted?"}
    
    PSS -- "No (Baseline/Privileged)" --> V1["Violation: GOV_SEC_001"]
    PSS -- "Yes" --> MESH{"2. Mesh Injection<br/>Enabled?"}

    MESH -- "No" --> V2["Violation: GOV_NET_002"]
    MESH -- "Yes" --> NETPOL{"3. Default Deny<br/>NetPol True?"}

    NETPOL -- "No" --> V3["Violation: GOV_NET_003"]
    NETPOL -- "Yes" --> IMG{"4. Image Pins<br/>@sha256: Digest?"}

    IMG -- "No (Mutable Tag)" --> V4["Violation: GOV_IMG_004"]
    IMG -- "Yes" --> LABELS{"5. Mandatory Labels<br/>(RequestID, Owner, CostCenter)?"}

    LABELS -- "Missing Keys" --> V5["Violation: GOV_GOV_005"]
    LABELS -- "Present" --> PRIV{"6. Privileged<br/>Flag Set?"}

    PRIV -- "True" --> V6["Violation: GOV_SEC_006"]
    PRIV -- "False" --> PASS["Result: Allowed = true (Zero Violations)"]

    V1 --> AGG["Aggregate Violations List"]
    V2 --> AGG
    V3 --> AGG
    V4 --> AGG
    V5 --> AGG
    V6 --> AGG
    AGG --> FAIL["Result: Allowed = false → Reject Admission (422)"]
```

---

## 4. Entity-Relationship (ER) Architecture: Policies & Exceptions

The Policy as Code persistence layer implements the strict relational constraints defined in `context.yml` and Migration `0006_create_policies_tables.sql`:

```mermaid
erDiagram
    TENANT ||--o{ POLICIES : owns
    TENANT ||--o{ POLICY_EXCEPTIONS : owns

    POLICY_LIFECYCLE_STATES ||--o{ POLICY_STATE_TRANSITIONS : from_status
    POLICY_LIFECYCLE_STATES ||--o{ POLICY_STATE_TRANSITIONS : to_status
    POLICY_LIFECYCLE_STATES ||--o{ POLICIES : lifecycle_state

    POLICY_EXCEPTION_STATUS ||--o{ POLICY_EXCEPTIONS : status

    POLICIES ||--o{ POLICY_EXCEPTIONS : "composite FK (tenant_id, policy_id)"

    POLICIES {
        uuid tenant_id PK,FK "Tenant Isolation Boundary"
        uuid id PK "Internal UUIDv7"
        text public_id UK "Prefixed: pol_01J..."
        text source "CHECK: GIT, API, SYSTEM"
        text external_id "UK(tenant_id, source, ext_id)"
        uuid correlation_id "W3C Trace Parent Link"
        text name "CHECK: Trimmed, <= 256 chars"
        text description "Default: empty string"
        text enforcement_point "CHECK: CEL, PSA, PC, BA, ORG, CI, ACD"
        text severity "CHECK: CRITICAL, HIGH, MEDIUM, ADVISORY"
        text lifecycle_state FK "REFERENCES policy_lifecycle_states(code)"
        text rule_expression "Trimmed CEL Expression"
        boolean exceptable "Enforces Exemption Eligibility"
        text detection_pair "Associated Prometheus / Audit Rule"
        bigint version "Optimistic Locking Counter"
        timestamptz created_at "Audit Creation Timestamp"
        timestamptz updated_at "Maintained by Storage Engine"
        timestamptz deleted_at "Soft Delete Window"
    }

    POLICY_EXCEPTIONS {
        uuid tenant_id PK,FK "Tenant Isolation Boundary"
        uuid id PK "Internal UUIDv7"
        text public_id UK "Prefixed: pex_01J..."
        uuid policy_id FK "Composite FK to POLICIES"
        text workload_id "Workload Identifier"
        text namespace "RFC-1123 Target Namespace"
        text approver_id "Audit Trail Approver Principal"
        text justification "CHECK: length >= 10 chars"
        text status FK "REFERENCES policy_exception_status(code)"
        timestamptz expires_at "CHECK: expires_at > created_at"
        uuid correlation_id "Audit Trace Link"
        bigint version "Optimistic Locking Counter"
        timestamptz created_at "Creation Timestamp"
        timestamptz updated_at "Update Timestamp"
        timestamptz deleted_at "Soft Delete Window"
    }

    POLICY_LIFECYCLE_STATES {
        text code PK "DRAFT, CI_ONLY, AUDIT, WARN, DENY, ROLLED_BACK, RETIRED"
        text description "Human-readable lifecycle description"
    }

    POLICY_STATE_TRANSITIONS {
        text from_status PK,FK "Source State"
        text to_status PK,FK "Permitted Destination State"
    }

    POLICY_EXCEPTION_STATUS {
        text code PK "PENDING, ACTIVE, REVOKED, EXPIRED"
        text description "Exception lifecycle state description"
    }
```

### Criticality Rationale: Why These Relational Constraints Are Essential

1. **Non-Bypassable Multi-Tenant Isolation via Composite Keys**:
   - Rather than relying on simple single-column primary keys, both `policies` and `policy_exceptions` enforce `PRIMARY KEY (tenant_id, id)`.
   - The foreign key relationship from `policy_exceptions` back to `policies` is strictly composite: `FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id)`.
   - **Why Critical**: This ensures that even if an application bug attempts to link an exception in Tenant A to a policy in Tenant B, the PostgreSQL engine immediately aborts with a foreign key constraint violation. Cross-tenant leakage is mathematically impossible at the persistence boundary.

2. **Storage-Enforced Finite State Machine**:
   - The lookup tables `policy_lifecycle_states` and `policy_state_transitions` codify permitted state paths (`DRAFT` → `CI_ONLY` → `AUDIT` → `WARN` → `DENY`).
   - **Why Critical**: State transitions cannot be corrupted by out-of-band database operations, unhandled panics, or application code regressions. The database engine actively rejects unauthorized jumps (e.g. promoting directly from `DRAFT` to `DENY`).

3. **Check Constraint Invariants & Zero Data Corruption**:
   - Mandatory field trimming: `CHECK (name = btrim(name) AND name <> '')` prevents whitespace pollution.
   - Closed domain sets: `CHECK (enforcement_point IN ('CEL', 'PSA', 'PC', 'BA', 'ORG', 'CI', 'ACD'))` ensures no untyped or invalid enforcement targets enter the platform.
   - Future expiration invariant: `CHECK (expires_at > created_at)` eliminates zero-day or retroactive exception bugs.
