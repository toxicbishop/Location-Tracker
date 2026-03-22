# How to Run This Project -- Complete Guide

---

## Step 1: Prerequisites

Install these on your machine if you don't have them:

### Go 1.21+
```bash
# Check if installed
go version

# If not installed -- download from https://go.dev/dl/
# For Ubuntu/Debian:
sudo apt install golang-go

# Verify
go version  # should say go1.21 or higher
```

### Docker + Docker Compose
```bash
# Check if installed
docker --version
docker compose version

# If not installed:
# https://docs.docker.com/engine/install/ubuntu/
# After installing, add your user to docker group so you don't need sudo:
sudo usermod -aG docker $USER
# Log out and back in for this to take effect
```

---

## Step 2: Get the Project Files

You have two options:

### Option A -- Copy from Claude's output
Download the files Claude gave you and put them in a folder:
```
location-tracker/
+-- docker-compose.yml
+-- schema.cql
+-- go.mod
+-- models/
|   +-- location.go
+-- db/
|   +-- cassandra.go
|   +-- redis.go
+-- producer/
|   +-- main.go
|   +-- ws.go
|   +-- Dockerfile
+-- consumer/
|   +-- main.go
|   +-- Dockerfile
+-- scripts/
    +-- simulate.sh
    +-- benchmark.sh
    +-- test-ws.html
```

### Option B -- Create fresh from terminal
```bash
mkdir location-tracker
cd location-tracker
mkdir -p models db producer consumer scripts
# Then paste each file's contents into the correct path
```

---

## Step 3: Fix the Module Name (Important!)

The go.mod uses `github.com/pranav/location-tracker` as the module name.
You need to make sure all imports match. Open `go.mod` and verify it says:

```
module github.com/pranav/location-tracker

go 1.21

require (
    github.com/gocql/gocql v1.6.0
    github.com/gorilla/websocket v1.5.1
    github.com/redis/go-redis/v9 v9.5.1
    github.com/segmentio/kafka-go v0.4.47
)
```

If you want to rename the module (e.g. to just `location-tracker`), run:
```bash
# From inside the location-tracker folder
go mod edit -module location-tracker

# Then update all import paths in every .go file
# Replace: "github.com/pranav/location-tracker/db"
# With:    "location-tracker/db"
# (and same for models)
```

Easiest approach: just keep the module name as-is, it works fine.

---

## Step 4: Download Dependencies

```bash
# From inside the location-tracker/ folder
go mod tidy
```

This will:
- Create a `go.sum` file
- Download all 4 libraries (gocql, gorilla/websocket, go-redis, kafka-go)

You should see output like:
```
go: downloading github.com/segmentio/kafka-go v0.4.47
go: downloading github.com/gocql/gocql v1.6.0
...
```

---

## Step 5: Start Infrastructure with Docker

```bash
# From inside the location-tracker/ folder
# This starts: Zookeeper, Kafka, Cassandra, Redis
docker compose up zookeeper kafka cassandra redis -d
```

The `-d` flag runs them in the background.

Check they're running:
```bash
docker compose ps
```

You should see 4 containers with status `Up`.

**Wait about 3045 seconds** for Cassandra to fully boot.
You can check when it's ready:
```bash
docker compose logs cassandra | tail -5
# Look for: "Starting listening for CQL clients"
```

---

## Step 6: Apply the Cassandra Schema

```bash
# Get the cassandra container name
docker compose ps
# It will be something like: location-tracker-cassandra-1

# Apply the schema
docker exec -i location-tracker-cassandra-1 cqlsh < schema.cql
```

If that works, you'll see no output (silent success).

Verify it worked:
```bash
docker exec -it location-tracker-cassandra-1 cqlsh

# Inside cqlsh:
USE rideshare;
DESCRIBE TABLES;
# Should show: driver_locations

exit
```

---

## Step 7: Run the Consumer

Open **Terminal 1** inside your `location-tracker/` folder:

```bash
go run ./consumer/main.go
```

You should see:
```
Connected to Cassandra
Connected to Redis
Consumer ready -- waiting for GPS events...
```

If Cassandra isn't ready yet, it will retry every 5 seconds automatically.

---

## Step 8: Run the Producer

Open **Terminal 2** inside your `location-tracker/` folder:

```bash
go run ./producer/main.go
```

You should see:
```
Producer listening on :8080 | kafka=localhost:9092 redis=localhost:6379
```

---

## Step 9: Test It

### Send a single GPS event
Open **Terminal 3**:

```bash
curl -X POST http://localhost:8080/location \
  -H "Content-Type: application/json" \
  -d '{
    "driver_id": "550e8400-e29b-41d4-a716-446655440001",
    "lat": 12.9716,
    "lng": 77.5946,
    "speed": 45.5
  }'
```

Expected response:
```json
{"status":"accepted"}
```

Check the consumer terminal -- you should see:
```
Saved: driver=550e8400... lat=12.9716 lng=77.5946 speed=45.5
```

### Fetch latest location from Redis
```bash
curl http://localhost:8080/driver/550e8400-e29b-41d4-a716-446655440001/location
```

### Test WebSocket live tracking
Open `scripts/test-ws.html` in your browser (just double-click the file).
Paste the driver UUID and click Connect.

Then run the simulator in Terminal 3:
```bash
chmod +x scripts/simulate.sh
./scripts/simulate.sh
```

You should see GPS updates streaming live in the browser.

---

## Step 10 (Optional): Run with Full Docker

Instead of running producer/consumer manually, build and run everything:

```bash
# Build and start all 5 services
docker compose up --build

# In a separate terminal, apply the schema
docker exec -i location-tracker-cassandra-1 cqlsh < schema.cql
```

---

## Common Errors & Fixes

### "connection refused" on Cassandra
Cassandra is still booting. Wait 30s and retry.
```bash
docker compose logs cassandra | grep "listening for CQL"
```

### "kafka: connection refused"
Kafka isn't up yet. Check:
```bash
docker compose logs kafka | tail -10
```

### go mod tidy fails
Make sure you're in the `location-tracker/` folder (the one with `go.mod`).

### Port 8080 already in use
Something else is using that port.
```bash
lsof -i :8080        # find what's using it
kill -9 <PID>        # kill it
```

### schema.cql: "No hosts provided"
The cassandra container isn't ready yet. Wait longer, then retry Step 6.

### WebSocket page shows "Connection error"
Make sure the producer is running (`go run ./producer/main.go`).
The WebSocket connects to `localhost:8080`.

---

## Folder Layout Summary

```
Your machine
+-- location-tracker/        <- everything goes here
    +-- go.mod               <- module definition + dependencies
    +-- go.sum               <- auto-generated after go mod tidy
    +-- docker-compose.yml   <- runs Kafka, Cassandra, Redis, (optionally producer+consumer)
    +-- schema.cql           <- Cassandra table definition
    +-- models/
    |   +-- location.go
    +-- db/
    |   +-- cassandra.go
    |   +-- redis.go
    +-- producer/
    |   +-- main.go
    |   +-- ws.go
    |   +-- Dockerfile
    +-- consumer/
    |   +-- main.go
    |   +-- Dockerfile
    +-- scripts/
        +-- simulate.sh      <- fake GPS events
        +-- benchmark.sh     <- throughput test
        +-- test-ws.html     <- browser WebSocket tester
```

---

## Quick Reference -- Commands You'll Use Most

```bash
# Start infrastructure
docker compose up zookeeper kafka cassandra redis -d

# Apply schema (do once after cassandra boots)
docker exec -i location-tracker-cassandra-1 cqlsh < schema.cql

# Run consumer (Terminal 1)
go run ./consumer/main.go

# Run producer (Terminal 2)
go run ./producer/main.go

# Simulate GPS events (Terminal 3)
./scripts/simulate.sh

# Stop everything
docker compose down
```
