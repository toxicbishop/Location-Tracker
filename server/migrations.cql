-- Run this after Cassandra is up:
-- docker exec -it <cassandra-container> cqlsh -f /schema.cql

CREATE KEYSPACE IF NOT EXISTS rideshare
    WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE rideshare;

CREATE TABLE IF NOT EXISTS driver_locations (
    driver_id   UUID,
    recorded_at TIMESTAMP,
    event_id    UUID,           -- idempotency key: same (driver_id, recorded_at, event_id) = safe re-insert
    lat         DOUBLE,
    lng         DOUBLE,
    speed       FLOAT,
    PRIMARY KEY (driver_id, recorded_at, event_id)
) WITH CLUSTERING ORDER BY (recorded_at DESC, event_id ASC)
  AND default_time_to_live = 86400;
  -- TTL: auto-delete GPS rows after 24 hours

