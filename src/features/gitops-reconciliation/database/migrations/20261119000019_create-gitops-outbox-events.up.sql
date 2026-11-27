-- migration:      0019
-- description:    create gitops_outbox_events table for durable domain event emission
-- author:         platform-core
-- date:           2026-11-25T14:20:00Z
-- depends_on:     0018
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists transactional outbox events for ReconciliationSucceeded, ReconciliationFailed, RollbackTriggered
-- entity:         gitops_outbox_events
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | NOT NULL

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS gitops_outbox_events (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    tenant_id VARCHAR(128) NOT NULL CHECK (tenant_id <> ''),
    aggregate_id VARCHAR(128) NOT NULL CHECK (aggregate_id <> ''),
    event_type VARCHAR(64) NOT NULL CHECK (event_type IN ('ReconciliationSucceeded', 'ReconciliationFailed', 'RollbackTriggered')),
    app_name VARCHAR(255) NOT NULL DEFAULT '',
    revision VARCHAR(128) NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_gitops_outbox_unpublished ON gitops_outbox_events (occurred_at) WHERE published_at IS NULL;
