SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TABLE IF EXISTS kubernetes_workload_provisioning.processed_events;
DROP TABLE IF EXISTS kubernetes_workload_provisioning.outbox;
