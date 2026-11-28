# Architecture Decision Records (ADRs) — Autoscaling (Phase 2 Data & Ingestion)

## ADR-001: Database Connection Ceiling Multiplier (`AS-INV-16`)
- **Status:** Accepted & Mechanized
- **Context:**
  Workloads scaling horizontally in Kubernetes can scale out in seconds. Because PostgreSQL provisions a dedicated operating system process per connection, unrestricted pod scale-out multiplies active connections: $\sum (maxReplicas \times perPodPoolSize)$. Without a strict connection budget ceiling, a horizontal pod scale-up event rapidly exhausts database connection pools, dropping traffic across all other workloads sharing the database.
- **Decision:**
  Enforce a hard mathematical connection ceiling gate at admission:
  1. Every autoscaled workload connecting to a database MUST declare its per-pod connection pool size.
  2. The admission gate computes:
     $$\text{Total Potential Connections} = maxReplicas \times perPodPoolSize \le DB_{connection\_budget}$$
  3. If total potential connections exceed the database connection budget (or managed pooler max client connections), the scaling plan is rejected at admission (`PAGE`).
- **Consequences:**
  - Prevents database connection exhaustion during peak traffic events.
  - Forces teams to size in-app pools conservatively or adopt connection pooling.

---

## ADR-002: Ingestion Consumer Partition Capping & Dual-Metric Scaling (`AS-INV-19`, `AS-INV-20`)
- **Status:** Accepted & Mechanized
- **Context:**
  Event-driven queue workers scaling on Kafka topics cannot process messages faster by adding pods beyond the topic partition count, because extra consumers in a consumer group remain idle. Concurrently, Pub/Sub metrics have an inherent 60–120s reporting latency, which causes reactive scale-up delays and cold-start message starvation if workers scale to zero.
- **Decision:**
  1. **Kafka Partition Bounding (`AS-INV-19`)**:
     For Kafka-backed queue workers, KEDA `maxReplicas` is strictly clamped:
     $$maxReplicas \le topic.partitionCount$$
     Plans proposing $maxReplicas > partitionCount$ are rejected.
  2. **Pub/Sub Dual-Metric & Floor Rule (`AS-INV-20`)**:
     Pub/Sub workers in production must enforce $minReplicas \ge 1$ and scale on dual metrics (backlog / `oldest_unacked_message_age` + CPU utilization) with client-side flow control.
- **Consequences:**
  - Prevents useless idle replica churn on Kafka.
  - Eliminates cold-start dropouts and buffer bloat on Pub/Sub.

---

## ADR-003: Governed Database Vertical Scaling in Planned Maintenance Windows (`AS-INV-17`, `AS-INV-18`)
- **Status:** Accepted & Mechanized
- **Context:**
  Unattended database autoscaling in production causes split-brain restarts, query aborts, and unplanned budget overruns. Unbounded storage autoscaling risks storage budget exhaustion.
- **Decision:**
  1. Production database CPU and memory scaling is strictly prohibited from running automatically. Changes must be executed as planned maintenance requests with operator approvals.
  2. Storage autoscaling must declare an explicit `increase_limit > 0`. Unlimited growth (`increase_limit = 0`) is rejected at admission (`AS-INV-18`).
- **Consequences:**
  - Predictable maintenance windows and zero un-audited database restarts.

---

## ADR-004: Transaction-Mode Connection Pooling Compatibility (`AS-INV-21`)
- **Status:** Accepted & Mechanized
- **Context:**
  Cloud SQL Managed Connection Pooling (pgbouncer) operates in transaction mode by default. Transaction mode breaks applications relying on session state (`LISTEN/NOTIFY`, `SET/RESET`, prepared statements, temporary tables, or `WITH HOLD` cursors).
- **Decision:**
  Admission checks verify application session requirements before assigning transaction-mode poolers. Workloads requiring session state must use session-mode pooling or private dedicated pools.
- **Consequences:**
  - Prevents silent runtime failures caused by dropped session state in connection proxies.
