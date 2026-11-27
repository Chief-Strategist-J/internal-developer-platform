SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('service_catalog.service_descriptors') IS NOT NULL AND to_regclass('public.service_descriptors') IS NULL THEN
        EXECUTE 'ALTER TABLE service_catalog.service_descriptors SET SCHEMA public';
    END IF;
END $$;
