package queries

import "fmt"

const (
	KeyPrefixRevocationSub = "revocation:sub:"
	KeyPrefixRevocationJTI = "revocation:jti:"
	DefaultTTLSeconds      = 86400
	RevokedSentinelValue   = "revoked"
	ExistsPositiveResult   = ":1"
	OkResponsePrefix       = "+OK"
)

func BuildRevocationSubKey(subjectID string) string {
	return fmt.Sprintf("%s%s", KeyPrefixRevocationSub, subjectID)
}

func BuildRevocationJTIKey(jti string) string {
	return fmt.Sprintf("%s%s", KeyPrefixRevocationJTI, jti)
}

func BuildExistsCommand(key string) string {
	return fmt.Sprintf("EXISTS %s", key)
}

func BuildSetexCommand(key string, ttl int, value string) string {
	return fmt.Sprintf("SETEX %s %d %s", key, ttl, value)
}

const (
	FlowInsertAccessGrant = `
		INSERT INTO access_grants (
			id, tenant_id, subject_id, target_namespace, role, reason,
			duration_seconds, approved_by, status, expires_at, idempotency_key,
			version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);
	`

	FlowGetAccessGrantByID = `
		SELECT id, tenant_id, subject_id, target_namespace, role, reason,
		       duration_seconds, approved_by, status, expires_at, idempotency_key,
		       version, created_at, updated_at
		FROM access_grants
		WHERE tenant_id = $1 AND id = $2;
	`

	FlowRevokeAccessGrant = `
		UPDATE access_grants
		SET status = 'REVOKED', updated_at = $3
		WHERE tenant_id = $1 AND id = $2 AND status = 'ACTIVE';
	`

	FlowListActiveAccessGrants = `
		SELECT id, tenant_id, subject_id, target_namespace, role, reason,
		       duration_seconds, approved_by, status, expires_at, idempotency_key,
		       version, created_at, updated_at
		FROM access_grants
		WHERE tenant_id = $1 AND subject_id = $2 AND status = 'ACTIVE' AND expires_at > $3
		ORDER BY created_at DESC;
	`

	FlowInsertUserBinding = `
		INSERT INTO rbac_user_bindings (subject_id, tenant_id, role_name, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (subject_id, tenant_id, role_name) DO NOTHING;
	`

	FlowDeleteUserBinding = `
		DELETE FROM rbac_user_bindings
		WHERE tenant_id = $1 AND subject_id = $2 AND role_name = $3;
	`

	FlowGetUserRoles = `
		SELECT role_name
		FROM rbac_user_bindings
		WHERE tenant_id = $1 AND subject_id = $2
		ORDER BY role_name ASC;
	`

	FlowInsertRBACAuditOutbox = `
		INSERT INTO rbac_audit_outbox (
			id, tenant_id, subject_id, resource, action, decision,
			reason, violation_code, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING;
	`

	FLOW_LIST_PENDING_RBAC_AUDIT_OUTBOX = `
		SELECT id, tenant_id, subject_id, resource, action, decision,
		       reason, violation_code, occurred_at
		FROM rbac_audit_outbox
		WHERE published_at IS NULL
		ORDER BY occurred_at ASC
		LIMIT $1;
	`

	FLOW_MARK_RBAC_AUDIT_OUTBOX_PUBLISHED = `
		UPDATE rbac_audit_outbox
		SET published_at = $2
		WHERE id = $1 AND published_at IS NULL;
	`

	FLOW_INSERT_ROLE_HIERARCHY = `
		INSERT INTO rbac_role_hierarchy (parent_role, child_role, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (parent_role, child_role) DO NOTHING;
	`

	FLOW_GET_ROLE_HIERARCHY = `
		SELECT parent_role, child_role
		FROM rbac_role_hierarchy
		ORDER BY parent_role ASC, child_role ASC;
	`

	FLOW_GET_ALL_ROLE_DEFINITIONS = `
		SELECT name, permissions
		FROM rbac_roles
		ORDER BY name ASC;
	`

	FLOW_CLAIM_EXCLUSIVE_ROLE_BINDING = `
		UPDATE rbac_user_bindings
		SET subject_id = $1, version = version + 1, updated_at = $4
		WHERE tenant_id = $2 AND role_name = $3 AND subject_id = '';
	`
)
