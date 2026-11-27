-- Phase 5 Expand-Safe Table Ownership Relocation into role_based_access_control (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.rbac_roles') IS NOT NULL AND to_regclass('role_based_access_control.rbac_roles') IS NULL THEN
        EXECUTE 'ALTER TABLE public.rbac_roles SET SCHEMA role_based_access_control';
    END IF;
    IF to_regclass('public.rbac_user_bindings') IS NOT NULL AND to_regclass('role_based_access_control.rbac_user_bindings') IS NULL THEN
        EXECUTE 'ALTER TABLE public.rbac_user_bindings SET SCHEMA role_based_access_control';
    END IF;
    IF to_regclass('public.access_grants') IS NOT NULL AND to_regclass('role_based_access_control.access_grants') IS NULL THEN
        EXECUTE 'ALTER TABLE public.access_grants SET SCHEMA role_based_access_control';
    END IF;
    IF to_regclass('public.rbac_audit_outbox') IS NOT NULL AND to_regclass('role_based_access_control.rbac_audit_outbox') IS NULL THEN
        EXECUTE 'ALTER TABLE public.rbac_audit_outbox SET SCHEMA role_based_access_control';
    END IF;
    IF to_regclass('public.rbac_role_hierarchy') IS NOT NULL AND to_regclass('role_based_access_control.rbac_role_hierarchy') IS NULL THEN
        EXECUTE 'ALTER TABLE public.rbac_role_hierarchy SET SCHEMA role_based_access_control';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA role_based_access_control TO svc_rbac;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA role_based_access_control TO svc_rbac;
