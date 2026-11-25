-- Row-level security policies (multi-tenancy)

ALTER TABLE rbac_user_bindings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rbac_user_bindings_tenant_isolation ON rbac_user_bindings;
CREATE POLICY rbac_user_bindings_tenant_isolation ON rbac_user_bindings
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));

ALTER TABLE access_grants ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS access_grants_tenant_isolation ON access_grants;
CREATE POLICY access_grants_tenant_isolation ON access_grants
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));

ALTER TABLE rbac_audit_outbox ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS rbac_audit_outbox_tenant_isolation ON rbac_audit_outbox;
CREATE POLICY rbac_audit_outbox_tenant_isolation ON rbac_audit_outbox
    USING (tenant_id = current_setting('app.current_tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.current_tenant_id', true));
