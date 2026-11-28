# Architecture Diagrams — Self-Service Provisioning

## 1. End-to-End Self-Service Provisioning Saga Flowchart
```mermaid
flowchart TD
    subgraph Ingress ["1. Ingress & Gateways"]
        REQ["Client POST /api/v1/clusters or /workloads"]
        IDEMP{"Redis Idempotency Lock (EC-02)"}
        AUTH{"RBAC & Tenant Token (INV-14)"}
    end

    subgraph Governance ["2. Governance & Planning"]
        APP{"Approval & Plan Pin (INV-09, INV-10)"}
        RES{"Atomic Reservation Tx (INV-04, INV-05, INV-15)"}
        INT["Persist Intent Row (INV-01)"]
    end

    subgraph Execution ["3. Orchestration & Delivery"]
        FENCE{"Fenced Attempt CAS (INV-03)"}
        LOOKUP{"Label Lookup (INV-02, EC-01)"}
        CREATE["Execute Downstream Effect"]
        SCAN["Secret Scanner Gate (INV-13)"]
        GITOPS["GitOps Commit & PR (INV-08)"]
        ARGO["Argo CD Sync Engine"]
        READY{"Observed Ready Gate (INV-12)"}
    end

    subgraph Compensation ["4. Compensation & Incident Recovery"]
        COMP["Validate Retention Decision (INV-06)"]
        TEARDOWN["Teardown DAG (DNS -> LB -> Compute) (INV-07)"]
        DIRTY["Transition to ORPHANED_DIRTY (EC-07)"]
        PAGE["PAGE Platform Tier-1 Alert"]
    end

    REQ --> IDEMP
    IDEMP -->|New Key| AUTH
    IDEMP -->|Duplicate Key| REPLAY["Deterministic Response Replay"]
    AUTH --> APP
    APP -->|Approved & Hash Valid| RES
    APP -->|Hash Mismatch| PLAN_ERR["Reject 422: Plan Hash Drift"]
    RES -->|Capacity Available| INT
    RES -->|Exhausted / Conflict| RES_ERR["Reject 409: Quota / Name Race"]
    INT --> FENCE
    FENCE -->|Attempt Valid| LOOKUP
    FENCE -->|Stale Attempt| FENCE_ERR["Reject: Fencing Token Expired"]
    LOOKUP -->|Resource Exists| ADOPT["Adopt Existing Resource ID"]
    LOOKUP -->|Not Found| CREATE
    CREATE --> SCAN
    ADOPT --> SCAN
    SCAN -->|Pass| GITOPS
    SCAN -->|Violation| SEC_PAGE["PAGE Security: Secrets in Repo"]
    GITOPS --> ARGO
    ARGO --> READY
    READY -->|Healthy Replicas Observed| SUCCESS["Terminal State: READY"]
    READY -->|Sync / Health Timeout| COMP
    CREATE -->|Fatal / Downstream Error| COMP
    COMP -->|Data Resource + Abandoned| TEARDOWN
    COMP -->|Data Resource + No Retention| DIRTY
    TEARDOWN --> COMP_DONE["Terminal State: COMPENSATED"]
    DIRTY --> PAGE
```

---

## 2. Distributed Sweeper Daemon & Leader Lease Architecture (Section C)
```mermaid
sequenceDiagram
    autonumber
    participant App as Platform API Replica
    participant Redis as Distributed Redis Lease
    participant DB as PostgreSQL Ledger
    participant Cron as K8s Daily Sweeper CronJob
    participant Prom as Prometheus Alerts

    Note over App,Redis: Hourly Reconciler Loop (kernel.Lifecycle)
    App->>Redis: SET ssp:lease:sweeper <pod_id> NX EX 300s
    alt Lease Acquired (Leader)
        Redis-->>App: OK (Leader Elected)
        App->>DB: FlowSelectExpiredDNSCooldowns()
        DB-->>App: Expired DNS Cooldown Rows
        App->>DB: FlowPruneExpiredDNSCooldowns()
        App->>DB: FlowSelectOrphanedIntents()
        DB-->>App: Untracked / Stalled Intent Rows
        alt Orphaned Dirty Detected
            App->>Prom: Increment ssp_sweeper_orphaned_dirty_count
            Prom-->>App: Trigger SSPOrphanedDirtyDetected (PAGE)
        end
        App->>DB: Scan for Duplicate Ready Targets
        alt Duplicates Found
            App->>Prom: Increment ssp_sweeper_duplicates_count
            Prom-->>App: Trigger SSPDuplicateResourceDetected (PAGE)
        end
    else Lease Denied (Follower)
        Redis-->>App: Nil (Another pod is active leader)
        Note over App: Sleep until next sweep interval
    end

    Note over Cron,App: Daily 03:00 UTC Declarative CronJob
    Cron->>App: GET http://platform-api:8080/readyz
    App-->>Cron: 200 OK (Platform API and Sweeper Active)
```

---

## 3. Finite State Machine & Lifecycle Transitions
```mermaid
stateDiagram-v2
    [*] --> RESERVED: Atomically Reserve Name & Quota (INV-04)
    RESERVED --> APPROVED: Bind Approval SHA-256 (INV-09, INV-10)
    RESERVED --> REJECTED: Quota Exhausted / Name Race (EC-03)
    APPROVED --> EXECUTING: Persist Intent Row (INV-01)
    EXECUTING --> EXECUTING: Retry Step with Bounded Exponential Backoff (INV-11)
    EXECUTING --> READY: First Deploy Observed Healthy (INV-12)
    EXECUTING --> COMPENSATING: Downstream Failure / Timeout
    COMPENSATING --> COMPENSATED: Teardown DAG Completed (INV-07)
    COMPENSATING --> ORPHANED_DIRTY: Compensation Blocked / Protection (INV-06, EC-07)
    ORPHANED_DIRTY --> COMPENSATED: Manual 2-Person Completion Runbook
    READY --> [*]
    COMPENSATED --> [*]
    REJECTED --> [*]
```

---

## 4. Teardown DAG Strict Ordering (`INV-07`)
```mermaid
flowchart LR
    D1["1. Delete Public DNS Record"] --> D2["2. Enter Quarantine Cooldown (dns_cooldowns)"]
    D2 --> D3["3. Delete Ingress / Load Balancer VIP"]
    D3 --> D4["4. Release Cloud Infrastructure & Compute"]
```

---

## 5. Observability & Alert Routing Topology (Section C)
```mermaid
flowchart TD
    subgraph Exporters ["Metrics Sources"]
        APP["Self-Service Provisioning Service"]
        SWEEP["Sweeper Daemon (kernel.Lifecycle)"]
        MW["Idempotency Middleware"]
    end

    subgraph Prometheus ["Prometheus Evaluation Engine"]
        SRV["Prometheus v2.54.1"]
        RULES["rules-configmap (/etc/prometheus/rules/*.yaml)"]
    end

    subgraph Alerts ["Alerting Severities"]
        PAGE["PAGE Tier-1 (PagerDuty / OpsGenie)"]
        TICKET["TICKET (Jira / Linear Platform Queue)"]
    end

    subgraph Dashboards ["Grafana SLI / SLO Models"]
        GRAF["Grafana v13.3.1 (/var/lib/grafana/dashboards)"]
        D1["Incident Overview (Zero-Tolerance)"]
        D2["Saga Funnel & Fencing"]
        D3["Downstream 429/403 Rates & Latency"]
        D4["Capacity & IP Quota Ceilings"]
    end

    APP -->|OTel Traces & Metrics| SRV
    SWEEP -->|Sweeper Counters| SRV
    MW -->|Dedup & Latency Metrics| SRV
    RULES -.-> SRV

    SRV -->|ORPHANED_DIRTY > 0| PAGE
    SRV -->|Duplicates > 0| PAGE
    SRV -->|Saga Stall > 30m| PAGE
    SRV -->|IP Space > 90%| PAGE
    SRV -->|Data Deletion w/o Decision| PAGE
    SRV -->|Dangling DNS Detected| PAGE
    SRV -->|Secrets in Repo| PAGE

    SRV -->|Tenant Quota Drift > 85%| TICKET
    SRV -->|Ready w/o Replicas| TICKET

    SRV --> GRAF
    GRAF --> D1
    GRAF --> D2
    GRAF --> D3
    GRAF --> D4
```
