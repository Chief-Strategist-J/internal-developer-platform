-- Phase 5 Schema Ownership + Transactional Outbox/Inbox (D-01, D-02, D-03, D-04)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE SCHEMA IF NOT EXISTS kubernetes_workload_provisioning;

REVOKE ALL ON SCHEMA kubernetes_workload_provisioning FROM PUBLIC;
GRANT USAGE, CREATE ON SCHEMA kubernetes_workload_provisioning TO svc_workload_provisioning;
GRANT USAGE ON SCHEMA kubernetes_workload_provisioning TO svc_outbox_relay;

CREATE TABLE IF NOT EXISTS kubernetes_workload_provisioning.outbox (
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

CREATE INDEX IF NOT EXISTS idx_kubernetes_workload_provisioning_outbox_unpublished
    ON kubernetes_workload_provisioning.outbox (occurred_at)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS kubernetes_workload_provisioning.processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA kubernetes_workload_provisioning TO svc_workload_provisioning;
GRANT SELECT, UPDATE ON kubernetes_workload_provisioning.outbox TO svc_outbox_relay;
