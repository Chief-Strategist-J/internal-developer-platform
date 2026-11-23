package queries

const (
	FLOW_INSERT_POLICY = `
INSERT INTO policies (
    id, public_id, tenant_id, source, external_id, correlation_id,
    name, description, enforcement_point, severity, lifecycle_state,
    rule_expression, exceptable, detection_pair, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
`

	FLOW_SELECT_POLICY_BY_ID = `
SELECT id, public_id, tenant_id, source, external_id, correlation_id,
       name, description, enforcement_point, severity, lifecycle_state,
       rule_expression, exceptable, detection_pair, version, created_at, updated_at, deleted_at
FROM policies
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
`

	FLOW_SELECT_POLICY_BY_PUBLIC_ID = `
SELECT id, public_id, tenant_id, source, external_id, correlation_id,
       name, description, enforcement_point, severity, lifecycle_state,
       rule_expression, exceptable, detection_pair, version, created_at, updated_at, deleted_at
FROM policies
WHERE tenant_id = $1 AND public_id = $2 AND deleted_at IS NULL
`

	FLOW_UPDATE_POLICY = `
UPDATE policies
SET name = $1, description = $2, rule_expression = $3, exceptable = $4,
    detection_pair = $5, version = version + 1, updated_at = $6
WHERE tenant_id = $7 AND id = $8 AND version = $9 AND deleted_at IS NULL
`

	FLOW_UPDATE_POLICY_LIFECYCLE = `
UPDATE policies
SET lifecycle_state = $1, version = version + 1, updated_at = $2
WHERE tenant_id = $3 AND id = $4 AND version = $5 AND deleted_at IS NULL
`

	FLOW_SOFT_DELETE_POLICY = `
UPDATE policies
SET deleted_at = $1, version = version + 1, updated_at = $1
WHERE tenant_id = $2 AND id = $3 AND version = $4 AND deleted_at IS NULL
`

	FLOW_LIST_POLICIES_KEYSET = `
SELECT id, public_id, tenant_id, source, external_id, correlation_id,
       name, description, enforcement_point, severity, lifecycle_state,
       rule_expression, exceptable, detection_pair, version, created_at, updated_at, deleted_at
FROM policies
WHERE tenant_id = $1 AND id > $2 AND deleted_at IS NULL
ORDER BY id ASC
LIMIT $3
`

	FLOW_LIST_ACTIVE_CEL_POLICIES = `
SELECT id, public_id, tenant_id, source, external_id, correlation_id,
       name, description, enforcement_point, severity, lifecycle_state,
       rule_expression, exceptable, detection_pair, version, created_at, updated_at, deleted_at
FROM policies
WHERE tenant_id = $1 AND enforcement_point = 'CEL' AND lifecycle_state IN ('AUDIT', 'WARN', 'DENY') AND deleted_at IS NULL
ORDER BY id ASC
`

	FLOW_INSERT_EXCEPTION = `
INSERT INTO policy_exceptions (
    id, public_id, tenant_id, policy_id, workload_id, namespace,
    approver_id, justification, status, expires_at, correlation_id,
    version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
`

	FLOW_SELECT_EXCEPTION_BY_ID = `
SELECT id, public_id, tenant_id, policy_id, workload_id, namespace,
       approver_id, justification, status, expires_at, correlation_id,
       version, created_at, updated_at, deleted_at
FROM policy_exceptions
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
`

	FLOW_SELECT_ACTIVE_EXCEPTIONS_FOR_WORKLOAD = `
SELECT id, public_id, tenant_id, policy_id, workload_id, namespace,
       approver_id, justification, status, expires_at, correlation_id,
       version, created_at, updated_at, deleted_at
FROM policy_exceptions
WHERE tenant_id = $1 AND namespace = $2 AND workload_id = $3 AND status = 'ACTIVE' AND expires_at > $4 AND deleted_at IS NULL
`

	FLOW_UPDATE_EXCEPTION_STATUS = `
UPDATE policy_exceptions
SET status = $1, version = version + 1, updated_at = $2
WHERE tenant_id = $3 AND id = $4 AND version = $5 AND deleted_at IS NULL
`

	FLOW_VERIFY_STATE_TRANSITION = `
SELECT 1 FROM policy_state_transitions
WHERE from_status = $1 AND to_status = $2
`

	FLOW_INSERT_VIOLATION = `
INSERT INTO policy_violations (
    id, public_id, tenant_id, policy_id, source, external_id,
    correlation_id, workload_id, namespace, severity, enforcement_point,
    status, message, detection_pair, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (tenant_id, source, external_id) DO UPDATE
SET message = EXCLUDED.message,
    severity = EXCLUDED.severity,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at,
    version = policy_violations.version + 1
`

	FLOW_SELECT_ACTIVE_VIOLATIONS_BY_POLICY = `
SELECT id, public_id, tenant_id, policy_id, source, external_id,
       correlation_id, workload_id, namespace, severity, enforcement_point,
       status, message, detection_pair, version, created_at, updated_at
FROM policy_violations
WHERE tenant_id = $1 AND policy_id = $2 AND status = 'ACTIVE'
ORDER BY created_at DESC
`

	FLOW_COUNT_ACTIVE_VIOLATIONS_BY_POLICY = `
SELECT COUNT(*)
FROM policy_violations
WHERE tenant_id = $1 AND policy_id = $2 AND status = 'ACTIVE'
`

	FLOW_RESOLVE_VIOLATION = `
UPDATE policy_violations
SET status = 'RESOLVED', version = version + 1, updated_at = $1
WHERE tenant_id = $2 AND id = $3 AND version = $4
`
)
