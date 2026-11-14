-- migration:      0004
-- description:    create workload provisions table
-- author:         chief-strategist
-- date:           2026-11-13T19:15:00Z
-- depends_on:     0003
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         initial schema creation for kubernetes-workload-provisioning feature

CREATE TABLE IF NOT EXISTS workload_provisions (
    id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(128) NOT NULL,
    service_id VARCHAR(128) NOT NULL,
    namespace VARCHAR(256) NOT NULL,
    workload_name VARCHAR(256) NOT NULL,
    cpu_request VARCHAR(32) NOT NULL,
    cpu_limit VARCHAR(32) NOT NULL,
    memory_request VARCHAR(32) NOT NULL,
    memory_limit VARCHAR(32) NOT NULL,
    replicas INTEGER NOT NULL DEFAULT 2,
    status VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(256) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT uq_workload_provisions_idempotency UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_workload_provisions_service ON workload_provisions(tenant_id, service_id);
CREATE INDEX IF NOT EXISTS idx_workload_provisions_status ON workload_provisions(status);

