-- migration:      0022
-- description:    validate rbac_audit_outbox and rbac_user_bindings integrity constraints
-- author:         platform-core
-- date:           2026-11-27T10:52:00Z
-- depends_on:     0021
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  metadata validation only
-- reason:         validate NOT VALID constraints from 0021 and enforce trimmed non-empty text bounds
-- entity:         rbac_audit_outbox, rbac_user_bindings
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         VALIDATE | CHECK

SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE rbac_audit_outbox VALIDATE CONSTRAINT rbac_audit_outbox_tenant_nonempty_ck;
ALTER TABLE rbac_audit_outbox VALIDATE CONSTRAINT rbac_audit_outbox_subject_nonempty_ck;

ALTER TABLE rbac_user_bindings
    DROP CONSTRAINT IF EXISTS rbac_user_bindings_subject_trimmed_ck,
    ADD CONSTRAINT rbac_user_bindings_subject_trimmed_ck CHECK (subject_id <> '' AND subject_id = btrim(subject_id)) NOT VALID;

ALTER TABLE rbac_user_bindings VALIDATE CONSTRAINT rbac_user_bindings_subject_trimmed_ck;

ALTER TABLE rbac_user_bindings
    DROP CONSTRAINT IF EXISTS rbac_user_bindings_tenant_trimmed_ck,
    ADD CONSTRAINT rbac_user_bindings_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)) NOT VALID;

ALTER TABLE rbac_user_bindings VALIDATE CONSTRAINT rbac_user_bindings_tenant_trimmed_ck;
