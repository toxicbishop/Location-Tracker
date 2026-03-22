package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gocql/gocql"
	"github.com/pranav/location-tracker/db"
	"github.com/pranav/location-tracker/models"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const (
	kafkaTopic    = "gps.updates"
	consumerGroup = "cassandra-writer"
	maxRetries    = 3
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	kafkaBroker   := envOr("KAFKA_BROKER", "localhost:9092")
	cassandraHost := envOr("CASSANDRA_HOST", "localhost")
	redisAddr     := envOr("REDIS_ADDR", "localhost:6379")

	session, err := waitForCassandra(cassandraHost, 10)
	if err != nil {
		log.Fatalf("cassandra not ready: %v", err)
	}
	defer session.Close()
	log.Println("Connected to Cassandra")

	rdb := db.NewRedisClient(redisAddr)
	defer rdb.Close()
	log.Println("Connected to Redis")

	// DLQ writer — publishes unprocessable events to gps.updates.dlq
	dlq := NewDLQWriter(kafkaBroker)
	defer dlq.Close()
	log.Println("DLQ writer ready")

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaBroker},
		Topic:          kafkaTopic,
		GroupID:        consumerGroup,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
	})
	defer reader.Close()
	log.Println("Consumer ready — waiting for GPS events...")

	ctx := context.Background()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("kafka read error: %v", err)
			continue
		}

		var event models.LocationEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			// Malformed JSON can never be fixed by retrying — send straight to DLQ
			dlq.Send(ctx, models.LocationEvent{}, fmt.Sprintf("unmarshal error: %v", err), 0)
			continue
		}

		// Step 1: persist full history to Cassandra (with retry)
		if err := insertWithRetry(session, event); err != nil {
			// All retries exhausted — don't drop the event, send to DLQ
			dlq.Send(ctx, event, fmt.Sprintf("cassandra insert failed: %v", err), maxRetries)
			continue
		}

		// Step 2: overwrite latest position in Redis (best-effort)
		if err := db.SetLatestLocation(ctx, rdb, event); err != nil {
			log.Printf("redis SET failed for driver %s (non-fatal): %v", event.DriverID, err)
		}

		// Step 3: publish to Redis pub/sub for WebSocket push (best-effort)
		if err := db.PublishLocation(ctx, rdb, event); err != nil {
			log.Printf("redis PUBLISH failed for driver %s (non-fatal): %v", event.DriverID, err)
		}

		log.Printf("Saved: driver=%s lat=%.4f lng=%.4f speed=%.1f",
			event.DriverID, event.Lat, event.Lng, event.Speed)
	}
}

// insertWithRetry retries db.InsertLocation with exponential backoff.
func insertWithRetry(session *gocql.Session, event models.LocationEvent) error {
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = db.InsertLocation(session, event)
		if err == nil {
			return nil
		}
		wait := time.Duration(attempt*attempt) * 100 * time.Millisecond
		log.Printf("insert attempt %d/%d failed: %v — retrying in %v", attempt, maxRetries, err, wait)
		time.Sleep(wait)
	}
	return err
}

// waitForCassandra retries the connection until Cassandra is up.
func waitForCassandra(host string, maxAttempts int) (*gocql.Session, error) {
	var lastErr error
	for i := 1; i <= maxAttempts; i++ {
		session, err := db.NewSession(host)
		if err == nil {
			return session, nil
		}
		lastErr = err
		log.Printf("Cassandra not ready (attempt %d/%d): %v", i, maxAttempts, err)
		time.Sleep(5 * time.Second)
	}
	return nil, lastErr
}

// keep redis import — used indirectly via db.NewRedisClient
var _ *redis.Client
