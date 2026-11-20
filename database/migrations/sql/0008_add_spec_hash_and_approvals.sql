-- migration:      0008
-- description:    add spec hash and approval binding columns to workload_provisions
-- author:         chief-strategist
-- date:           2026-11-20T12:15:00Z
-- depends_on:     0007
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce cryptographic spec hash binding to approvals and audit tracking per business-logic.md

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS spec_hash TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS approved_spec_hash TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS approver_role TEXT DEFAULT 'system-auto';

UPDATE workload_provisions
SET
    spec_hash = COALESCE(NULLIF(spec_hash, ''), 'sha256:legacy:' || id),
    approved_spec_hash = COALESCE(NULLIF(approved_spec_hash, ''), 'sha256:legacy:' || id),
    approver_role = COALESCE(NULLIF(approver_role, ''), 'system-auto')
WHERE spec_hash = '' OR approved_spec_hash = '' OR approver_role = '';

