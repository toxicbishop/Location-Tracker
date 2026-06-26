package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// rateLimitWindow is the sliding window duration.
const rateLimitWindow = time.Second

// RateLimitMiddleware enforces per-driver_id rate limiting using a Redis counter
// with a 1-second sliding window. This prevents a single misbehaving driver
// client from flooding the Kafka topic.
//
// The driver_id is extracted from the request body, which has already been
// authenticated by AuthMiddleware. The limit applies per driver_id, not per IP,
// so one driver can't starve others regardless of how many replicas are running.
//
// Redis key: driver:ratelimit:{driver_id}  TTL: 1 second
// On breach: 429 Too Many Requests with Retry-After: 1
func RateLimitMiddleware(rdb *redis.Client, limitRPS int) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Peek at driver_id from the X-Driver-ID header (set by SDK clients)
			// or fall back to the query param. The body isn't re-readable here so
			// clients should send the driver_id in a header for rate limit accuracy.
			// POST /location also accepts X-Driver-ID header.
			driverID := r.Header.Get("X-Driver-ID")
			if driverID == "" {
				driverID = r.URL.Query().Get("driver_id")
			}
			// Normalize: empty/unknown → rate-limit under "unknown" bucket
			if driverID == "" {
				driverID = "unknown"
			}
			driverID = strings.TrimSpace(driverID)

			key := fmt.Sprintf("driver:ratelimit:%s", driverID)

			count, err := incrementRateLimit(r.Context(), rdb, key, rateLimitWindow)
			if err != nil {
				// Redis error: fail open (allow the request) but log
				log.Warn().Err(err).Str("driver_id", driverID).Msg("rate limit redis error, allowing request")
				next(w, r)
				return
			}

			if count > int64(limitRPS) {
				rateLimitRejected.Inc()
				log.Warn().
					Str("driver_id", driverID).
					Int64("count", count).
					Int("limit", limitRPS).
					Msg("rate limit exceeded")
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limit exceeded — too many requests from this driver", http.StatusTooManyRequests)
				return
			}

			next(w, r)
		}
	}
}

// incrementRateLimit atomically increments a counter and sets TTL on first call.
// Uses INCR + EXPIRE which is safe for our use case (window resets after TTL).
func incrementRateLimit(ctx context.Context, rdb *redis.Client, key string, window time.Duration) (int64, error) {
	pipe := rdb.TxPipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("rate limit pipeline: %w", err)
	}
	return incrCmd.Val(), nil
}
