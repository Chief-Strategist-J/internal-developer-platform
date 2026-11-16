# Architecture & Operational Diagrams: Kubernetes Workload Provisioning

This document visualizes the complete end-to-end lifecycle, finite state transitions, multi-tenant network isolation boundaries, and failure mitigation paths for the Workload Provisioning Engine.

---

## 1. High-Level Component Architecture Flow

```mermaid
flowchart TD
    subgraph Client["Tenant / API Gateway"]
        CMD["WorkloadProvisionCommand<br/>(RFC-1123, ID, Image, CPU, Mem, Replicas)"]
    end

    subgraph ServiceLayer["Workload Provisioning Hexagonal Service"]
        SVC["WorkloadProvisioningService"]
        VAL["ValidateWorkloadCommand"]
        IDEMP{"Idempotency Key<br/>Exists?"}
        MUT["EvaluateWorkloadMutations"]
        ADM{"ValidateAdmission"}
        FSM["State Machine Transitions<br/>SUBMITTED → VALIDATED → ADMITTED → ACTIVE"]
    end

    subgraph Storage["Persistence Layer"]
        PG[("Postgres / RLS Store<br/>workload_provisions table")]
    end

    subgraph K8sAdapter["Kubernetes Client Adapter"]
        GEN["Manifest Generator"]
        NS["Namespace (PSS Restricted, ASM)"]
        RQ["ResourceQuota (10 CPU, 20Gi)"]
        LR["LimitRange (Default Floor)"]
        NP["NetworkPolicy (Default Deny, DNS 53)"]
        DEP["Deployment (Non-root 10001, Probes)"]
        PDB["PDB (minAvailable=1)"]
    end

    CMD --> VAL
    VAL --> SVC
    SVC --> IDEMP
    IDEMP -- "Yes" --> PG
    IDEMP -- "No" --> MUT
    MUT --> ADM
    ADM -- "Violations" --> REJ["Status: REJECTED"]
    ADM -- "Compliant" --> FSM
    FSM --> PG
    FSM --> GEN
    GEN --> NS
    GEN --> RQ
    GEN --> LR
    GEN --> NP
    GEN --> DEP
    GEN --> PDB
```

---

## 2. Finite State Machine (FSM) Lifecycle & Rollback

Tracks the transition of every provision record from initial command receipt to observed cluster health. Any fatal error triggers immediate rollback to `REJECTED` and version increment to preserve the audit trail.

```mermaid
stateDiagram-v2
    [*] --> SUBMITTED : ProvisionWorkload() / ValidateSchema()
    
    SUBMITTED --> VALIDATED : EventValidate (Payload verified)
    SUBMITTED --> REJECTED : Schema Failure / Invalid RFC-1123
    
    VALIDATED --> ADMITTED : EventAdmit (CIS GKE Baseline Passed)
    VALIDATED --> REJECTED : Critical Security Violation (UID 0 / PrivEscalation)
    
    ADMITTED --> ACTIVE : EventActivate (K8s Manifests Applied & PDB Bound)
    ADMITTED --> REJECTED : K8s API Failure / Quota Exceeded (Rollback)
    
    ACTIVE --> REJECTED : Health Probe Failure / Pod Eviction Failure
    ACTIVE --> [*] : Observed In Running System
    REJECTED --> [*] : Audit Record Retained in Postgres
```

---

## 3. End-to-End GitOps & Admission Webhook Pipeline

Shows the complete control loop spanning the Workload API, Git repository, Argo CD, Kubernetes API Server, and admission verification:

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Developer / CI Pipeline
    participant Gateway as API Gateway / Router
    participant Service as WorkloadProvisioningService
    participant FSM as State Machine
    participant DB as Postgres (workload_provisions)
    participant Git as GitOps Git Repository
    participant Argo as Argo CD Reconciler
    participant K8s as GKE API Server (Admission)
    participant Node as Kubelet / Running Pods

    Dev->>Gateway: POST /api/v1/workloads (WorkloadProvisionCommand)
    Gateway->>Gateway: Extract X-Request-ID & X-Tenant-ID
    Gateway->>Service: ProvisionWorkload(ctx, cmd)
    Service->>DB: Query Idempotency Key
    alt Duplicate Request
        DB-->>Service: Return Existing Record
        Service-->>Gateway: HTTP 201 (Idempotent Replay)
        Gateway-->>Dev: Return Cached Manifest Bundle
    else New Request
        Service->>Service: EvaluateWorkloadMutations()
        Service->>Service: ValidateAdmission()
        Service->>DB: INSERT INTO workload_provisions (Status=SUBMITTED)
        Service->>FSM: Transition(EventValidate) -> VALIDATED
        Service->>FSM: Transition(EventAdmit) -> ADMITTED
        Service->>Service: GenerateManifestBundle()
        Service->>Git: Commit Manifests (Namespace, Quota, NetPol, Deployment, PDB)
        Git-->>Service: Git Commit SHA (Audit Logged)
        Service->>FSM: Transition(EventActivate) -> ACTIVE
        Service->>DB: UPDATE workload_provisions (Status=ACTIVE, Version=2)
        Service-->>Gateway: HTTP 201 Created (ManifestBundle + Status)
        Gateway-->>Dev: Success Response
        
        Note over Git,Argo: Asynchronous Reconciliation Loop
        Argo->>Git: Poll & Detect New Manifest Commit
        Argo->>K8s: Sync Application (Apply Manifests)
        K8s->>K8s: ValidatingAdmissionWebhook (Restricted PSS)
        K8s->>Node: Schedule Pods (Node Affinity + PDB)
        Node->>K8s: Update ReplicaStatus (AvailableReplicas = 3)
        Argo->>K8s: Verify Health (Observed State == Spec)
        Argo-->>Git: Status Synced & Healthy
    end
```

---

## 4. Multi-Tenant Cluster Namespace Isolation Topology

Visualizes how every workload is partitioned within its dedicated namespace with default-deny networking, restricted security contexts, and resource quotas:

```mermaid
flowchart TD
    subgraph Cluster["GKE Multi-Tenant Cluster"]
        subgraph NS["Tenant Namespace: payments-prod"]
            direction TB
            
            subgraph Policies["Boundary Policies"]
                PSS["PSS: pod-security.kubernetes.io/enforce=restricted"]
                ASM["Service Mesh: istio.io/rev=asm-managed (mTLS STRICT)"]
                RQ["ResourceQuota: 10 CPU / 20Gi Mem / 50 Pods"]
                LR["LimitRange: Container Floors"]
            end
            
            subgraph NetPol["Zero-Trust NetworkPolicy"]
                DENY_IN["Default Deny Ingress"]
                DENY_EG["Default Deny Egress"]
                ALLOW_DNS["Allow UDP/TCP 53 to kube-dns"]
            end
            
            subgraph Workload["Payment Workload"]
                DEP["Deployment: payment-api (replicas=3)"]
                PDB["PodDisruptionBudget: minAvailable=1"]
                
                subgraph Pod1["Pod 1 (Node A)"]
                    C1["Container: payment-api<br/>UID: 10001 (Non-root)<br/>RootFS: ReadOnly<br/>Drop: ALL<br/>Probes: /readyz, /livez"]
                end
                
                subgraph Pod2["Pod 2 (Node B)"]
                    C2["Container: payment-api<br/>UID: 10001 (Non-root)<br/>RootFS: ReadOnly<br/>Drop: ALL<br/>Probes: /readyz, /livez"]
                end
            end
            
            Policies -.-> Workload
            NetPol -.-> Workload
            DEP --> Pod1
            DEP --> Pod2
            PDB -.-> Pod1
            PDB -.-> Pod2
        end
    end
```

---

## 5. Failure Recovery & Quota Exceeded Scenario

Visualizes how the system detects, fails closed, and recovers when a workload exceeds namespace resource limits or fails admission checks:

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Developer / CI
    participant Service as WorkloadProvisioningService
    participant Store as Postgres Store
    participant K8s as GKE API Server
    participant Alert as Prometheus / Alertmanager

    Dev->>Service: ProvisionWorkload(CPU=50, Mem=100Gi)
    Service->>Service: EvaluateWorkloadMutations()
    Service->>K8s: Check ResourceQuota Headroom
    alt Quota Exceeded
        K8s-->>Service: 403 Forbidden (exceeded quota: limits.cpu requested 50, limit 20)
        Service->>Store: UPDATE workload_provisions (Status=REJECTED)
        Service->>Alert: Emit Metric (quota_rejection_total{tenant="ecommerce"})
        Service-->>Dev: 409 Conflict / Quota Exceeded (Actionable remediation instructions)
    else Admitted
        Service->>K8s: ApplyDeployment()
        Service-->>Dev: 201 Created
    end
```
