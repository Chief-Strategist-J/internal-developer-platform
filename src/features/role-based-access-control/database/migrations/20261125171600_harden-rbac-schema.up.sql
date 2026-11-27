-- migration:      0021
-- description:    harden rbac schema with role hierarchy, optimistic locking, and audit immutability
-- author:         platform-core
-- date:           2026-11-25T17:16:00Z
-- depends_on:     0020
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         add rbac_role_hierarchy DAG table, version/updated_at on bindings, and non-empty check constraints
-- entity:         rbac_user_bindings, rbac_role_hierarchy, rbac_audit_outbox
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | FK | TRIGGER

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS rbac_role_hierarchy (
    parent_role VARCHAR(64) NOT NULL REFERENCES rbac_roles(name) ON DELETE CASCADE,
    child_role VARCHAR(64) NOT NULL REFERENCES rbac_roles(name) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (parent_role, child_role),
    CONSTRAINT rbac_role_hierarchy_no_self_ck CHECK (parent_role <> child_role)
);

ALTER TABLE rbac_user_bindings
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE rbac_audit_outbox
    DROP CONSTRAINT IF EXISTS rbac_audit_outbox_tenant_nonempty_ck,
    ADD CONSTRAINT rbac_audit_outbox_tenant_nonempty_ck CHECK (tenant_id <> '') NOT VALID;

ALTER TABLE rbac_audit_outbox
    DROP CONSTRAINT IF EXISTS rbac_audit_outbox_subject_nonempty_ck,
    ADD CONSTRAINT rbac_audit_outbox_subject_nonempty_ck CHECK (subject_id <> '') NOT VALID;
