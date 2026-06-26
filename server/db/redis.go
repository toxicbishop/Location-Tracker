package db

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pranav/location-tracker/models"
	"github.com/redis/go-redis/v9"
)


// NewRedisClient creates and returns a Redis client.
func NewRedisClient(addr string) *redis.Client {
	opts := &redis.Options{
		Addr: addr,
		DB:   0,
	}

	if os.Getenv("REDIS_TLS_ENABLE") == "true" {
		opts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // For demo/internal use
		}
	}

	return redis.NewClient(opts)
}


// redisKey returns the Redis key for a driver's latest location.
// Namespaced to avoid collisions if you add other Redis data later.
func redisKey(driverID string) string {
	return "driver:latest:" + driverID
}

// driverTTL is how long a driver's latest-location key lives in Redis.
// If a driver goes offline, their key expires and they stop appearing in
// GetNearbyDrivers results within this window.
const driverTTL = 60 * time.Second

// SetLatestLocation stores only the most recent GPS event for a driver.
// It also updates the Redis Geospatial index for proximity queries.
func SetLatestLocation(ctx context.Context, rdb *redis.Client, event models.LocationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal location: %w", err)
	}

	// 1. Store the JSON with a 60s TTL.
	// When a driver goes offline, their key naturally expires — no cleanup job needed.
	if err := rdb.Set(ctx, redisKey(event.DriverID), payload, driverTTL).Err(); err != nil {
		return err
	}

	// 2. Add to GEO index for proximity searches (GEORADIUS/GEOSEARCH)
	return rdb.GeoAdd(ctx, "drivers:geo", &redis.GeoLocation{
		Name:      event.DriverID,
		Longitude: event.Lng,
		Latitude:  event.Lat,
	}).Err()
}

// GetNearbyDrivers finds drivers within a certain radius (in km) of a location.
// It filters out stale drivers whose latest-location Redis key has expired (TTL elapsed).
// This handles the case where a GEO member persists in the sorted set even after
// a driver goes offline — the key expiry is the authoritative freshness signal.
func GetNearbyDrivers(ctx context.Context, rdb *redis.Client, lat, lng, radiusKm float64) ([]string, error) {
	res, err := rdb.GeoRadius(ctx, "drivers:geo", lng, lat, &redis.GeoRadiusQuery{
		Radius: radiusKm,
		Unit:   "km",
	}).Result()

	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return []string{}, nil
	}

	// Pipeline EXISTS checks for all candidate drivers in one round-trip.
	// A driver whose latest-location key has expired is stale and excluded.
	pipe := rdb.Pipeline()
	cmds := make([]*redis.IntCmd, len(res))
	for i, loc := range res {
		cmds[i] = pipe.Exists(ctx, redisKey(loc.Name))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("stale driver pipeline: %w", err)
	}

	var active []string
	for i, loc := range res {
		if cmds[i].Val() > 0 {
			active = append(active, loc.Name)
		}
	}
	return active, nil
}


// GetLatestLocation retrieves the most recent GPS event for a driver from Redis.
// Returns (nil, nil) if the driver has no cached location yet.
func GetLatestLocation(ctx context.Context, rdb *redis.Client, driverID string) (*models.LocationEvent, error) {
	val, err := rdb.Get(ctx, redisKey(driverID)).Result()
	if err == redis.Nil {
		// Key doesn't exist — driver hasn't sent a location yet
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var event models.LocationEvent
	if err := json.Unmarshal([]byte(val), &event); err != nil {
		return nil, fmt.Errorf("unmarshal location: %w", err)
	}
	return &event, nil
}

// PublishLocationToStream appends a location event to a per-driver Redis Stream.
//
// Unlike Pub/Sub, Streams persist messages. A WebSocket handler that restarts
// can resume from its last-seen message ID instead of dropping all updates.
// MAXLEN 200 keeps the stream bounded to ~200 events per driver (roughly 7 minutes
// at 2s/event) — enough for reconnect replay without unbounded memory growth.
func PublishLocationToStream(ctx context.Context, rdb *redis.Client, event models.LocationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal for stream: %w", err)
	}

	stream := "driver:stream:" + event.DriverID
	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 200,
		Approx: true, // ~ is fine for our use case
		Values: map[string]interface{}{
			"payload": string(payload),
		},
	}).Err()
}

// ReadLocationStream reads new messages from a driver's Redis Stream.
// lastID is the last message ID the caller processed.
// Pass "0" to read from the beginning (replay), or "$" for live-only.
// blockDuration controls how long to wait for new messages before returning.
// Returns the new messages and the ID of the last one (to pass on the next call).
func ReadLocationStream(ctx context.Context, rdb *redis.Client, driverID, lastID string, blockMs int) ([]models.LocationEvent, string, error) {
	stream := "driver:stream:" + driverID

	result, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{stream, lastID},
		Count:  20,
		Block:  0, // use context for cancellation; caller manages deadline
	}).Result()

	if err == redis.Nil {
		return nil, lastID, nil
	}
	if err != nil {
		return nil, lastID, fmt.Errorf("xread: %w", err)
	}

	var events []models.LocationEvent
	newLastID := lastID
	for _, stream := range result {
		for _, msg := range stream.Messages {
			rawPayload, ok := msg.Values["payload"].(string)
			if !ok {
				continue
			}
			var event models.LocationEvent
			if err := json.Unmarshal([]byte(rawPayload), &event); err == nil {
				events = append(events, event)
			}
			newLastID = msg.ID
		}
	}
	return events, newLastID, nil
}
