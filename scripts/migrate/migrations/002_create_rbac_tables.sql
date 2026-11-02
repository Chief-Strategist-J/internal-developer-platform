-- 002_create_rbac_tables.sql
CREATE TABLE IF NOT EXISTS rbac_roles (
    name VARCHAR(64) PRIMARY KEY,
    permissions JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS rbac_user_bindings (
    subject_id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(128) NOT NULL,
    role_name VARCHAR(64) NOT NULL REFERENCES rbac_roles(name) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (subject_id, tenant_id, role_name)
);

-- Seed default roles
INSERT INTO rbac_roles (name, permissions) VALUES
('PlatformAdmin', '[{"resource": "*", "action": "*"}]'),
('Developer', '[{"resource": "auth", "action": "read"}, {"resource": "service-catalog", "action": "read"}, {"resource": "service-catalog", "action": "write"}, {"resource": "service-catalog", "action": "update"}, {"resource": "infrastructure", "action": "read"}]'),
('Viewer', '[{"resource": "auth", "action": "read"}, {"resource": "service-catalog", "action": "read"}, {"resource": "infrastructure", "action": "read"}]')
ON CONFLICT (name) DO UPDATE SET permissions = EXCLUDED.permissions;
