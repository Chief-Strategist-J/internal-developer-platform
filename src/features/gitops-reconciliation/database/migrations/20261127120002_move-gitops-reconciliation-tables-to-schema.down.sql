SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('gitops_reconciliation.gitops_reconciliations') IS NOT NULL AND to_regclass('public.gitops_reconciliations') IS NULL THEN
        EXECUTE 'ALTER TABLE gitops_reconciliation.gitops_reconciliations SET SCHEMA public';
    END IF;
    IF to_regclass('gitops_reconciliation.gitops_secret_mechanisms') IS NOT NULL AND to_regclass('public.gitops_secret_mechanisms') IS NULL THEN
        EXECUTE 'ALTER TABLE gitops_reconciliation.gitops_secret_mechanisms SET SCHEMA public';
    END IF;
    IF to_regclass('gitops_reconciliation.gitops_outbox_events') IS NOT NULL AND to_regclass('public.gitops_outbox_events') IS NULL THEN
        EXECUTE 'ALTER TABLE gitops_reconciliation.gitops_outbox_events SET SCHEMA public';
    END IF;
END $$;
