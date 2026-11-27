-- Phase 5 Expand-Safe Table Ownership Relocation into platform_regime (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.regime_kms_custody_modes') IS NOT NULL AND to_regclass('platform_regime.regime_kms_custody_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE public.regime_kms_custody_modes SET SCHEMA platform_regime';
    END IF;
    IF to_regclass('public.regime_cluster_modes') IS NOT NULL AND to_regclass('platform_regime.regime_cluster_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE public.regime_cluster_modes SET SCHEMA platform_regime';
    END IF;
    IF to_regclass('public.regime_tenancy_modes') IS NOT NULL AND to_regclass('platform_regime.regime_tenancy_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE public.regime_tenancy_modes SET SCHEMA platform_regime';
    END IF;
    IF to_regclass('public.regime_policy_status') IS NOT NULL AND to_regclass('platform_regime.regime_policy_status') IS NULL THEN
        EXECUTE 'ALTER TABLE public.regime_policy_status SET SCHEMA platform_regime';
    END IF;
    IF to_regclass('public.platform_regime_policies') IS NOT NULL AND to_regclass('platform_regime.platform_regime_policies') IS NULL THEN
        EXECUTE 'ALTER TABLE public.platform_regime_policies SET SCHEMA platform_regime';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA platform_regime TO svc_platform_regime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA platform_regime TO svc_platform_regime;
