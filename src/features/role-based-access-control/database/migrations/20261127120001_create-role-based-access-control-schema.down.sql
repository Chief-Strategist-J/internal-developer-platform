SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TABLE IF EXISTS role_based_access_control.processed_events;
DROP TABLE IF EXISTS role_based_access_control.outbox;
