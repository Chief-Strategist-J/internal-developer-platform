# Failure Cases & Production Runbooks — Autoscaling (Phase 2 Data & Ingestion)

## Edge Cases Matrix (`EC-01` – `EC-10`)
| ID | Scenario | Enforcement / Handling | Verification / Test |
|---|---|---|---|
| `EC-01` | Pod scale-out saturates PostgreSQL connections | Admission calculates $maxReplicas \times poolSize \le DB_{budget}$ (`AS-INV-16`) | Scaling plan with excessive maxReplicas |
| `EC-02` | KEDA scales Kafka consumers past partition count | Bounded by partition metadata: $maxReplicas \le partitions$ (`AS-INV-19`) | ScaledObject with maxReplicas > partitions |
| `EC-03` | Pub/Sub metrics delayed during sudden queue surge | Dual-metric scaling: fall back to CPU rate while backlog metric catches up (`AS-INV-20`) | Simulate metric ingestion delay |
| `EC-04` | Transaction pooler breaks LISTEN/NOTIFY consumer | Rejection at admission if session-dependent features are declared (`AS-INV-21`) | Submit plan requiring LISTEN with tx pooling |
| `EC-05` | Unattended automatic DB resize restarts database | Reject automated vertical DB resize in prod; route to scheduled window (`AS-INV-17`) | Trigger automated Cloud SQL resize |
| `EC-06` | Cloud SQL storage auto-growth exhausts budget | Mandatory increase limit check: `increase_limit > 0` (`AS-INV-18`) | Instance with unlimited growth limit |
| `EC-07` | Metrics API downtime locks HPA scale-down | Freeze scale-down; maintain current capacity; page on-call (`AS-INV-04`) | Drop metrics-server API |
| `EC-08` | Consumer scaling flapping due to small batch sizes | 5-minute scale-down stabilization window with 0.1 tolerance clamp | High-frequency burst traffic profile |
| `EC-09` | Spot VM preemption hits queue workers | Spot allowed only for tolerant worker tiers; graceful drain within 30s | Inject node preemption signal |
| `EC-10` | Database failover reconnect storm | Exponential backoff with jitter on pool reconnection; cap surge | Simulate Cloud SQL replica failover |

---

## Production Operating Runbooks

### <a id="runbook-db-connection-exhaustion"></a> Runbook 1: Database Connection Exhaustion Under Scale-Out
- **Alert Trigger:** `DatabaseConnectionsExhaustionApproaching` (Severity: `PAGE`, Team: `Platform & Workload-Owners`)
- **Description:** Active connections to PostgreSQL instance exceed `cfg.db.conn_page` (e.g. 85% of max connections) due to pod horizontal scale-out.
- **Immediate Mitigation Procedure:**
  1. Inspect active database connection count per client IP:
     ```sql
     SELECT client_addr, count(*) 
     FROM pg_stat_activity 
     GROUP BY client_addr 
     ORDER BY count(*) DESC;
     ```
  2. Identify the scaling deployment causing the surge:
     ```bash
     kubectl get hpa -A --sort-by=.status.currentReplicas
     ```
  3. Clamp `maxReplicas` temporarily on offending workload:
     ```bash
     kubectl patch hpa <workload-hpa> -n <namespace> -p '{"spec":{"maxReplicas": <clamped_value>}}'
     ```
  4. Enable Cloud SQL Managed Connection Pooling (transaction mode) if supported by the workload queries.

---

### <a id="runbook-kafka-partition-lag"></a> Runbook 2: Ingestion Backlog Stall Due to Kafka Partition Capping
- **Alert Trigger:** `KafkaConsumerLagCritical` (Severity: `PAGE`, Team: `Data-Platform`)
- **Description:** Consumer lag is growing on Kafka topic, but consumers cannot scale further because `currentReplicas == partitionCount`.
- **Immediate Mitigation Procedure:**
  1. Confirm consumers are saturated across all available partitions:
     ```bash
     kafka-consumer-groups --bootstrap-server <broker> --describe --group <group-id>
     ```
  2. Adding pods will NOT help because extra pods in a group sit idle.
  3. Increase topic partitions via controlled planned change:
     ```bash
     kafka-topics --bootstrap-server <broker> --alter --topic <topic-name> --partitions <new_partition_count>
     ```
  4. Update `maxReplicas` in `ScaledObject` to match the new partition count.
