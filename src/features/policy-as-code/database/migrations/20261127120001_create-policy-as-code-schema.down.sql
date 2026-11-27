SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TABLE IF EXISTS policy_as_code.processed_events;
DROP TABLE IF EXISTS policy_as_code.outbox;
