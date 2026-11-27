SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('role_based_access_control.rbac_roles') IS NOT NULL AND to_regclass('public.rbac_roles') IS NULL THEN
        EXECUTE 'ALTER TABLE role_based_access_control.rbac_roles SET SCHEMA public';
    END IF;
    IF to_regclass('role_based_access_control.rbac_user_bindings') IS NOT NULL AND to_regclass('public.rbac_user_bindings') IS NULL THEN
        EXECUTE 'ALTER TABLE role_based_access_control.rbac_user_bindings SET SCHEMA public';
    END IF;
    IF to_regclass('role_based_access_control.access_grants') IS NOT NULL AND to_regclass('public.access_grants') IS NULL THEN
        EXECUTE 'ALTER TABLE role_based_access_control.access_grants SET SCHEMA public';
    END IF;
    IF to_regclass('role_based_access_control.rbac_audit_outbox') IS NOT NULL AND to_regclass('public.rbac_audit_outbox') IS NULL THEN
        EXECUTE 'ALTER TABLE role_based_access_control.rbac_audit_outbox SET SCHEMA public';
    END IF;
    IF to_regclass('role_based_access_control.rbac_role_hierarchy') IS NOT NULL AND to_regclass('public.rbac_role_hierarchy') IS NULL THEN
        EXECUTE 'ALTER TABLE role_based_access_control.rbac_role_hierarchy SET SCHEMA public';
    END IF;
END $$;
