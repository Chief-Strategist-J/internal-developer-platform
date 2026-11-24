# Disaster Recovery: Architectural Diagrams

## 1. Disaster Recovery 15-State Lifecycle Machine

The DR Plan lifecycle strictly adheres to the 15-state forward-locking finite state machine. Transitions past `PROMOTING` are forward-only to prevent split-brain conditions.

```mermaid
stateDiagram-v2
    [*] --> NOT_READY: Plan Created
    NOT_READY --> READY: §2 Dependency Probes Pass
    READY --> NOT_READY: Dependency Probe Fails / Lag Exceeds RPO
    READY --> DECLARED: Incident Approved (Two-Person Quorum)
    DECLARED --> FENCING: Failover Initiated
    FENCING --> PROMOTING: Old Primary Fenced (Epoch Incremented)
    FENCING --> ABORTED: Abort Requested (Prior to Promotion)
    DECLARED --> ABORTED: Abort Requested
    PROMOTING --> SCALING: Database Promoted (Forward-Lock Engaged)
    SCALING --> VALIDATING: Capacity Reserved & Pods Scaled
    VALIDATING --> ROUTING: Posture & Health Validated
    ROUTING --> FAILED_OVER: Traffic Switched to DR Region
    FAILED_OVER --> RECONCILING: Failback Declared (Reverse Lag Monitored)
    RECONCILING --> FAILBACK_READY: Divergence == 0 & Reverse Lag == 0
    FAILBACK_READY --> FAILING_BACK: Two-Person Approved Switchover
    FAILING_BACK --> NORMAL: Traffic Restored to Primary
    NORMAL --> READY: Baseline Restored
    ABORTED --> NOT_READY: Incident Cleared
```

---

## 2. Six-Stage Failover Execution DAG

When an authorized two-person failover is triggered, the execution workflow enforces strict dependency sequencing:

```mermaid
sequenceDiagram
    autonumber
    actor Operator as Lead SRE (Initiator)
    actor Approver as Director (Approver)
    participant API as DR Control API
    participant WF as Failover Workflow DAG
    participant Fence as Fencing Authority
    participant DB as Cloud SQL / AlloyDB Adapter
    participant Cluster as GKE Cluster Adapter
    participant DNS as Cloud DNS Routing Adapter

    Operator->>API: POST /api/v1/dr/failovers (Initiate)
    API-->>Operator: 202 Accepted (Pending Secondary Approval)
    Approver->>API: POST /api/v1/dr/failovers/{op}:approve
    API->>WF: ExecuteFailoverWorkflow(Plan)
    
    rect rgb(240, 220, 220)
        Note over WF,Fence: Stage 1: Fencing (DR-INV-01)
        WF->>Fence: Revoke Old Primary IAM / Network Paths
        Fence-->>WF: Increment Fencing Epoch -> Recorded
    end

    rect rgb(220, 240, 220)
        Note over WF,DB: Stage 2: Database Promotion
        WF->>DB: Initiate Switchover / Promote Replica
        DB-->>WF: Replica Promoted to Primary
    end

    rect rgb(220, 220, 240)
        Note over WF,Cluster: Stage 3 & 4: Scaling & Workload Validation
        WF->>Cluster: Activate Reserved Node Pools (DR-INV-05)
        Cluster-->>WF: Pods Ready & Posture Conforming (DR-INV-06)
    end

    rect rgb(240, 240, 220)
        Note over WF,DNS: Stage 5: Traffic Routing Lock (DR-INV-02)
        WF->>DNS: Switch Traffic to DR Region VIP
        WF->>DNS: Lock Routing Policy (Prevent Auto-Failback)
        DNS-->>WF: Routing Locked to DR
    end

    WF-->>API: Plan Status -> FAILED_OVER
    API-->>Approver: 200 OK (Failover Completed)
```

---

## 3. Two-Person Declaration & Break-Glass Gate

Stateful operations require cryptographic separation of duties:

```mermaid
flowchart TD
    A[Disaster Event Detected] --> B[Operator Submits POST /api/v1/dr/incidents:declare]
    B --> C{State = PENDING_APPROVAL}
    C -->|Single Actor| D[Rejected: Two-Person Rule Mandatory]
    C -->|Distinct Second Actor| E[POST /api/v1/dr/incidents/:id:approve]
    E --> F[Incident Status: DECLARED]
    F --> G[Failover Execution Unlocked]

    H[Emergency Access Required] --> I[POST /api/v1/dr/breakglass:activate]
    I --> J{Requester != Approver?}
    J -->|No| K[409 Conflict: Self-approval Forbidden]
    J -->|Yes| L[Issue Time-Boxed DR_EMERGENCY_OPERATOR Role]
    L --> M[Enforce Mandatory Audit Logging to Surviving Sink]
    M --> N[Auto-Expire Grant After Configured Duration]
```

