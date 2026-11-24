-- rollback: 0015
SET lock_timeout = '3s';

DROP TABLE IF EXISTS dr_fencing_epochs CASCADE;
DROP TABLE IF EXISTS dr_plans CASCADE;
DROP TABLE IF EXISTS dr_topologies CASCADE;
DROP TABLE IF EXISTS dr_plan_status CASCADE;

