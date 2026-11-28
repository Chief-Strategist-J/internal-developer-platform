# Failure Cases & Production Runbooks — Deployment Strategies

## Edge Cases Matrix (`EC-01` – `EC-12`)
| ID | Scenario | Enforcement / Handling | Verification / Test |
|---|---|---|---|
| `EC-01` | Prometheus unreachable during active analysis | Fail-closed: pause step progression; if unreachable > `cfg.analysis.timeout`, abort rollout to 0% | Simulate Prometheus connection drop |
| `EC-02` | Canary pod CrashLoopBackOff during ramp | Pod readiness gate detects crash; abort triggered before any mesh traffic shifts to canary | Deploy crashing image tag |
| `EC-03` | Traffic weight desync between mesh and ledger | Distributed reconciler verifies Istio VirtualService weights against ledger state; auto-heals drift | Modify VirtualService directly via kubectl |
| `EC-04` | Database connection lost during step advance | Step advance transaction fails atomically; reconciler retries step advance using optimistic locking | Kill DB session mid-step |
| `EC-05` | Concurrent rollback requested via UI | Manual abort immediately overrides active step, forces state `ABORTING`, shifts mesh weight to 0% | Trigger manual abort while analyzing |
| `EC-06` | Low traffic volume produces inconclusive sample | Adaptive window: if request count < `cfg.analysis.min_samples`, extend analysis window up to ceiling | Inject low QPS traffic profile |
| `EC-07` | Stable replica set unhealthy during cutover | Promotion gate holds; canary pods maintained at 100% until new stable replicas pass readiness checks | Block stable pods readiness |
| `EC-08` | Blue/Green preview service exposes internal route | Ingress admission rejects unauthenticated public exposure of preview service endpoints | Submit invalid preview ingress spec |
| `EC-09` | Pod disruption budget blocks canary scale down | Drain controller respects PDB and gracefully evicts canary pods with timeout | Deploy restrictive PDB |
| `EC-10` | Envoy proxy configuration timeout | Health check verifies Envoy route synchronization before shifting traffic weight | Simulate Envoy xDS propagation delay |
| `EC-11` | Mid-rollout configuration hash update | Invalidate active execution; require graceful abort and new execution initialized with updated spec hash | Push new git commit mid-rollout |
| `EC-12` | Worker crash during traffic shifting step | Fenced lease lock: standby worker detects stale lease and resumes step reconciliation | Send SIGKILL to active worker |

---

## Production Operating Runbooks

### <a id="runbook-canary-pretraffic-failure"></a> Runbook 1: Invariant P0-03 Pre-Traffic Health Check Breach
- **Alert Trigger:** `CanaryPreTrafficHealthFailure` (Severity: `WARNING`, Team: `Workload-Owners`)
- **Description:** Canary pods failed readiness or startup probes within `cfg.pod_ready_timeout`. No production traffic was exposed.
- **Immediate Mitigation Procedure:**
  1. Inspect pod startup logs and events:
     ```bash
     kubectl describe pods -l app=<workload>,role=canary -n <namespace>
     kubectl logs -l app=<workload>,role=canary -n <namespace> --tail=100
     ```
  2. Verify if failure was due to application crash, missing secret, or database migration issue.
  3. The rollout engine automatically reaps canary pods and transitions execution to `ABORTED`.
  4. Workload owner fixes bug and triggers a new rollout.

---

### <a id="runbook-canary-metric-abort"></a> Runbook 2: Automated Abort on Metric SLO Breach (`P0-04`)
- **Alert Trigger:** `CanaryRolloutAbortedOnMetricBreach` (Severity: `PAGE`, Team: `Workload-Owners`)
- **Description:** Real-time Prometheus analysis detected error rate or latency breaches exceeding SLO ceilings during an active canary step. Mesh traffic was immediately reverted to 0%.
- **Immediate Mitigation Procedure:**
  1. Inspect the recorded analysis breach metrics in the ledger:
     ```sql
     SELECT metric_name, query, measured_value, threshold_value, verdict, created_at 
     FROM deployment_strategies.rollout_analysis_runs 
     WHERE execution_id = '<execution_id>' AND verdict = 'FAIL';
     ```
  2. Verify mesh traffic weight is 0% on canary:
     ```bash
     kubectl get virtualservice <workload>-vs -n <namespace> -o yaml
     ```
  3. Confirm stable revision is handling 100% of user traffic without error elevation.
  4. File incident report with root cause analysis.

---

### <a id="runbook-mesh-weight-desync"></a> Runbook 3: Service Mesh Weight Desynchronization (`EC-03`)
- **Alert Trigger:** `RolloutMeshWeightDesynchronized` (Severity: `PAGE`, Team: `Platform`)
- **Description:** The Istio VirtualService or Gateway API HTTPRoute traffic weight does not match the active ledger state.
- **Immediate Mitigation Procedure:**
  1. Run reconciler drift inspection:
     ```bash
     python3 /home/btpl-lap-22/live/System-Architect/llm-obs-policies/policy-orchestrator/src/api/cli/main.py task --phase status
     ```
  2. If automated reconciliation fails, manually align VirtualService to safe state (0% canary, 100% stable).
