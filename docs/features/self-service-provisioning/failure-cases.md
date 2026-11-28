# Failure Cases & Production Operating Model — Self-Service Provisioning

## Edge Cases Matrix (`EC-01` – `EC-15`)
| ID | Scenario | Enforcement / Handling |
| --- | --- | --- |
| `EC-01` | Timeout after successful create | Persist `operation_id` + `request_labels` in `provisioning_intents`; lookup by label before create |
| `EC-02` | Double submit / client retry | `IdempotencyMiddleware` (`M-01` step 10): replay identical payload, `422` on mismatch, `409` in-flight |
| `EC-03` | Concurrent name race | Unique partial index `ssp_res_active_name_uq` on normalized name → `409 Conflict` |
| `EC-04` | Downstream 429 rate limit | `ClassifyStepError`: honor `Retry-After`, exponential backoff, fail after `maxAttempts` |
| `EC-05` | Cloud quota exhausted mid-DAG | Retryable backoff → compensate + release reservation |
| `EC-06` | Zone capacity stockout | `SelectAlternateZoneOrFamily` tries allowed alternates before failing |
| `EC-07` | Compensation fails | Transition via `machines.ValidateTransition` to `ORPHANED_DIRTY` and page platform |
| `EC-08` | Failure after point of no return | `workflows.ResolvePointOfNoReturnAction` prefers `FORWARD_COMPLETE` when possible |
| `EC-09` | Template/policy updated mid-saga | `ValidateApprovalAndPin` rejects `PlanHash` drift |
| `EC-10` | Policy promoted to DENY mid-saga | Fail step with rule IDs, fix template, resume |
| `EC-11` | Owner group deleted mid-saga | Pause before ownership-bearing steps |
| `EC-12` | Argo CD sync fails after commit | `EvaluateObservedReady` blocks `READY`; fix-forward commit |
| `EC-13` | Decommission races provisioning | `saga_locks` serializes per `service_key` |
| `EC-14` | Ephemeral TTL with data resource | `ValidateDataSafeCompensation` blocks deletion without retention decision |
| `EC-15` | Engine / DB failover mid-saga | Persisted intents + fenced `WHERE attempt = :mine` prevent duplicates |

## Section C: Production Operating Model
- **SLOs:** Request → `READY` p95 latency, `ORPHANED_DIRTY` count (`0`), duplicate targets (`0`), untracked intents (`0`), dangling DNS (`0`).
- **Sweepers:** `RunSweeper` detects untracked `PENDING` intents and duplicate `READY` targets.
