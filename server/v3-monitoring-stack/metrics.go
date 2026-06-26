package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// All consumer metrics live here.
// promauto registers them automatically — no manual Register() calls needed.

var (
	// How many events were successfully written to Cassandra
	eventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "consumer_events_processed_total",
		Help: "Total GPS events successfully written to Cassandra",
	})

	// How many events failed after all retries and were sent to DLQ
	eventsFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "consumer_events_failed_total",
		Help: "Total GPS events that failed processing and were sent to DLQ",
	})

	// How many events landed in the DLQ (includes unmarshal failures)
	dlqSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "consumer_dlq_sent_total",
		Help: "Total events published to the dead letter queue",
	})

	// How long Cassandra inserts take — use this to spot slowdowns
	cassandraInsertDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "consumer_cassandra_insert_duration_seconds",
		Help:    "Duration of Cassandra INSERT operations",
		Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
	})

	// How many Cassandra retries happened (not failures — retries)
	cassandraRetries = promauto.NewCounter(prometheus.CounterOpts{
		Name: "consumer_cassandra_retries_total",
		Help: "Total Cassandra insert retries (not final failures)",
	})

	// How many active WebSocket connections exist right now (from consumer's perspective)
	// This is a gauge because it goes up and down
	activeDrivers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "consumer_active_drivers_total",
		Help: "Number of distinct drivers seen in the current session",
	})

	// Lag: time between event's recorded_at and when consumer processed it
	// High values = Kafka consumer is falling behind
	processingLagSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "consumer_processing_lag_seconds",
		Help:    "Time between GPS event recorded_at and consumer processing time",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	})
)

// startMetricsServer exposes /metrics on a separate port so it doesn't
// mix with application traffic. Prometheus scrapes this endpoint.
func startMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			// Non-fatal — metrics server failure shouldn't crash the consumer
			panic("metrics server failed: " + err.Error())
		}
	}()
}
