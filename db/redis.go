package db

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"

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

// SetLatestLocation stores only the most recent GPS event for a driver.
// It also updates the Redis Geospatial index for proximity queries.
func SetLatestLocation(ctx context.Context, rdb *redis.Client, event models.LocationEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal location: %w", err)
	}

	// 1. Store the JSON for O(1) lookup
	if err := rdb.Set(ctx, redisKey(event.DriverID), payload, 0).Err(); err != nil {
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
func GetNearbyDrivers(ctx context.Context, rdb *redis.Client, lat, lng, radiusKm float64) ([]string, error) {
	res, err := rdb.GeoRadius(ctx, "drivers:geo", lng, lat, &redis.GeoRadiusQuery{
		Radius: radiusKm,
		Unit:   "km",
	}).Result()

	if err != nil {
		return nil, err
	}

	drivers := make([]string, len(res))
	for i, loc := range res {
		drivers[i] = loc.Name
	}
	return drivers, nil
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

