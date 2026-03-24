# Real-time Driver Location Tracker

A production-grade GPS event pipeline built with Go, Kafka, Cassandra, Redis, and WebSockets.
Tracks live driver positions for a ride-sharing application.

---

## Recent Enhancements

- **Security & Auth**: Added JWT and API Key validation.
- **Geospatial Queries**: Proximity search ("find nearby drivers") using Redis GEO.
- **Observability**: Structured JSON logging (`zerolog`) and Prometheus metrics.
- **Performance**: Batch ingestion support for high-throughput clients.
- **Historical Data**: `/history` endpoint for time-series journey tracking from Cassandra.
- **Resilience**: Integrated Dead Letter Queue (DLQ) support.

---

## Architecture

![Architecture Diagram](assets/architecture.svg)

The system architecture is designed for high availability and low latency. You can view and edit the high-fidelity diagram using the **draw.io** extension by opening [assets/architecture.drawio](assets/architecture.drawio).

### Why this stack?

| Need                          | Solution                      | Why not the alternative?                                        |
| ----------------------------- | ----------------------------- | --------------------------------------------------------------- |
| High-throughput GPS ingestion | **Kafka**                     | REST -> DB directly would bottleneck on write spikes            |
| Full location history         | **Cassandra**                 | Time-series data, append-only, TTL support, scales horizontally |
| Current position lookup       | **Redis**                     | O(1) in-memory vs Cassandra disk I/O for a 2s polling use case  |
| Live position streaming       | **Redis Pub/Sub + WebSocket** | No polling overhead; push-based fan-out to multiple riders      |

---

## Project Structure

```text
location-tracker/
|-- Makefile                    # Build, test, run, and migrate
|-- .env.example                # Config template
|-- migrations.cql              # Cassandra keyspace + tables
|-- docker-compose.yml          # Infrastructure + Application stack
|
|-- api/                        # HTTP Producer + WebSocket Service
|   |-- auth.go                 # JWT & API Key Middlewares
|   |-- main.go                 # Handlers & Routing
|   |-- ws.go                   # WebSocket logic
|   |-- metrics.go              # Prometheus instrumentation
|   |-- main_test.go            # Validation tests
|   |-- Dockerfile
|
|-- worker/                     # Kafka Consumer Worker
|   |-- main.go                 # Processing logic (Kafka -> DBs)
|   |-- dlq.go                  # Dead Letter Queue handling
|   |-- Dockerfile
|
|-- db/                         # Data Access Layer
|   |-- cassandra.go            # Cassandra Session & Time-series Queries
|   |-- redis.go                # Redis Caching & Geospatial Queries
|   |-- redis_test.go           # Redis unit tests (miniredis)
|
|-- models/                     # Shared data structures
|   |-- location.go             # LocationEvent
|
|-- scripts/                    # Tools & Automation
|   |-- init-cassandra.sh       # Automated schema migration
|   |-- simulate.sh             # Live GPS event generator
|   |-- test-ws.html            # WebSocket test UI
```

---

## API Endpoints

| Method | Path                    | Auth    | Description                          |
| ------ | ----------------------- | ------- | ------------------------------------ |
| `POST` | `/location`             | JWT/Key | Ingest single GPS event              |
| `POST` | `/location/batch`       | JWT/Key | Ingest array of GPS events           |
| `GET`  | `/driver/{id}/location` | None    | Latest position from Redis           |
| `GET`  | `/driver/{id}/history`  | None    | Journey history from Cassandra       |
| `GET`  | `/drivers/nearby`       | None    | Find drivers within X km (Redis GEO) |
| `GET`  | `/ws/driver/{id}`       | JWT/Key | Live position stream (WebSocket)     |
| `GET`  | `/health`               | None    | Health check                         |

### POST /location

```bash
curl -X POST http://localhost:8080/location \
  -H "Content-Type: application/json" \
  -d '{
    "driver_id": "550e8400-e29b-41d4-a716-446655440001",
    "lat": 12.9716,
    "lng": 77.5946,
    "speed": 45.5
  }'
# -> 202 Accepted { "status": "accepted" }
```

### GET /driver/{id}/location

```bash
curl http://localhost:8080/driver/550e8400-e29b-41d4-a716-446655440001/location
# -> { "driver_id": "...", "lat": 12.9716, "lng": 77.5946, "speed": 45.5, "recorded_at": "..." }
```

### WebSocket /driver/{id}/ws

Open `scripts/test-ws.html` in your browser, paste a driver UUID, click Connect.
Receives a JSON push for every GPS event from that driver in real time.

---

## Setup & Running

This project uses a `Makefile` for developer ergonomics.

### 1. Prerequisites
- Docker & Docker Compose
- Go 1.23+ (if running locally)

### 2. Quick Start (Full Stack)
```bash
# Start all infrastructure and apps
make docker-up

# Wait ~45s for Cassandra, then run migrations (automatic via script)
./scripts/init-cassandra.sh

# Run simulator to see data flowing
./scripts/simulate.sh
```

### 3. Developer Workflow
```bash
# Build binaries
make build

# Run unit tests (including API validation and Redis mocks)
make test

# Send manual test data
make mock-data
```

### Environment Variables

| Variable         | Default          | Description                  |
| ---------------- | ---------------- | ---------------------------- |
| `KAFKA_BROKER`   | `localhost:9092` | Kafka broker address         |
| `CASSANDRA_HOST` | `localhost`      | Cassandra host               |
| `REDIS_ADDR`     | `localhost:6379` | Redis address                |
| `LISTEN_ADDR`    | `:8080`          | Producer HTTP listen address |

---

## Scaling

```bash
# Run 3 consumer instances -- Kafka distributes partitions across them
docker-compose up --scale consumer=3
```

Kafka's consumer group (`cassandra-writer`) ensures each partition is handled
by exactly one consumer at a time. No duplicate writes to Cassandra.

---

## Benchmark

```bash
chmod +x scripts/benchmark.sh

# Default: 500 requests, 20 parallel workers
./scripts/benchmark.sh

# Custom: 2000 requests, 50 workers
./scripts/benchmark.sh 2000 50
```

Sample output:

```text
============================================
  Location Tracker -- Throughput Benchmark
============================================
  Target : http://localhost:8080/location
  Total  : 500 requests
  Workers: 20 parallel
--------------------------------------------
  Running .......... done

============================================
  Results
============================================
  Total sent:          500
  Successful (202):    498
  Failed:              2
  Elapsed:             1.84 s
  Throughput:          270.6 req/s
  Redis cached drivers: 20
============================================
```

---

## Key Design Decisions

**1. Message key = `driver_id` in Kafka**
Routes all events for the same driver to the same partition. Guarantees ordering
per driver -- critical for accurate location history.

**2. Cassandra partition key = `driver_id`**
All rows for a driver live on the same node. Range queries like
"give me the last 5 minutes for driver X" touch one partition -- fast.

**3. TTL = 86400s on GPS rows**
GPS history older than 24h has no operational value. Cassandra auto-expires rows --
no cleanup job needed.

**4. Redis is best-effort**
If the Redis write fails after a Cassandra insert, the consumer logs it and moves on.
Cassandra is the source of truth. Redis is a fast cache that repopulates on the next event.

**5. Pub/sub channel per driver**
`driver:updates:{id}` means a WebSocket handler only subscribes to updates for
the specific driver a rider is watching -- no unnecessary fan-out.

**6. Initial position on WebSocket connect**
The WebSocket handler sends the latest cached position from Redis immediately on
connect. The rider sees the driver's position right away without waiting for the next GPS event.

---

## Concepts Covered

| Concept                                      | Where                                                    |
| -------------------------------------------- | -------------------------------------------------------- |
| Kafka producer with message keys             | `producer/main.go`                                       |
| Kafka consumer groups + partition assignment | `consumer/main.go`                                       |
| Horizontal consumer scaling                  | `docker-compose up --scale consumer=N`                   |
| Cassandra time-series schema                 | `schema.cql`                                             |
| Cassandra partition + clustering keys        | `db/cassandra.go`                                        |
| TTL-based data expiry                        | `schema.cql` -- `default_time_to_live`                   |
| Redis key-value cache                        | `db/redis.go` -- `SetLatestLocation`                     |
| Redis pub/sub                                | `db/redis.go` -- `PublishLocation` / `SubscribeToDriver` |
| WebSocket upgrade + lifecycle                | `producer/ws.go`                                         |
| Exponential backoff retry                    | `consumer/main.go` -- `insertWithRetry`                  |
| At-least-once delivery                       | Kafka offset committed after successful Cassandra insert |
| Multi-stage Docker builds                    | `producer/Dockerfile`, `consumer/Dockerfile`             |
| Environment-based config                     | `envOr()` in both services                               |

---

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for details.

## License

This project is licensed under the **GNU General Public License v3.0**. See the [LICENSE](./LICENSE) file for more information.
