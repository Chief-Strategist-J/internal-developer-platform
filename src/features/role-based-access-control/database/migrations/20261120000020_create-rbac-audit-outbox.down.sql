-- rollback:       0020
-- description:    drop rbac_audit_outbox table
SET lock_timeout = '3s';

DROP INDEX IF EXISTS idx_rbac_audit_outbox_tenant_subject;
DROP INDEX IF EXISTS idx_rbac_audit_outbox_unpublished;
DROP TABLE IF EXISTS rbac_audit_outbox;
