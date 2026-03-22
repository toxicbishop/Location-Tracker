package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pranav/location-tracker/models"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and returns a Redis client.
// Call client.Close() when done.
func NewRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: addr, // e.g. "localhost:6379"
		DB:   0,
	})
}

// redisKey returns the Redis key for a driver's latest location.
// Namespaced to avoid collisions if you add other Redis data later.
func redisKey(driverID string) string {
	return "driver:latest:" + driverID
}

// SetLatestLocation stores only the most recent GPS event for a driver.
// This overwrites whatever was there before — we only care about current position.
// No TTL set here: if a driver goes offline, the last known location stays
// until they send a new one (useful for "last seen" features).
func SetLatestLocation(ctx context.Context, rdb *redis.Client, event models.LocationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal location: %w", err)
	}

	return rdb.Set(ctx, redisKey(event.DriverID), payload, 0).Err()
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

// PublishLocation broadcasts a location update to anyone watching a driver.
func PublishLocation(ctx context.Context, rdb *redis.Client, event models.LocationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal for publish: %w", err)
	}

	channel := "driver:updates:" + event.DriverID
	return rdb.Publish(ctx, channel, payload).Err()
}

// SubscribeToDriver returns a Redis PubSub object for a specific driver.
func SubscribeToDriver(ctx context.Context, rdb *redis.Client, driverID string) *redis.PubSub {
	channel := "driver:updates:" + driverID
	return rdb.Subscribe(ctx, channel)
}

