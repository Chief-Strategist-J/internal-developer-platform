-- migration:      20261127163000
-- description:    create self_service_provisioning schema and durable intent ledger tables (INV-01)
-- author:         platform-core
-- date:           2026-11-27T16:30:00Z
-- reversible:     YES
-- lock_risk:      LOW

SET lock_timeout = '3s';

CREATE SCHEMA IF NOT EXISTS self_service_provisioning;

CREATE TABLE IF NOT EXISTS self_service_provisioning.provisioning_intents (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    public_id VARCHAR(128) NOT NULL UNIQUE CHECK (public_id <> ''),
    tenant_id VARCHAR(128) NOT NULL CHECK (tenant_id <> ''),
    resource_kind VARCHAR(32) NOT NULL CHECK (resource_kind IN ('CLUSTER', 'WORKLOAD_IDENTITY')),
    target_id VARCHAR(255) NOT NULL CHECK (target_id <> ''),
    attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1),
    status VARCHAR(32) NOT NULL CHECK (status IN ('PENDING', 'READY', 'FAILED')),
    outcome_id VARCHAR(255) NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ssp_intents_tenant_target
    ON self_service_provisioning.provisioning_intents (tenant_id, resource_kind, target_id);

CREATE TABLE IF NOT EXISTS self_service_provisioning.outbox (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    aggregate_id VARCHAR(128) NOT NULL CHECK (aggregate_id <> ''),
    event_type VARCHAR(128) NOT NULL CHECK (event_type <> ''),
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS self_service_provisioning.processed_events (
    event_id VARCHAR(128) PRIMARY KEY CHECK (event_id <> ''),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
