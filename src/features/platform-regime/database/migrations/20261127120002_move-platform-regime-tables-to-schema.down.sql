SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('platform_regime.regime_kms_custody_modes') IS NOT NULL AND to_regclass('public.regime_kms_custody_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE platform_regime.regime_kms_custody_modes SET SCHEMA public';
    END IF;
    IF to_regclass('platform_regime.regime_cluster_modes') IS NOT NULL AND to_regclass('public.regime_cluster_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE platform_regime.regime_cluster_modes SET SCHEMA public';
    END IF;
    IF to_regclass('platform_regime.regime_tenancy_modes') IS NOT NULL AND to_regclass('public.regime_tenancy_modes') IS NULL THEN
        EXECUTE 'ALTER TABLE platform_regime.regime_tenancy_modes SET SCHEMA public';
    END IF;
    IF to_regclass('platform_regime.regime_policy_status') IS NOT NULL AND to_regclass('public.regime_policy_status') IS NULL THEN
        EXECUTE 'ALTER TABLE platform_regime.regime_policy_status SET SCHEMA public';
    END IF;
    IF to_regclass('platform_regime.platform_regime_policies') IS NOT NULL AND to_regclass('public.platform_regime_policies') IS NULL THEN
        EXECUTE 'ALTER TABLE platform_regime.platform_regime_policies SET SCHEMA public';
    END IF;
END $$;
