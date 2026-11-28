# Architecture Diagrams — Deployment Strategies

## 1. End-to-End Progressive Delivery & Traffic Shifting Flowchart
```mermaid
flowchart TD
    subgraph Ingress ["1. Ingress & Triggers"]
        REQ["Inbound Rollout Trigger (POST /api/v1/rollouts)"]
        IDEMP{"Idempotency Lock"}
        AUTH{"RBAC & Tenant Verification"}
    end

    subgraph Ledger ["2. Durable Ledger & Validation"]
        PLAN["Load & Validate RolloutPlan"]
        INIT["Persist RolloutExecution (State: INITIALIZING)"]
        EXPAND{"Expand/Contract DB Check"}
    end

    subgraph RolloutLoop ["3. Progressive Canary Step Loop"]
        PODS["Scale Canary Pod Replicas"]
        READYGATE{"100% Pods Ready & mTLS Verified (P0-03)"}
        SHIFT["Shift Mesh Weight to W_k (State: ANALYZING)"]
        PROM["Query Prometheus Analysis Gate"]
        METRICGATE{"Error Rate & Latency SLO Pass?"}
        PAUSE["Wait Step Pause Duration"]
        NEXTSTEP{"More Steps Remaining?"}
    end

    subgraph Completion ["4. Promotion & Cutover"]
        PROMOTE["Update Stable Deployment Spec (State: PROMOTING)"]
        STABLEGATE{"Stable Pods 100% Ready"}
        SHIFT100["Shift Stable Mesh Weight to 100%"]
        CLEAN["Scale Canary Pods to 0"]
        DONE["Persist State: COMPLETED"]
    end

    subgraph Abort ["5. Automated Abort & Compensation"]
        ABORT["Instant Revert Mesh Weight to 0% (State: ABORTING)"]
        CLEANABORT["Scale Canary Pods to 0"]
        ALERT["Page On-Call Tier-1 Alert"]
        FAIL["Persist State: ABORTED"]
    end

    REQ --> IDEMP
    IDEMP --> AUTH
    AUTH --> PLAN
    PLAN --> INIT
    INIT --> EXPAND
    EXPAND -->|Valid| PODS
    EXPAND -->|Incompatible| FAIL

    PODS --> READYGATE
    READYGATE -->|Pass| SHIFT
    READYGATE -->|Timeout / Fail| ABORT

    SHIFT --> PROM
    PROM --> METRICGATE
    METRICGATE -->|SLO Violated| ABORT
    METRICGATE -->|SLO Pass| PAUSE
    PAUSE --> NEXTSTEP

    NEXTSTEP -->|Yes, k = k + 1| PODS
    NEXTSTEP -->|No, 100% Reached| PROMOTE

    PROMOTE --> STABLEGATE
    STABLEGATE --> SHIFT100
    SHIFT100 --> CLEAN
    CLEAN --> DONE

    ABORT --> CLEANABORT
    CLEANABORT --> ALERT
    ALERT --> FAIL
```

---

## 2. Rollout Execution Finite State Machine (FSM)
```mermaid
stateDiagram-v2
    [*] --> INITIALIZING: Create Execution
    INITIALIZING --> ROUTING_STEP: Plan Validated & Pre-flight Passed
    INITIALIZING --> ABORTED: Pre-flight Failed / Spec Incompatible

    ROUTING_STEP --> ANALYZING: Pods 100% Ready (P0-03) & Mesh Weight Shifted
    ROUTING_STEP --> ABORTING: Pod Readiness Timeout / CrashLoop

    ANALYZING --> ROUTING_STEP: Step Passed & Next Step Exists
    ANALYZING --> PROMOTING: Final Step Passed (100% Weight)
    ANALYZING --> PAUSED: Manual Pause Triggered
    ANALYZING --> ABORTING: Metric SLO Breach / Error Threshold Exceeded

    PAUSED --> ANALYZING: Resume Triggered
    PAUSED --> ABORTING: Manual Abort Triggered

    PROMOTING --> COMPLETED: Stable Pods Verified & Canary Replicas Reaped
    PROMOTING --> ABORTING: Stable Pod Cutover Failure

    ABORTING --> ABORTED: Mesh Zeroed & Canary Replicas Scaled to 0
    ABORTED --> [*]
    COMPLETED --> [*]
```

---

## 3. Service Mesh & Pod Replicas Traffic Allocation Sequence
```mermaid
sequenceDiagram
    autonumber
    actor Caller as CD Pipeline / GitOps
    participant Svc as DeploymentStrategiesService
    participant Repo as PostgreSQL Ledger
    participant Mesh as MeshTrafficAdapter (Istio)
    participant K8s as ArgoRolloutsAdapter (k8s.io)
    participant Prom as PrometheusAnalysisAdapter

    Caller->>Svc: StartRollout(workload_id, plan_id)
    Svc->>Repo: CreateExecution(INIT)
    Svc->>K8s: ScaleCanaryReplicas(target_count)
    K8s-->>Svc: PodsReady(100%)
    
    loop For each step k in RolloutPlan
        Svc->>Mesh: SetTrafficWeight(canary_weight_k)
        Mesh-->>Svc: WeightShiftApplied(weight_k)
        Svc->>Repo: UpdateExecution(ROUTING_STEP -> ANALYZING)
        
        loop Analysis Pause Duration
            Svc->>Prom: QueryMetrics(error_rate, p99_latency)
            Prom-->>Svc: MetricResult(pass=true)
        end
    end

    Svc->>K8s: PromoteStableRevision()
    K8s-->>Svc: StablePodsReady()
    Svc->>Mesh: SetTrafficWeight(canary_weight=0, stable_weight=100)
    Svc->>K8s: ScaleCanaryReplicas(0)
    Svc->>Repo: UpdateExecution(COMPLETED)
    Svc-->>Caller: RolloutResult(COMPLETED)
```

---

## 4. Automated Metric Analysis & Abort Compensation Sequence
```mermaid
sequenceDiagram
    autonumber
    participant Svc as DeploymentStrategiesService
    participant Prom as PrometheusAnalysisAdapter
    participant Mesh as MeshTrafficAdapter (Istio)
    participant K8s as ArgoRolloutsAdapter
    participant Repo as PostgreSQL Ledger
    participant Pager as AlertNotificationPort

    Note over Svc,Prom: Active Step at 20% Canary Weight
    Svc->>Prom: QueryMetrics(error_rate, p99_latency)
    Prom-->>Svc: MetricResult(error_rate=1.8%, threshold=0.5%, verdict=FAIL)
    Note over Svc: Breach Count >= Consecutive Ceiling

    Svc->>Repo: TransitionState(ABORTING)
    Svc->>Mesh: SetTrafficWeight(canary_weight=0, stable_weight=100)
    Mesh-->>Svc: TrafficImmediatelyZeroed()
    Svc->>K8s: ScaleCanaryReplicas(0)
    K8s-->>Svc: CanaryPodsReaped()
    Svc->>Repo: RecordAnalysisRun(verdict=FAIL, reason="Error rate 1.8% > 0.5%")
    Svc->>Repo: TransitionState(ABORTED)
    Svc->>Pager: EmitTier1Page(workload_id, "Canary auto-aborted on SLO breach")
```
