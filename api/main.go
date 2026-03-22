package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pranav/location-tracker/db"
	"github.com/pranav/location-tracker/models"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const (
	kafkaBroker = "localhost:9092"
	kafkaTopic  = "gps.updates"
	redisAddr   = "localhost:6379"
	listenAddr  = ":8080"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	writer *kafka.Writer
	rdb    *redis.Client
)

func main() {
	writer = &kafka.Writer{
		Addr:         kafka.TCP(kafkaBroker),
		Topic:        kafkaTopic,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
	}
	defer writer.Close()

	rdb = db.NewRedisClient(redisAddr)
	defer rdb.Close()

	metricsAddr := envOr("METRICS_ADDR", ":9090")
	startMetricsServer(metricsAddr)
	log.Printf("Producer metrics on %s", metricsAddr)

	http.HandleFunc("/location", metricsMiddleware("/location", handleLocation))
	http.HandleFunc("/driver/", metricsMiddleware("/driver", handleGetLatestLocation))
	http.HandleFunc("/ws/driver/", metricsMiddleware("/ws", handleWebSocket))
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Producer listening on %s | kafka=%s redis=%s", listenAddr, kafkaBroker, redisAddr)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

// POST /location
// Body: { "driver_id": "<uuid>", "lat": 12.97, "lng": 77.59, "speed": 45.2 }
func handleLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event models.LocationEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if event.DriverID == "" {
		http.Error(w, "driver_id is required", http.StatusBadRequest)
		return
	}

	if event.RecordedAt.IsZero() {
		event.RecordedAt = time.Now().UTC()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		http.Error(w, "serialization error", http.StatusInternalServerError)
		return
	}

	err = writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(event.DriverID),
		Value: payload,
	})
	if err != nil {
		log.Printf("kafka write error: %v", err)
		kafkaPublishErrors.Inc()
		http.Error(w, "failed to publish event", http.StatusInternalServerError)
		return
	}

	kafkaPublishTotal.Inc()
	log.Printf("Published: driver=%s lat=%.4f lng=%.4f", event.DriverID, event.Lat, event.Lng)
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

// GET /driver/{driver_id}/location
// Returns the latest cached position for a driver from Redis.
//
// Why Redis and not Cassandra?
// Cassandra reads require a CQL query, coordinator node lookup, and disk I/O.
// Redis is an in-memory O(1) key lookup — sub-millisecond at any scale.
// A rider app polls this every 2s; it needs to be fast.
func handleGetLatestLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse /driver/{id}/location
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[2] != "location" {
		http.Error(w, "usage: GET /driver/{id}/location", http.StatusBadRequest)
		return
	}
	driverID := parts[1]

	event, err := db.GetLatestLocation(r.Context(), rdb, driverID)
	if err != nil {
		log.Printf("redis read error for driver %s: %v", driverID, err)
		http.Error(w, "failed to fetch location", http.StatusInternalServerError)
		return
	}

	if event == nil {
		http.Error(w, `{"error":"no location found for driver"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}
