-- Run this after Cassandra is up:
-- docker exec -it <cassandra-container> cqlsh -f /schema.cql

CREATE KEYSPACE IF NOT EXISTS rideshare
    WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

USE rideshare;

CREATE TABLE IF NOT EXISTS driver_locations (
    driver_id   UUID,
    recorded_at TIMESTAMP,
    lat         DOUBLE,
    lng         DOUBLE,
    speed       FLOAT,
    PRIMARY KEY (driver_id, recorded_at)
) WITH CLUSTERING ORDER BY (recorded_at DESC)
  AND default_time_to_live = 86400;
  -- TTL: auto-delete GPS rows after 24 hours
