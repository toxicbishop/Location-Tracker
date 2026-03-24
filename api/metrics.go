package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)


var (
	// Total HTTP requests by method, path, and status code
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "producer_http_requests_total",
		Help: "Total HTTP requests handled by the producer",
	}, []string{"method", "path", "status"})

	// How long each HTTP request takes — use this to find slow endpoints
	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "producer_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// Total events successfully published to Kafka
	kafkaPublishTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "producer_kafka_publish_total",
		Help: "Total GPS events successfully published to Kafka",
	})

	// Total Kafka publish failures
	kafkaPublishErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "producer_kafka_publish_errors_total",
		Help: "Total Kafka publish failures",
	})

	// Active WebSocket connections right now
	activeWebSocketConns = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "producer_websocket_connections_active",
		Help: "Number of currently active WebSocket connections",
	})
)

// metricsMiddleware wraps an HTTP handler and records request count + duration.
// Usage: http.HandleFunc("/location", metricsMiddleware("/location", handleLocation))
func metricsMiddleware(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		rw := &statusResponseWriter{ResponseWriter: w, status: 200}
		next(rw, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.status)

		httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	}
}

// statusResponseWriter captures the HTTP status code written by a handler.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements the http.Hijacker interface.
// This is required for WebSocket upgrades to work when using this middleware.
func (rw *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}


// startMetricsServer exposes /metrics on a dedicated port.
// Producer app runs on :8080, metrics on :9090.
func startMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			panic("metrics server failed: " + err.Error())
		}
	}()
}
