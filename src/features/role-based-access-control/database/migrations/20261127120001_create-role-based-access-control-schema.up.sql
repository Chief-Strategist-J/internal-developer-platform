-- Phase 5 Schema Ownership + Transactional Outbox/Inbox (D-01, D-02, D-03, D-04)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE SCHEMA IF NOT EXISTS role_based_access_control;

REVOKE ALL ON SCHEMA role_based_access_control FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA role_based_access_control TO svc_rbac;
GRANT USAGE ON SCHEMA role_based_access_control TO svc_outbox_relay;

CREATE TABLE IF NOT EXISTS role_based_access_control.outbox (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(128) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    aggregate_version BIGINT NOT NULL DEFAULT 1,
    event_type VARCHAR(255) NOT NULL,
    schema_version VARCHAR(32) NOT NULL DEFAULT 'v1',
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_role_based_access_control_outbox_unpublished
    ON role_based_access_control.outbox (occurred_at)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS role_based_access_control.processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA role_based_access_control TO svc_rbac;
GRANT SELECT, UPDATE ON role_based_access_control.outbox TO svc_outbox_relay;
