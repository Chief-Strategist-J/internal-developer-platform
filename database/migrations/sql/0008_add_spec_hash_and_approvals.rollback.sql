-- rollback:     0008
-- description:    drop spec hash and approval binding columns from workload_provisions

ALTER TABLE workload_provisions
    DROP COLUMN IF EXISTS approver_role,
    DROP COLUMN IF EXISTS approved_spec_hash,
    DROP COLUMN IF EXISTS spec_hash;
