-- 001_create_service_catalog.sql
CREATE TABLE IF NOT EXISTS service_descriptors (
    id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(128) NOT NULL,
    name VARCHAR(256) NOT NULL,
    owner VARCHAR(256) NOT NULL,
    lifecycle VARCHAR(64) NOT NULL,
    status VARCHAR(64) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_services_tenant ON service_descriptors(tenant_id);
CREATE INDEX IF NOT EXISTS idx_services_status ON service_descriptors(status);
