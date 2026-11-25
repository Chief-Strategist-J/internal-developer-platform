-- rollback:       0019
-- description:    drop gitops_outbox_events table
SET lock_timeout = '3s';

DROP INDEX IF EXISTS idx_gitops_outbox_unpublished;
DROP TABLE IF EXISTS gitops_outbox_events;
