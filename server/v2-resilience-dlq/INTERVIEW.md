# Interview Prep -- Real-time Driver Location Tracker

Use this to prepare for technical interviews. For each question, there's a
strong answer you can give based directly on what you built.

---

## Project Introduction (30-second pitch)

> "I built a real-time GPS event pipeline for a ride-sharing use case.
> Drivers send location updates every 2 seconds. The system uses Kafka as the
> event backbone, Cassandra for time-series history with auto-expiry, Redis as
> a fast cache for current position, and WebSockets for live push to the rider's
> browser -- all written in Go. I'll walk you through any part of the stack."

---

## System Design Questions

**Q: Walk me through what happens when a driver sends a GPS update.**

> "The driver POSTs to `/location`. The producer API validates the payload and
> publishes it to a Kafka topic called `gps.updates`, using the `driver_id` as
> the message key. That key ensures all events for the same driver always route
> to the same partition, preserving ordering. A Go consumer reads from that
> topic using a consumer group, writes the event to Cassandra for history,
> updates a Redis key for the latest position, and publishes to a Redis pub/sub
> channel. Any rider watching that driver via WebSocket receives the update in
> real time."

---

**Q: Why did you use Kafka instead of writing directly to Cassandra?**

> "Two reasons. First, decoupling -- the driver-facing API doesn't need to wait
> for Cassandra to acknowledge a write. The producer just publishes to Kafka and
> returns a 202. This keeps the API fast even under write pressure. Second,
> resilience -- if Cassandra is temporarily slow or the consumer crashes, events
> accumulate in Kafka and are processed when the consumer recovers. Without
> Kafka, those writes would be lost."

---

**Q: Why Cassandra for GPS history and not PostgreSQL?**

> "GPS data is append-only, never updated, and bursty -- imagine thousands of
> drivers sending events every 2 seconds. Cassandra is built for exactly this:
> high write throughput, time-series data, and horizontal scaling.
>
> The schema uses `driver_id` as the partition key, so all rows for a driver
> land on the same node -- range queries like 'last 5 minutes for driver X'
> touch a single partition. I also set a TTL of 24 hours, so Cassandra
> auto-expires old rows with no maintenance job needed.
>
> PostgreSQL would work at small scale but would need heavy tuning -- table
> partitioning, vacuum settings, connection pooling -- to handle sustained
> high-frequency writes across thousands of drivers."

---

**Q: You use both Cassandra and Redis. Isn't that redundant?**

> "No -- they serve different purposes. Cassandra stores the full GPS history
> with time-series ordering. Redis stores only the latest position per driver.
>
> When a rider asks 'where is my driver right now?', I hit Redis -- sub-millisecond
> O(1) key lookup, no disk I/O. When a support team asks 'show me driver X's
> route over the last hour', I query Cassandra.
>
> The Redis write is also best-effort. If it fails, Cassandra still has the data.
> The rider just gets a slightly stale position until the next GPS event, which
> is acceptable."

---

**Q: How does the WebSocket work?**

> "When a rider connects to `/driver/{id}/ws`, the HTTP connection is upgraded
> to a WebSocket using the `gorilla/websocket` library. The handler then
> subscribes to a Redis pub/sub channel -- `driver:updates:{id}` -- and
> immediately sends the latest position from Redis so the rider doesn't see a
> blank screen.
>
> From that point, every time the consumer processes a GPS event, it publishes
> to that channel. Redis delivers it to all subscribed WebSocket handlers, which
> write it to the rider's connection. It's a true push model -- no polling.
>
> When the rider disconnects, a goroutine detects the read error, cancels the
> context, and the subscription is cleaned up."

---

**Q: What happens if the consumer crashes mid-processing?**

> "Kafka's consumer group tracks offsets. The consumer only commits an offset
> after a successful Cassandra insert. If it crashes before committing, Kafka
> replays that message on restart. This gives at-least-once delivery semantics --
> the same event might be processed twice in a crash-recovery scenario, but
> Cassandra inserts are idempotent since we're inserting a row with the exact
> same primary key and values."

---

**Q: How would you scale this system?**

> "The consumer is stateless -- I can run multiple instances with
> `docker-compose up --scale consumer=3`. Kafka distributes partitions across
> them; each partition is handled by exactly one consumer, so there are no
> duplicate writes.
>
> The producer is also stateless, so it scales behind a load balancer.
>
> Cassandra scales horizontally by adding nodes -- replication factor handles
> data distribution automatically.
>
> The WebSocket handlers are per-connection goroutines backed by Redis pub/sub,
> so they scale with the producer instances."

---

**Q: How would you handle a driver that goes offline suddenly?**

> "The last known position stays in Redis indefinitely -- I don't set a TTL on
> the latest-location key. So the rider sees 'last seen at X position' rather
> than an error. In a production system I'd add a `last_seen` timestamp to the
> cached value and show the rider a 'driver offline' indicator if it's older
> than, say, 30 seconds."

---

## Go-Specific Questions

**Q: How do you handle concurrency in the consumer?**

> "The main loop is a single goroutine reading from Kafka sequentially. The
> goroutine-per-message pattern would work too, but it complicates offset
> management -- you'd need to track which offsets are safe to commit. Sequential
> processing keeps it simple; Cassandra's write throughput is high enough that
> the single goroutine isn't the bottleneck.
>
> In the WebSocket handler, I use a goroutine to detect disconnections -- it
> blocks on `ReadMessage` and cancels the context when the rider closes the
> connection. The main handler loop then exits cleanly via a `select` on
> `ctx.Done()`."

---

**Q: Why did you use `gorilla/websocket` instead of the standard library?**

> "Go's standard library doesn't have WebSocket support built in. `gorilla/websocket`
> is the de facto standard -- it handles the HTTP upgrade, fragmentation, ping/pong
> frames, and write deadlines cleanly. I set a 5-second write deadline on each
> frame so a slow client can't block the handler goroutine indefinitely."

---

**Q: How do you avoid goroutine leaks in the WebSocket handler?**

> "Every WebSocket connection gets a `context.WithCancel`. The disconnect-detection
> goroutine calls `cancel()` when it gets a read error. The main handler loop
> selects on `ctx.Done()` and returns, which triggers the deferred `sub.Close()`
> and `conn.Close()`. So when a rider disconnects, both the Redis subscription
> and the goroutine are cleaned up within milliseconds."

---

## Behavioural / Design Trade-off Questions

**Q: What would you do differently if you built this again?**

> "A few things. First, I'd add a schema registry and use Avro or Protobuf for
> the Kafka messages instead of raw JSON -- it enforces contracts between producer
> and consumer. Second, I'd add structured logging with `zap` or `slog` instead
> of `log.Printf`. Third, I'd add metrics -- Prometheus counters for events
> processed, Kafka consumer lag, and Redis hit/miss rate. These are the first
> things an SRE would ask for in production."

---

**Q: What's the weakest part of this design?**

> "The Redis pub/sub fan-out. If the producer service restarts while riders are
> connected, all WebSocket connections drop and riders have to reconnect. A more
> robust approach would be to use Redis Streams instead of pub/sub -- Streams
> support consumer groups and message acknowledgment, so a restarting handler
> can resume from where it left off. I kept pub/sub because it's simpler for a
> portfolio project, but I'm aware of the trade-off."

---

## Quick-fire Answers

| Question | Answer |
|---|---|
| What port does the API run on? | 8080 |
| What's the Kafka topic? | `gps.updates` |
| What's the Cassandra keyspace? | `rideshare` |
| How long does GPS data live? | 24 hours (TTL) |
| What's the Redis key format? | `driver:latest:{uuid}` and `driver:updates:{uuid}` |
| What library for WebSockets? | `gorilla/websocket v1.5.1` |
| What library for Kafka? | `segmentio/kafka-go v0.4.47` |
| What library for Cassandra? | `gocql/gocql v1.6.0` |
| What library for Redis? | `redis/go-redis/v9` |
| How do you scale consumers? | `docker-compose up --scale consumer=N` |
| What delivery guarantee? | At-least-once |
