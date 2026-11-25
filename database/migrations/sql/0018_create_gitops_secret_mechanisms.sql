-- migration:      0018
-- description:    create gitops_secret_mechanisms table for D-SEC configuration
-- author:         platform-core
-- date:           2026-11-25T11:46:00Z
-- depends_on:     0017
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists tenant-scoped destination-cluster secret mechanism (D-SEC)
-- entity:         gitops_secret_mechanisms
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | NOT NULL

SET lock_timeout = '3s';

CREATE TABLE IF NOT EXISTS gitops_secret_mechanisms (
    tenant_id VARCHAR(128) PRIMARY KEY CHECK (tenant_id <> ''),
    mechanism VARCHAR(64) NOT NULL CHECK (mechanism IN ('SEALED_SECRETS', 'EXTERNAL_SECRETS_OPERATOR', 'SECRETS_STORE_CSI_DRIVER')),
    vault_endpoint TEXT NOT NULL DEFAULT '',
    cluster_scope VARCHAR(128) NOT NULL DEFAULT '',
    secret_store_ref VARCHAR(255) NOT NULL DEFAULT '',
    csi_driver_name VARCHAR(255) NOT NULL DEFAULT '',
    encryption_key_ref VARCHAR(255) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
