-- Enable Row-Level Security on role_based_access_control tenant-scoped tables (D-01, D-09)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE role_based_access_control.rbac_user_bindings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rbac_user_bindings_tenant_isolation ON role_based_access_control.rbac_user_bindings;
CREATE POLICY rbac_user_bindings_tenant_isolation ON role_based_access_control.rbac_user_bindings
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));

ALTER TABLE role_based_access_control.access_grants ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS access_grants_tenant_isolation ON role_based_access_control.access_grants;
CREATE POLICY access_grants_tenant_isolation ON role_based_access_control.access_grants
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));

ALTER TABLE role_based_access_control.rbac_audit_outbox ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rbac_audit_outbox_tenant_isolation ON role_based_access_control.rbac_audit_outbox;
CREATE POLICY rbac_audit_outbox_tenant_isolation ON role_based_access_control.rbac_audit_outbox
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));
