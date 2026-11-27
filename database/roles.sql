-- Declarative per-feature PostgreSQL roles and schema grants (D-01, VII.1)
-- Auto-derived from src/features/*/context.yml persistence declarations.

CREATE SCHEMA IF NOT EXISTS platform;

DO $$
DECLARE
    role_name TEXT;
    role_list TEXT[] := ARRAY[
        'svc_platform',
        'svc_outbox_relay',
        'svc_service_catalog',
        'svc_rbac',
        'svc_gitops_reconciliation',
        'svc_workload_provisioning',
        'svc_policy_as_code',
        'svc_platform_regime',
        'svc_disaster_recovery'
    ];
BEGIN
    FOREACH role_name IN ARRAY role_list LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE', role_name);
        END IF;
    END LOOP;
END $$;

REVOKE ALL ON SCHEMA platform FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA platform TO svc_platform;
