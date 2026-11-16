package queries

const (
	FLOW_INSERT_WORKLOAD_PROVISION = `
		INSERT INTO workload_provisions (
			id, tenant_id, service_id, namespace, workload_name,
			cpu_request, cpu_limit, memory_request, memory_limit,
			replicas, status, idempotency_key, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	FLOW_SELECT_WORKLOAD_BY_ID = `
		SELECT id, tenant_id, service_id, namespace, workload_name,
		       cpu_request, cpu_limit, memory_request, memory_limit,
		       replicas, status, idempotency_key, version, created_at, updated_at
		FROM workload_provisions
		WHERE tenant_id = $1 AND id = $2
	`

	FLOW_SELECT_WORKLOAD_BY_IDEMPOTENCY = `
		SELECT id, tenant_id, service_id, namespace, workload_name,
		       cpu_request, cpu_limit, memory_request, memory_limit,
		       replicas, status, idempotency_key, version, created_at, updated_at
		FROM workload_provisions
		WHERE tenant_id = $1 AND idempotency_key = $2
	`

	FLOW_LIST_WORKLOAD_PROVISIONS = `
		SELECT id, tenant_id, service_id, namespace, workload_name,
		       cpu_request, cpu_limit, memory_request, memory_limit,
		       replicas, status, idempotency_key, version, created_at, updated_at
		FROM workload_provisions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	FLOW_UPDATE_WORKLOAD_PROVISION = `
		UPDATE workload_provisions
		SET status = $1, version = version + 1, updated_at = $2
		WHERE tenant_id = $3 AND id = $4 AND version = $5
	`
)

