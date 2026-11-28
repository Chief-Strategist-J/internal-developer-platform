# Architecture Diagrams — Autoscaling (Phase 2 Data & Ingestion)

## 1. Database Connection Ceiling & Admission Cascade Flowchart
```mermaid
flowchart TD
    subgraph PlanSubmission ["1. Scaling Plan Ingestion"]
        PLAN["Submit ScalingPlan (maxReplicas, perPodPoolSize)"]
        CEIL["Load Ceilings: Quota, IP, Node Max, DB Budget"]
    end

    subgraph AdmissionEvaluation ["2. Connection Ceiling Gate (AS-INV-16)"]
        CALC["Compute Projected = maxReplicas * perPodPoolSize"]
        CHECK{"Projected <= DB Connection Budget?"}
        POOLER{"Managed Pooler Active?"}
        TXMODE{"Session State Required? (AS-INV-21)"}
    end

    subgraph Decision ["3. Admission Verdict"]
        ALLOW["Plan APPROVED & Applied"]
        REJECT["REJECT 422: Connection Ceiling Exceeded"]
        WARN["REJECT 422: Transaction Pooler Incompatible"]
    end

    PLAN --> CEIL
    CEIL --> CALC
    CALC --> CHECK
    CHECK -->|Yes| POOLER
    CHECK -->|No| REJECT

    POOLER -->|Yes| TXMODE
    POOLER -->|No Direct DB| ALLOW
    TXMODE -->|No Session Features| ALLOW
    TXMODE -->|Requires LISTEN/NOTIFY| WARN
```

---

## 2. Kafka Consumer Partition Capping Sequence Diagram
```mermaid
sequenceDiagram
    autonumber
    actor Caller as Service Team
    participant Adm as AutoscalingAdmission
    participant KEDA as KEDA Controller
    participant Kafka as Kafka Cluster (Topic Metadata)

    Caller->>Adm: Submit ScaledObject(topic="orders", maxReplicas=32)
    Adm->>Kafka: GetTopicPartitions("orders")
    Kafka-->>Adm: Partitions = 16
    Note over Adm: Evaluate AS-INV-19: maxReplicas (32) > Partitions (16)
    Adm-->>Caller: REJECT 422: maxReplicas exceeds partition count (16)

    Caller->>Adm: Resubmit ScaledObject(topic="orders", maxReplicas=16)
    Adm->>Kafka: GetTopicPartitions("orders")
    Kafka-->>Adm: Partitions = 16
    Adm->>KEDA: Apply ScaledObject
    KEDA-->>Caller: ScaledObject Active (Max 16 Pods)
```

---

## 3. Pub/Sub Dual-Metric Scaling & Floor State Machine
```mermaid
stateDiagram-v2
    [*] --> MIN_FLOOR: Deploy Pub/Sub Consumer (minReplicas >= 1)
    MIN_FLOOR --> BACKLOG_SURGE: Backlog > Threshold OR Oldest Msg Age > 30s
    BACKLOG_SURGE --> CPU_STABILIZATION: Replicas Scale Out (Rate Clamped)
    CPU_STABILIZATION --> BACKLOG_SURGE: Backlog Persists
    CPU_STABILIZATION --> SCALE_DOWN_COOLING: Backlog Drained (5-min Stabilize)
    SCALE_DOWN_COOLING --> MIN_FLOOR: Replicas Return to minReplicas (Never 0 in Prod)
```
