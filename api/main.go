package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

var (
	writer *kafka.Writer
	rdb    *redis.Client
)

func main() {
	// 1. Load configuration
	_ = godotenv.Load()
	setupLogging()

	kafkaBroker := envOr("KAFKA_BROKER", "localhost:9092")
	kafkaTopic := envOr("KAFKA_TOPIC", "gps.updates")
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")
	listenAddr := ":" + envOr("HTTP_PORT", "8080")
	metricsAddr := ":" + envOr("METRICS_PORT", "9090")

	// 2. Initialize Kafka Writer
	writer = &kafka.Writer{
		Addr:         kafka.TCP(kafkaBroker),
		Topic:        kafkaTopic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 10 * time.Millisecond,
		Async:        true,
	}

	// 3. Initialize Redis
	rdb = db.NewRedisClient(redisAddr)

	// 4. Initialize Cassandra
	cassHost := envOr("CASSANDRA_HOSTS", "localhost")
	session, err := db.NewSession(cassHost)
	if err != nil {
		log.Warn().Err(err).Msg("Cassandra not available, /history will fail")
	} else if session != nil {
		defer session.Close()
	}

	// 5. Start Metrics Server
	startMetricsServer(metricsAddr)
	log.Info().Str("addr", metricsAddr).Msg("Metrics server started")

	// 6. Define Routes
	mux := http.NewServeMux()
	mux.HandleFunc("/location", metricsMiddleware("/location", AuthMiddleware(handleLocation)))
	mux.HandleFunc("/location/batch", metricsMiddleware("/location/batch", AuthMiddleware(handleLocationBatch)))
	mux.HandleFunc("/driver/", metricsMiddleware("/driver", handleGetLatestLocation))
	mux.HandleFunc("/driver/history/", metricsMiddleware("/history", handleGetDriverHistory(session)))
	mux.HandleFunc("/drivers/nearby", metricsMiddleware("/nearby", handleGetNearbyDrivers))
	mux.HandleFunc("/ws/driver/", metricsMiddleware("/ws", AuthMiddleware(handleWebSocket)))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	// 7. Graceful Shutdown Handling
	go func() {
		log.Info().Str("addr", listenAddr).Msg("API server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("API server failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info().Msg("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	writer.Close()
	rdb.Close()
	log.Info().Msg("Server stopped")
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	level, _ := zerolog.ParseLevel(envOr("LOG_LEVEL", "info"))
	zerolog.SetGlobalLevel(level)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// POST /location
func handleLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event models.LocationEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateLocation(event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if event.RecordedAt.IsZero() {
		event.RecordedAt = time.Now().UTC()
	}

	payload, _ := json.Marshal(event)
	err := writer.WriteMessages(r.Context(), kafka.Message{
		Key:   []byte(event.DriverID),
		Value: payload,
	})

	if err != nil {
		log.Error().Err(err).Str("driver_id", event.DriverID).Msg("kafka publish failed")
		kafkaPublishErrors.Inc()
		http.Error(w, "failed to publish", http.StatusInternalServerError)
		return
	}

	kafkaPublishTotal.Inc()
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

// POST /location/batch
func handleLocationBatch(w http.ResponseWriter, r *http.Request) {
	var events []models.LocationEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	messages := make([]kafka.Message, 0, len(events))
	for _, e := range events {
		if err := validateLocation(e); err != nil {
			log.Warn().Err(err).Str("driver_id", e.DriverID).Msg("invalid location in batch")
			continue
		}
		if e.RecordedAt.IsZero() {
			e.RecordedAt = time.Now().UTC()
		}
		payload, _ := json.Marshal(e)
		messages = append(messages, kafka.Message{
			Key:   []byte(e.DriverID),
			Value: payload,
		})
	}

	if err := writer.WriteMessages(r.Context(), messages...); err != nil {
		log.Error().Err(err).Msg("batch kafka publish failed")
		http.Error(w, "failed to publish batch", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "accepted", "count": len(messages)})
}

// GET /drivers/nearby?lat=12.97&lng=77.59&radius=5
func handleGetNearbyDrivers(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lngStr := r.URL.Query().Get("lng")
	radStr := r.URL.Query().Get("radius")

	lat, _ := strconv.ParseFloat(latStr, 64)
	lng, _ := strconv.ParseFloat(lngStr, 64)
	radius, _ := strconv.ParseFloat(radStr, 64)

	if radius <= 0 {
		radius = 5.0 // default 5km
	}

	drivers, err := db.GetNearbyDrivers(r.Context(), rdb, lat, lng, radius)
	if err != nil {
		log.Error().Err(err).Msg("redis geo search failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"center": map[string]float64{"lat": lat, "lng": lng},
		"radius": radius,
		"drivers": drivers,
	})
}

// GET /driver/{id}/location
func handleGetLatestLocation(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.Error(w, "driver_id required", http.StatusBadRequest)
		return
	}
	driverID := parts[1]

	event, err := db.GetLatestLocation(r.Context(), rdb, driverID)
	if err != nil {
		log.Error().Err(err).Str("driver_id", driverID).Msg("redis read error")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if event == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}



// GET /driver/{id}/history?minutes=10
func handleGetDriverHistory(session *gocql.Session) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if session == nil {
			http.Error(w, "history storage unavailable", http.StatusServiceUnavailable)
			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			http.Error(w, "driver_id required", http.StatusBadRequest)
			return
		}
		driverID := parts[2]

		minStr := r.URL.Query().Get("minutes")
		minutes, _ := strconv.Atoi(minStr)
		if minutes <= 0 {
			minutes = 10
		}

		events, err := db.GetRecentLocations(session, driverID, minutes)
		if err != nil {
			log.Error().Err(err).Str("driver_id", driverID).Msg("cassandra read error")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	}
}


func validateLocation(e models.LocationEvent) error {
	if e.DriverID == "" {
		return http.ErrBodyNotAllowed 
	}
	if e.Lat < -90 || e.Lat > 90 {
		return http.ErrLineTooLong 
	}
	if e.Lng < -180 || e.Lng > 180 {
		return http.ErrAbortHandler
	}
	return nil
}


