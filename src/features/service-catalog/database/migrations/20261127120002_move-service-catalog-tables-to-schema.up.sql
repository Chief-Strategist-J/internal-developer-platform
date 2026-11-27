-- Phase 5 Expand-Safe Table Ownership Relocation into service_catalog (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.service_descriptors') IS NOT NULL AND to_regclass('service_catalog.service_descriptors') IS NULL THEN
        EXECUTE 'ALTER TABLE public.service_descriptors SET SCHEMA service_catalog';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA service_catalog TO svc_service_catalog;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA service_catalog TO svc_service_catalog;
