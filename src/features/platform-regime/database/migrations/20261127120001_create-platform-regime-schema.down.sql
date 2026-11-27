SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TABLE IF EXISTS platform_regime.processed_events;
DROP TABLE IF EXISTS platform_regime.outbox;
