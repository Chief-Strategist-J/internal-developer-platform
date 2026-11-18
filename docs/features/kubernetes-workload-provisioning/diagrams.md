# Architecture & Operational Diagrams: Kubernetes Workload Provisioning

This document provides exhaustive, production-grade architectural diagrams and end-to-end runtime execution trees mapping the complete lifecycle of workload provisioning across our 3-API platform topology.

---

## 1. Complete Runtime Execution ASCII Tree

The following ASCII tree maps the exact invocation sequence of a workload provisioning transaction from edge ingress down to cluster readiness observation:

```text
[HTTP Ingress Gateway] : POST /api/v1/workloads
  │
  ├── 1. Ingress Middleware Pipeline
  │     ├── Extract & Validate W3C Trace Context (traceparent, tracestate)
  │     ├── Ingest Protocol Headers (x-request-id, x-correlation-id, x-tenant-id, x-client-id)
  │     ├── Enforce Ingress Gateway Max Body Size & Closed Schema (additionalProperties: false)
  │     └── Initialize OpenTelemetry Root Server Span: "POST /api/v1/workloads"
  │
  ├── 2. Delivery Handler: WorkloadProvisioningHandler.Handle()
  │     ├── Deserialize JSON Request DTO → contracts.WorkloadProvisionCommand
  │     ├── Open Internal Child Span: "kubernetes.workload.provision"
  │     └── Invoke Domain Service: WorkloadProvisioningService.ProvisionWorkload(ctx, cmd)
  │
  ├── 3. Workload Provisioning Service Execution
  │     │
  │     ├── 3.1 Command Contract Validation
  │     │     ├── Child Span: "kubernetes.workload.validate_command"
  │     │     ├── Verify RFC-1123 DNS Subdomain Names (workload_name, namespace)
  │     │     ├── Verify Positive Integer Replicas & Immutable Image Digest
  │     │     └── ⚠ On Failure: RecordError(span, err) → Return 400 ValidationError
  │     │
  │     ├── 3.2 Root-Cause Idempotency Check
  │     │     ├── Child Span: "kubernetes.workload.idempotency_lookup"
  │     │     ├── Query Postgres: SELECT * FROM workload_provisions WHERE tenant_id = :t AND idempotency_key = :k
  │     │     └── ↳ If Exists: Return Cached Record (Idempotent 200 OK)
  │     │
  │     ├── 3.3 Mutation Evaluation
  │     │     ├── Child Span: "kubernetes.workload.evaluate_mutations"
  │     │     ├── Calculate CPU Request/Limit Headroom & Memory Ratio
  │     │     ├── Determine PodDisruptionBudget requirement (minAvailable=1 if replicas >= 2)
  │     │     └── Inject Managed Cloud Service Mesh Annotations ("sidecar.istio.io/inject": "true")
  │     │
  │     ├── 3.4 Namespace Quota Headroom Verification
  │     │     ├── Child Span: "kubernetes.workload.verify_quota_headroom"
  │     │     ├── Downstream K8s Call: k8sClient.GetNamespaceQuota(ctx, namespace)
  │     │     ├── Evaluate: headroom.RemainingCPU >= requiredCPU && headroom.RemainingMem >= requiredMem
  │     │     └── ⚠ On Insufficient Quota: RecordError(span, quotaErr) → Return 409 ConflictError
  │     │
  │     ├── 3.5 In-Process CEL Admission Policy Gate
  │     │     ├── Child Span: "kubernetes.workload.validate_admission"
  │     │     ├── Delegate to CELEvaluator.Evaluate(ctx, admissionInput)
  │     │     │     ├── Evaluate Rule 1: Restricted Pod Security Standard (runAsNonRoot, drop ALL caps)
  │     │     │     ├── Evaluate Rule 2: Cloud Service Mesh Injection Mandatory
  │     │     │     ├── Evaluate Rule 3: Namespace Default-Deny NetworkPolicy Enforced
  │     │     │     ├── Evaluate Rule 4: Immutable @sha256: Image Digest Pinned
  │     │     │     ├── Evaluate Rule 5: Mandatory Governance Labels Present (request-id, owner, cost-center)
  │     │     │     └── Evaluate Rule 6: Disallow Privileged Containers
  │     │     └── ⚠ On Violation: RecordError(span, err) → Transition FSM to REJECTED → Return 422 UnprocessableEntity
  │     │
  │     ├── 3.6 Provisioning DAG Execution
  │     │     ├── Child Span: "kubernetes.workload.dag.execute"
  │     │     ├── Step 1: Validate Spec Blueprint
  │     │     ├── Step 2: Apply Evaluated Mutations
  │     │     ├── Step 3: Verify Resource Quotas
  │     │     └── Step 4: Generate K8s Spec Manifests (Deployment, Service, PDB)
  │     │
  │     ├── 3.7 Initial State Persistence
  │     │     ├── Child Span: "kubernetes.workload.db_insert"
  │     │     └── SQL Insert: INSERT INTO workload_provisions (status = 'SUBMITTED', version = 1)
  │     │
  │     ├── 3.8 State Machine Transition: SUBMITTED → VALIDATED → ADMITTED → PROVISIONING
  │     │     ├── Child Span: "kubernetes.workload.state_machine_transition"
  │     │     └── Enforce Strict State Matrix & Transition Guards
  │     │
  │     ├── 3.9 Downstream Resilient Kubernetes / GitOps Application
  │     │     ├── Child Span: "kubernetes.workload.apply_deployment"
  │     │     ├── Acquire Bulkhead Slot (MaxInflight Concurrency Semaphore)
  │     │     ├── Verify Circuit Breaker State (CLOSED)
  │     │     ├── Deduct Integer Milli-Tokens from Rate Limiter Bucket
  │     │     ├── Execute k8sClient.ApplyDeployment(ctx, cmd, mutations)
  │     │     └── ↳ If mutations.PDBRequired: Execute k8sClient.ApplyPDB(ctx, ns, name, minAvailable)
  │     │
  │     ├── 3.10 Cluster Readiness Observation
  │     │     ├── Child Span: "kubernetes.workload.observe_readiness"
  │     │     ├── Poll Pod Status: readyReplicas == desiredReplicas && endpointsAvailable
  │     │     ├── ↳ If Ready: FSM Transition → 'ACTIVE' (Version = 2)
  │     │     └── ↳ If Degraded/Timeout: FSM Transition → 'DEGRADED' (Version = 2)
  │     │
  │     └── 3.11 Final Audit Persistence & Response
  │           ├── Update Database: UPDATE workload_provisions SET status = :s, version = 2 WHERE id = :id AND version = 1
  │           └── Return 201 Created Enveloped Response with WorkloadProvisionResponse DTO
```

---

## 2. Cross-API Platform Lifecycle Ordering

The platform enforces a strict unidirectional dependency hierarchy. Workloads cannot be deployed until their parent cluster is hardened and dependent cloud resources are provisioned:

```mermaid
flowchart TD
    subgraph Tier1["1. Cluster API (§4.1)"]
        GKE["GKE Cluster Provisioning Engine"]
        POSTURE["9-Point Hardened Baseline Validation<br/>(Private Endpoints, Shielded Nodes, BinAuth, Workload Identity)"]
        GKE_READY["Cluster Status: READY"]
        GKE --> POSTURE
        POSTURE --> GKE_READY
    end

    subgraph Tier2["2. Cloud Resource API (§4.2)"]
        WI["Workload Identity KSA-to-GSA Binding<br/>(roles/iam.workloadIdentityUser)"]
        DNS["Private Cloud DNS Zone Record<br/>(A / CNAME with TTL)"]
        SQL["Cloud SQL Database & IAM User<br/>(CLOUD_IAM_SERVICE_ACCOUNT)"]
        LEDGER["Cloud Resource Ledger<br/>(Orphan Detection & Deletion Protection)"]
        WI --> LEDGER
        DNS --> LEDGER
        SQL --> LEDGER
    end

    subgraph Tier3["3. Workload API (§4.3)"]
        CMD["Tenant Provisioning Request"]
        CEL["In-Process CEL Admission Gate<br/>(Restricted PSS, Default Deny, Mesh Inject)"]
        DAG["Provisioning Execution DAG"]
        K8S_SPEC["Render Manifests<br/>(Deployment, Service, PDB)"]
        CMD --> CEL
        CEL --> DAG
        DAG --> K8S_SPEC
    end

    subgraph Tier4["4. GitOps Single Writer (§0.17)"]
        GITOPS["GitOps Manifest Publisher<br/>(src/infra/clients/gitops)"]
        MERKLE["Deterministic SHA-256 Merkle Tree Commit"]
        REPO[("Platform Git Repository")]
        ARGO["Argo CD Application Controller<br/>(src/infra/clients/argocd)"]
        CLUSTER[("Target Hardened GKE Cluster")]
        
        GITOPS --> MERKLE
        MERKLE --> REPO
        REPO --> ARGO
        ARGO -- "Single Writer Sync" --> CLUSTER
    end

    GKE_READY --> Tier2
    LEDGER --> Tier3
    K8S_SPEC --> GITOPS
```

---

## 3. Downstream Client Resilience Architecture

All downstream network communication with Kubernetes API servers, GCP services, and Argo CD traverses our resilient execution chassis:

```mermaid
flowchart LR
    REQ["Downstream Request"] --> BH{"Bulkhead<br/>Channel Semaphore"}
    
    BH -- "Slots Available" --> CB{"Circuit Breaker<br/>State"}
    BH -- "Max Inflight Reached" --> DROP_BH["429 DownstreamRateLimitedError"]

    CB -- "CLOSED (Normal)" --> TB{"Token Bucket<br/>Integer Milli-Tokens"}
    CB -- "OPEN (Tripped)" --> DROP_CB["503 DownstreamUnavailableError"]
    CB -- "HALF-OPEN (Probe)" --> PROBE["Execute Single Probe"]

    TB -- "Tokens Available" --> RETRY["Retry Budget & Backoff<br/>(Exponential + Full Jitter)"]
    TB -- "Tokens Exhausted" --> DROP_TB["429 DownstreamRateLimitedError"]

    RETRY --> EXEC["Execute Network Call<br/>(Kubernetes / GCP / Argo CD)"]
    EXEC -- "Success" --> OK["Return Result & Reset Failures"]
    EXEC -- "Transient Failure" --> RETRY
    EXEC -- "Consecutive Failures >= 3" --> TRIP["Trip Circuit Breaker to OPEN"]
```

---

## 4. Finite State Machine (FSM) Transition Diagram

Every workload provision record transitions through an audited finite state machine with optimistic locking:

```mermaid
stateDiagram-v2
    [*] --> SUBMITTED : Client Command Received
    SUBMITTED --> VALIDATED : Command Schema & Spec Validated
    VALIDATED --> ADMITTED : In-Process CEL & Quota Passed
    VALIDATED --> REJECTED : Admission Violations / Quota Insufficient
    
    ADMITTED --> PROVISIONING : Manifests Generated & Dispatched
    PROVISIONING --> ACTIVE : All Replicas Ready & Healthy
    PROVISIONING --> DEGRADED : Partial Replica Failure / Readiness Timeout
    PROVISIONING --> REJECTED : Downstream K8s / KCC Apply Error

    ACTIVE --> DECOMMISSIONED : Teardown Command Issued
    DEGRADED --> PROVISIONING : Reconciliation Re-attempt
    REJECTED --> [*]
    DECOMMISSIONED --> [*]
```

---

## 5. Platform Cross-Feature Entity-Relationship (ER) Architecture

The platform links tenant workloads, GitOps reconciliation audits, and security policies across a unified multi-tenant relational boundary:

```mermaid
erDiagram
    TENANT ||--o{ WORKLOAD_PROVISIONS : provisions
    TENANT ||--o{ GITOPS_RECONCILIATIONS : tracks
    TENANT ||--o{ POLICIES : applies

    WORKLOAD_PROVISIONS ||--o{ GITOPS_RECONCILIATIONS : "reconciles workload_id"
    POLICIES ||--o{ WORKLOAD_PROVISIONS : "governs admission"

    WORKLOAD_PROVISIONS {
        uuid id PK "Workload Identity UUID"
        uuid tenant_id "Tenant Boundary"
        text idempotency_key "UK(tenant_id, idempotency_key)"
        text workload_name "RFC-1123 DNS Subdomain"
        text namespace "Target Namespace"
        text image "Container Image with @sha256: Digest"
        text cpu_request "Normalized Millicores"
        text memory_request "Normalized Megabytes"
        int replicas "Replicas Count >= 1"
        text status "CHECK: SUBMITTED, VALIDATED, ADMITTED, PROVISIONING, ACTIVE, DEGRADED, REJECTED"
        text strategy "CHECK: rolling, canary, blue_green"
        bigint version "Optimistic Concurrency Counter"
        timestamptz created_at "Creation Timestamp"
        timestamptz updated_at "Update Timestamp"
    }

    GITOPS_RECONCILIATIONS {
        uuid id PK "Reconciliation Record ID"
        uuid tenant_id "Tenant Boundary"
        text workload_id "Referenced Workload"
        text commit_sha "SHA-256 Merkle Commit Hash"
        text status "CHECK: SYNCING, HEALTHY, DEGRADED, UNKNOWN"
        timestamptz reconciled_at "Reconciliation Timestamp"
        text error_message "Failure Detail Envelope"
        timestamptz created_at "Creation Timestamp"
        timestamptz updated_at "Update Timestamp"
    }

    POLICIES {
        uuid tenant_id PK,FK "Tenant Isolation Boundary"
        uuid id PK "Internal UUIDv7"
        text public_id UK "Prefixed: pol_01J..."
        text name "Policy Name"
        text enforcement_point "CEL, PSA, PC, BA, ORG, CI, ACD"
        text severity "CRITICAL, HIGH, MEDIUM, ADVISORY"
        text lifecycle_state "DRAFT, CI_ONLY, AUDIT, WARN, DENY"
        text rule_expression "CEL Rule Definition"
        boolean exceptable "Exemption Flag"
        bigint version "Version Lock"
        timestamptz created_at "Creation Timestamp"
        timestamptz updated_at "Update Timestamp"
    }
```

### Criticality Rationale: Why Cross-Feature Relational Integrity Is Essential

1. **Root-Cause Idempotency & Race Condition Prevention**:
   - The unique constraint `UNIQUE (tenant_id, idempotency_key)` on `workload_provisions` guarantees mutation safety at the database engine level. Duplicate HTTP POST requests from flaky networks are neutralized before any Kubernetes or GitOps write occurs.
2. **Cryptographic Traceability from Spec to Cluster State**:
   - Linking `workload_provisions` to `gitops_reconciliations` via `commit_sha` ensures that every running container in the GKE cluster can be traced back to its exact Git commit hash, its triggering API request ID, and the governing policy set.
3. **Optimistic Locking Guardrails**:
   - The `version BIGINT` column on all stateful tables prevents lost updates during concurrent scale or rollout commands (`WHERE id = :id AND version = :version`), strictly conforming to Guardrail 7 of `2-apis.writing.rule.md`.
