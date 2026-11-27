SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP POLICY IF EXISTS rbac_user_bindings_tenant_isolation ON role_based_access_control.rbac_user_bindings;
DROP POLICY IF EXISTS access_grants_tenant_isolation ON role_based_access_control.access_grants;
DROP POLICY IF EXISTS rbac_audit_outbox_tenant_isolation ON role_based_access_control.rbac_audit_outbox;
