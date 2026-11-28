# Failure Cases & Production Runbooks — Self-Service Provisioning

## Edge Cases Matrix (`EC-01` – `EC-15`)
| ID | Scenario | Enforcement / Handling | Verification / Test |
| --- | --- | --- | --- |
| `EC-01` | Timeout after successful create | Persist `operation_id` + `request_labels` in `provisioning_intents`; lookup by label before create | Inject timeout after 200 OK |
| `EC-02` | Double submit / client retry | `IdempotencyMiddleware`: replay identical payload, `422` on mismatch, `409` in-flight | Concurrent identical POSTs |
| `EC-03` | Concurrent name race | Unique partial index `ssp_res_active_name_uq` on normalized name → `409 Conflict` | Parallel reservations |
| `EC-04` | Downstream 429 rate limit | `ClassifyStepError`: honor `Retry-After`, exponential backoff, fail after `maxAttempts` | Sustained 429 injection |
| `EC-05` | Cloud quota exhausted mid-DAG | Retryable backoff → compensate + release reservation | Shrink quota between reserve & exec |
| `EC-06` | Zone capacity stockout | `SelectAlternateZoneOrFamily` tries allowed alternates before failing | Simulate stockout error |
| `EC-07` | Compensation fails | Transition via `machines.ValidateTransition` to `ORPHANED_DIRTY` and page platform | Enable deletion protection |
| `EC-08` | Failure after point of no return | `workflows.ResolvePointOfNoReturnAction` prefers `FORWARD_COMPLETE` when possible | Inject step failure after repo push |
| `EC-09` | Template/policy updated mid-saga | `ValidateApprovalAndPin` rejects `PlanHash` drift | Publish v+1 during active saga |
| `EC-10` | Policy promoted to DENY mid-saga | Fail step with rule IDs, fix template, resume | Tighten rule during saga |
| `EC-11` | Owner group deleted mid-saga | Pause before ownership-bearing steps | Delete group in test IdP |
| `EC-12` | Argo CD sync fails after commit | `EvaluateObservedReady` blocks `READY`; fix-forward commit | Manifest sync failure |
| `EC-13` | Decommission races provisioning | `saga_locks` serializes per `service_key` | Concurrent decommission + resume |
| `EC-14` | Ephemeral TTL with data resource | `ValidateDataSafeCompensation` blocks deletion without retention decision | Attach DB to ephemeral env |
| `EC-15` | Engine / DB failover mid-saga | Persisted intents + fenced `WHERE attempt = :mine` prevent duplicates | Worker SIGKILL past lease |

---

## Production Operating Runbooks (Section C)

### <a id="runbook-orphaned-dirty"></a> Runbook 1: Handling `ORPHANED_DIRTY` Breaches (`INV-06`, `EC-07`)
- **Alert Trigger:** `SSPOrphanedDirtyDetected` (Severity: `PAGE`, Team: `Platform`)
- **Description:** A provisioning saga failed mid-execution and automated compensation was blocked (e.g. deletion protection enabled on database, cloud API refusal, or unrecorded retention decision).
- **Immediate Mitigation Procedure:**
  1. Inspect active orphaned records in PostgreSQL ledger:
     ```sql
     SELECT public_id, request_id, service_key, state, active_operation, error_details 
     FROM self_service_provisioning.saga_locks 
     WHERE state = 'ORPHANED_DIRTY';
     ```
  2. Inspect cloud audit logs for the resource ID referenced in `error_details`.
  3. Engage team lead for **Two-Person Manual Completion Approval**:
     - If resource contains customer data: verify data retention decision in `retention_decisions`.
     - If retention decision is `ABANDON` or `DELETE_CONFIRMED`: manually disable cloud deletion protection.
  4. Execute manual compensation or adopt resource:
     ```bash
     curl -X POST http://platform-api:8080/api/v1/provisioning/sagas/<request_id>/reconcile \
       -H "Authorization: Bearer $PLATFORM_ADMIN_TOKEN" \
       -d '{"action": "FORCE_COMPENSATE", "approver_id": "usr_lead_01"}'
     ```
  5. Confirm `ssp_sweeper_orphaned_dirty_count` returns to `0`.

---

### <a id="runbook-duplicate-resources"></a> Runbook 2: Handling Duplicate Resource Scans (`INV-02`, `EC-01`)
- **Alert Trigger:** `SSPDuplicateResourceDetected` (Severity: `PAGE`, Team: `Platform`)
- **Description:** Sweeper scan identified >1 active cloud resource mapped to the same `(request_id, step, target_type)`.
- **Immediate Mitigation Procedure:**
  1. Freeze downstream step executions by enabling step throttle:
     ```bash
     curl -X POST http://platform-api:8080/api/v1/platform/circuit-breakers/ssp-steps/freeze
     ```
  2. Query duplicate intents:
     ```sql
     SELECT request_id, step_name, count(*) 
     FROM self_service_provisioning.provisioning_intents 
     GROUP BY request_id, step_name 
     HAVING count(*) > 1;
     ```
  3. Identify the oldest created resource in cloud inventory:
     - Mark oldest resource as primary in `provisioning_intents`.
     - Quarantine and decommission extraneous duplicates.
  4. Lift step freeze once deduplication lookup is repaired.

---

### <a id="runbook-saga-stall"></a> Runbook 3: Handling Stalled Provisioning Sagas (`INV-11`, `EC-15`)
- **Alert Trigger:** `SSPSagaStallDetected` (Severity: `PAGE`, Team: `Platform`)
- **Description:** One or more active sagas have remained in `EXECUTING` or `COMPENSATING` without state transition for >30 minutes.
- **Immediate Mitigation Procedure:**
  1. Check Redis distributed leader lease status:
     ```bash
     redis-cli GET ssp:lease:sweeper
     ```
  2. Query stalled intent timestamps:
     ```sql
     SELECT public_id, request_id, step_name, attempt, created_at, updated_at 
     FROM self_service_provisioning.provisioning_intents 
     WHERE state IN ('PENDING', 'IN_FLIGHT') 
       AND updated_at < NOW() - INTERVAL '30 minutes';
     ```
  3. Verify database lock contention or downstream provider outages (Git host, Kubernetes API, Cloud IAM).
  4. Trigger saga resume via API with incremented fencing token.

---

### <a id="runbook-dangling-dns"></a> Runbook 4: Dangling DNS & Subdomain Takeover Prevention (`INV-07`)
- **Alert Trigger:** `SSPDanglingDNSDetected` (Severity: `PAGE`, Team: `Security`)
- **Description:** Decommissioned DNS record is still resolving or was reclaimed before cooldown expiration.
- **Immediate Mitigation Procedure:**
  1. Immediately delete public DNS zone record for the affected domain:
     ```bash
     curl -X DELETE http://platform-api:8080/api/v1/dns/records/<dns_id>
     ```
  2. Audit `dns_cooldowns` table to verify retention cooldown period (`72 hours` default).
  3. Inspect external web endpoints for unauthorized takeover content. Rotate active cookies and SSO sessions if domain was hijacked.

---

### <a id="runbook-tenant-quota-drift"></a> Runbook 5: Tenant Capacity Ceilings & Quota Drift (`INV-05`, `INV-15`)
- **Alert Trigger:** `SSPTenantQuotaDrift` (Severity: `TICKET`, Team: `Platform`)
- **Description:** Tenant quota units used exceed 85% of assigned ceiling.
- **Resolution Procedure:**
  1. Check current allocation in `tenant_capacity_ceilings`:
     ```sql
     SELECT tenant_id, quota_units_used, quota_units_limit, ip_cidr_allocated, ip_cidr_limit 
     FROM self_service_provisioning.tenant_capacity_ceilings 
     WHERE quota_units_used::float / quota_units_limit > 0.85;
     ```
  2. Open capacity review ticket with tenant owner.
  3. Adjust ceiling or prune decommissioned cluster namespaces.
