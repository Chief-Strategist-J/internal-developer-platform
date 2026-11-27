-- Phase 5 Schema Ownership + Transactional Outbox/Inbox (D-01, D-02, D-03, D-04)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE SCHEMA IF NOT EXISTS platform_regime;

REVOKE ALL ON SCHEMA platform_regime FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA platform_regime TO svc_platform_regime;
GRANT USAGE ON SCHEMA platform_regime TO svc_outbox_relay;

CREATE TABLE IF NOT EXISTS platform_regime.outbox (
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

CREATE INDEX IF NOT EXISTS idx_platform_regime_outbox_unpublished
    ON platform_regime.outbox (occurred_at)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS platform_regime.processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA platform_regime TO svc_platform_regime;
GRANT SELECT, UPDATE ON platform_regime.outbox TO svc_outbox_relay;
