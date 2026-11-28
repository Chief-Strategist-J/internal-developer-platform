/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SQL QUERIES (V.3 role 12, Invariant D)

 1. Role:
    - Centralizes named, parameterized PostgreSQL queries for ledger, reservation, approval, and governance adapters.
 2. Invariants:
    - Zero execution logic, zero inline comments below this header.
*/
package queries

const (
	UpsertProvisioningIntentSQL = `
		INSERT INTO self_service_provisioning.provisioning_intents (
			id, public_id, tenant_id, resource_kind, target_id, attempt, status, outcome_id, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			attempt = EXCLUDED.attempt,
			status = EXCLUDED.status,
			updated_at = NOW()
	`

	UpdateProvisioningOutcomeFencedSQL = `
		UPDATE self_service_provisioning.provisioning_intents
		SET status = $3, outcome_id = $4, error_message = $5, version = version + 1, updated_at = NOW()
		WHERE id = $1 AND attempt = $2
	`

	ListProvisioningIntentsByTenantSQL = `
		SELECT id, public_id, tenant_id, resource_kind, target_id, attempt, status, outcome_id, error_message
		FROM self_service_provisioning.provisioning_intents
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	ExpireStaleReservationsSQL = `
		UPDATE self_service_provisioning.provisioning_reservations
		SET status = 'EXPIRED', updated_at = NOW()
		WHERE status = 'RESERVED' AND expires_at <= NOW()
	`

	SelectTenantCapacityCeilingSQL = `
		SELECT max_quota_units, max_ip_cidr_blocks, max_cluster_ip_util_pct
		FROM self_service_provisioning.tenant_capacity_ceilings
		WHERE tenant_id = $1
	`

	SumTenantActiveReservationsSQL = `
		SELECT COALESCE(SUM(quota_units), 0), COALESCE(SUM(ip_cidr_blocks), 0)
		FROM self_service_provisioning.provisioning_reservations
		WHERE tenant_id = $1 AND status IN ('RESERVED', 'COMMITTED')
	`

	InsertProvisioningReservationSQL = `
		INSERT INTO self_service_provisioning.provisioning_reservations (
			id, public_id, tenant_id, resource_kind, normalized_name, quota_units, ip_cidr_blocks, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'RESERVED', $8)
	`

	CommitProvisioningReservationSQL = `
		UPDATE self_service_provisioning.provisioning_reservations
		SET status = 'COMMITTED', updated_at = NOW()
		WHERE id = $1 AND status = 'RESERVED'
	`

	ReleaseProvisioningReservationSQL = `
		UPDATE self_service_provisioning.provisioning_reservations
		SET status = 'RELEASED', updated_at = NOW()
		WHERE id = $1 AND status = 'RESERVED'
	`

	UpsertTenantCapacityCeilingSQL = `
		INSERT INTO self_service_provisioning.tenant_capacity_ceilings (
			tenant_id, max_quota_units, max_ip_cidr_blocks, max_cluster_ip_util_pct
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET
			max_quota_units = EXCLUDED.max_quota_units,
			max_ip_cidr_blocks = EXCLUDED.max_ip_cidr_blocks,
			max_cluster_ip_util_pct = EXCLUDED.max_cluster_ip_util_pct,
			updated_at = NOW()
	`

	InsertProvisioningApprovalSQL = `
		INSERT INTO self_service_provisioning.provisioning_approvals (
			id, public_id, tenant_id, requester_id, approver_id, plan_hash, template_sha, policy_sha, budget_units, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	SelectApprovalForUpdateSQL = `
		SELECT id, public_id, tenant_id, requester_id, approver_id, plan_hash, template_sha, policy_sha, budget_units, status, expires_at
		FROM self_service_provisioning.provisioning_approvals
		WHERE id = $1
		FOR UPDATE
	`

	SumConsumedApprovalBudget30dSQL = `
		SELECT COALESCE(SUM(budget_units), 0)
		FROM self_service_provisioning.provisioning_approvals
		WHERE tenant_id = $1 AND status = 'CONSUMED' AND created_at >= NOW() - INTERVAL '30 days'
	`

	ConsumeProvisioningApprovalSQL = `
		UPDATE self_service_provisioning.provisioning_approvals
		SET status = 'CONSUMED', updated_at = NOW()
		WHERE id = $1 AND status = 'APPROVED'
	`

	UpdateIntentOperationIDSQL = `
		UPDATE self_service_provisioning.provisioning_intents
		SET operation_id = $2, updated_at = NOW()
		WHERE id = $1
	`

	InsertRetentionDecisionSQL = `
		INSERT INTO self_service_provisioning.retention_decisions (
			id, tenant_id, resource_id, resource_class, decision, decided_by
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`

	CountRetentionDecisionSQL = `
		SELECT COUNT(1)
		FROM self_service_provisioning.retention_decisions
		WHERE tenant_id = $1 AND resource_id = $2
	`

	UpsertDNSCooldownSQL = `
		INSERT INTO self_service_provisioning.dns_cooldowns (dns_name, tenant_id, cooldown_until)
		VALUES ($1, $2, $3)
		ON CONFLICT (dns_name) DO UPDATE SET cooldown_until = EXCLUDED.cooldown_until, released_at = NOW()
	`

	CountActiveDNSCooldownSQL = `
		SELECT COUNT(1)
		FROM self_service_provisioning.dns_cooldowns
		WHERE dns_name = $1 AND cooldown_until > NOW()
	`

	AcquireSagaLockSQL = `
		INSERT INTO self_service_provisioning.saga_locks (service_key, tenant_id, active_operation, state)
		VALUES ($1, $2, $3, 'RUNNING')
		ON CONFLICT (service_key) DO UPDATE SET
			active_operation = EXCLUDED.active_operation,
			state = 'RUNNING',
			updated_at = NOW()
		WHERE self_service_provisioning.saga_locks.state = 'COMPLETED'
	`

	ReleaseSagaLockSQL = `
		UPDATE self_service_provisioning.saga_locks
		SET state = $2, updated_at = NOW()
		WHERE service_key = $1
	`

	SweepUntrackedIntentsSQL = `
		SELECT id FROM self_service_provisioning.provisioning_intents
		WHERE status = 'PENDING' AND outcome_id = ''
	`

	SweepDuplicateReadyTargetsSQL = `
		SELECT target_id FROM self_service_provisioning.provisioning_intents
		WHERE status = 'READY'
		GROUP BY resource_kind, target_id
		HAVING COUNT(1) > 1
	`
)
