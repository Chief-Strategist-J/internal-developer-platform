-- migration:      0021
-- description:    rollback rbac schema hardening
-- author:         platform-core
-- date:           2026-11-25T17:16:00Z
-- depends_on:     0021
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         revert rbac_role_hierarchy and added constraints/columns

SET lock_timeout = '3s';

ALTER TABLE rbac_audit_outbox
    DROP CONSTRAINT IF EXISTS rbac_audit_outbox_subject_nonempty_ck,
    DROP CONSTRAINT IF EXISTS rbac_audit_outbox_tenant_nonempty_ck;

ALTER TABLE rbac_user_bindings
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS version;

DROP TABLE IF EXISTS rbac_role_hierarchy;
