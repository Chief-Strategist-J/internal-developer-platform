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

---

### <a id="runbook-capped-scaling"></a> Runbook 3: Capped Scaling (desired replicas never arrive)
- **Signal:** HPA desired replicas differ from current replicas; `ReplicaFailure` events; Pending pods. Tier-critical workloads page; others ticket (`AS-INV-03`, `AS-INV-10`).
- **Find the cap, in order:**
  1. Namespace quota: `kubectl describe resourcequota -n <namespace>` (pods, requests.cpu, requests.memory).
  2. Pod IP range: `kubectl get nodes -o custom-columns=NAME:.metadata.name,PODCIDR:.spec.podCIDR` and compare node pool max x per-node range with the pod range.
  3. Node pool maximum: `kubectl get events -A --field-selector reason=NotTriggerScaleUp`.
  4. Budget cap and database connection ceiling: compare `maxReplicas x perPodPoolSize` with the database connection budget (`AS-INV-16`).
- **Mitigate:** raise the limiting ceiling through a provisioning request with approval, or shed lower tiers to free capacity. Never raise quota ad hoc without the budget approval.
- **Verify:** `kubectl get hpa -n <namespace>` shows desired equals current; re-run `POST /api/v1/autoscaling/plans:evaluate` with the new ceilings and expect `allowed=true`.

---

### <a id="runbook-stockout"></a> Runbook 4: Stockout (Pending pods, no nodes arrive)
- **Signal:** tier-critical pods Pending beyond `cfg.pending.page`; `NotTriggerScaleUp` events with a zone resource exhaustion reason (`AS-INV-09`).
- **Procedure:**
  1. Confirm the cause: `kubectl describe pod <pod> -n <namespace>` and the cluster autoscaler status.
  2. Let the compute class fall through to the next priority (preferred family, then standard fallback).
  3. Check reservation utilization for the tier-critical class; a missing reservation means no capacity guarantee.
  4. If still Pending, shed lower tiers to free nodes.
- **Verify:** Pending count returns to zero and the tier-critical HPA reaches its desired replicas.

---

### <a id="runbook-metrics-outage"></a> Runbook 5: Metrics Outage (signal missing, never read as idle)
- **Signal:** `FailedGetResourceMetric` or `FailedGetExternalMetric` HPA conditions (`AS-INV-04`). While a metric is unavailable the HPA skips scale-down and can still scale up, so capacity and cost ratchet upward.
- **Procedure:**
  1. Scale-down is already frozen by the HPA; do not force it. `POST /api/v1/autoscaling/plans:evaluate` rejects a proposed scale-down while the signal is `UNAVAILABLE` (`SCALEDOWN_ON_UNAVAILABLE_SIGNAL`).
  2. Restore the metrics adapter: `kubectl -n kube-system get pods -l k8s-app=metrics-server` and its logs.
  3. Verify HPA conditions recover: `kubectl describe hpa <name> -n <namespace>`.
  4. Hold manual capacity for tier-critical services until the signal returns.

---

### <a id="runbook-flapping"></a> Runbook 6: Replica Flapping (two writers)
- **Signal:** desired replicas oscillate; Argo CD shows out-of-sync on `spec.replicas` (`AS-INV-01`). Page in production.
- **Procedure:**
  1. Find the second writer: Argo CD diff on replicas, a KEDA `ScaledObject` plus an HPA on one target, or a manual `kubectl scale`.
  2. Remove it. Replicas must not be committed in Git while an autoscaler owns them; Argo CD ignores `spec.replicas`.
  3. Freeze sync on the application until one writer remains.
- **Verify:** replicas are stable for a full stabilization window; the plan evaluates with no `MULTIPLE_REPLICA_WRITERS` or `REPLICAS_PINNED_IN_GIT`.

---

### <a id="runbook-mass-preemption"></a> Runbook 7: Mass Spot Preemption
- **Signal:** many nodes preempted at once; tier-critical pods on Spot nodes must be zero (`AS-INV-07`, `AS-INV-08`).
- **Procedure:**
  1. Confirm only tolerant classes were affected: list pods on Spot nodes by workload class.
  2. Watch the compute class fall back to standard nodes; Spot gives 30 seconds notice (15 seconds for ordinary pods by default) and PodDisruptionBudgets do not apply to preemption.
  3. If a tier-critical pod ran on Spot, page: move it through the compute class and fix the placement (`TIER_CRITICAL_ON_SPOT`).
  4. Review the Spot share and shutdown tests for the affected classes (`SHUTDOWN_EXCEEDS_SPOT_NOTICE`).

---

## Alert Coverage

Defined in `deploy/k8s/base/observability/prometheus/rules/autoscaling.rules.yaml`: `DatabaseConnectionsExhaustionApproaching` (Runbook 1), `KafkaConsumerLagCritical` (Runbook 2) and `AutoscalingThresholdConfigMissing`. Runbooks 3 to 7 are operator procedures; their alerts need platform-owned `cfg.*` values and are not defined yet.
