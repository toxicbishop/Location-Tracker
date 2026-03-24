#!/bin/bash
set -e

UNTIL="until cqlsh cassandra -e 'describe cluster'; do echo 'Waiting for Cassandra...'; sleep 5; done"
docker exec -it cassandra bash -c "$UNTIL"

echo "Creating keyspace and tables..."
docker exec -i cassandra cqlsh < migrations.cql

echo "Cassandra initialization complete."
