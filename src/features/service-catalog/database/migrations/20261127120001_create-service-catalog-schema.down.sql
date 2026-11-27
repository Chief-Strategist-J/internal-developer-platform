SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TABLE IF EXISTS service_catalog.processed_events;
DROP TABLE IF EXISTS service_catalog.outbox;
