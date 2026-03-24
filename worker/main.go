package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/joho/godotenv"
	"github.com/pranav/location-tracker/db"
	"github.com/pranav/location-tracker/models"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

const (
	maxRetries = 3
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	_ = godotenv.Load()
	setupLogging()

	kafkaBroker := envOr("KAFKA_BROKER", "localhost:9092")
	kafkaTopic := envOr("KAFKA_TOPIC", "gps.updates")
	consumerGroup := envOr("CONSUMER_GROUP", "cassandra-writer")
	cassandraHost := envOr("CASSANDRA_HOSTS", "localhost")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	metricsAddr := envOr("METRICS_ADDR", ":9091")

	// 1. Connect to Cassandra
	session, err := waitForCassandra(cassandraHost, 10)
	if err != nil {
		log.Fatal().Err(err).Msg("cassandra not ready")
	}
	defer session.Close()
	log.Info().Msg("Connected to Cassandra")

	// 2. Connect to Redis
	rdb := db.NewRedisClient(redisAddr)
	defer rdb.Close()
	log.Info().Msg("Connected to Redis")

	// 3. Setup DLQ
	dlq := NewDLQWriter(kafkaBroker)
	defer dlq.Close()
	log.Info().Msg("DLQ writer ready")

	// 4. Setup Kafka Reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaBroker},
		Topic:          kafkaTopic,
		GroupID:        consumerGroup,
		CommitInterval: time.Second,
	})
	defer reader.Close()

	// 5. Start Metrics
	startMetricsServer(metricsAddr)
	log.Info().Str("addr", metricsAddr).Msg("Metrics server started")

	// 6. Main Loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info().Msg("Worker started, consuming events...")
		for {
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					log.Info().Msg("Kafka reader context cancelled, exiting goroutine.")
					return
				}
				log.Error().Err(err).Msg("kafka read error")
				continue
			}

			go processMessage(ctx, session, rdb, dlq, msg)
		}
	}()

	<-stop
	log.Info().Msg("Shutting down worker...")
	cancel()
	time.Sleep(2 * time.Second) // Small buffer to finish in-flight
	log.Info().Msg("Worker shut down gracefully.")
}

func processMessage(ctx context.Context, session *gocql.Session, rdb *redis.Client, dlq *DLQWriter, msg kafka.Message) {
	var event models.LocationEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		dlq.Send(ctx, models.LocationEvent{}, fmt.Sprintf("unmarshal error: %v", err), 0)
		dlqSent.Inc()
		log.Error().Err(err).Bytes("message_value", msg.Value).Msg("Failed to unmarshal Kafka message, sent to DLQ")
		return
	}

	start := time.Now()
	activeDrivers.Inc() // simplified tracking for now

	// Step 1: persist full history to Cassandra (with retry)
	if err := insertWithRetry(session, event); err != nil {
		dlq.Send(ctx, event, fmt.Sprintf("cassandra insert failed: %v", err), maxRetries)
		dlqSent.Inc()
		eventsFailed.Inc()
		log.Error().Err(err).Str("driver_id", event.DriverID).Msg("Failed to insert into Cassandra after retries, sent to DLQ")
		return
	}

	duration := time.Since(start).Seconds()
	cassandraInsertDuration.Observe(duration)
	eventsProcessed.Inc()

	if !event.RecordedAt.IsZero() {
		lag := time.Now().UTC().Sub(event.RecordedAt.UTC()).Seconds()
		processingLagSeconds.Observe(lag)
	}

	// Step 2: overwrite latest position in Redis (best-effort)
	if err := db.SetLatestLocation(ctx, rdb, event); err != nil {
		log.Warn().Err(err).Str("driver_id", event.DriverID).Msg("redis SET failed (non-fatal)")
	}

	// Step 3: publish to Redis pub/sub for WebSocket push (best-effort)
	if err := db.PublishLocation(ctx, rdb, event); err != nil {
		log.Warn().Err(err).Str("driver_id", event.DriverID).Msg("redis PUBLISH failed (non-fatal)")
	}

	log.Debug().Str("driver_id", event.DriverID).Float64("lat", event.Lat).Float64("lng", event.Lng).Float32("speed", event.Speed).Msg("Saved location event")
}


func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	level, _ := zerolog.ParseLevel(envOr("LOG_LEVEL", "info"))
	zerolog.SetGlobalLevel(level)
}

func waitForCassandra(host string, maxAttempts int) (*gocql.Session, error) {
	var lastErr error
	for i := 1; i <= maxAttempts; i++ {
		session, err := db.NewSession(host)
		if err == nil {
			return session, nil
		}
		lastErr = err
		log.Warn().Err(err).Msgf("Cassandra not ready (attempt %d/%d)", i, maxAttempts)
		time.Sleep(5 * time.Second)
	}
	return nil, fmt.Errorf("timeout waiting for cassandra: %w", lastErr)
}

func insertWithRetry(session *gocql.Session, event models.LocationEvent) error {
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = db.InsertLocation(session, event)
		if err == nil {
			return nil
		}
		cassandraRetries.Inc()
		wait := time.Duration(attempt*attempt) * 100 * time.Millisecond
		log.Warn().Err(err).Int("attempt", attempt).Int("max_retries", maxRetries).Dur("wait", wait).Msg("Cassandra insert failed, retrying")
		time.Sleep(wait)
	}
	return err
}
