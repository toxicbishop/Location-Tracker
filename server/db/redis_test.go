package db

import (
	"context"
	"testing"
	"github.com/pranav/location-tracker/models"
	"github.com/alicebob/miniredis/v2"
)

func TestRedisLatestLocation(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer s.Close()

	rdb := NewRedisClient(s.Addr())
	ctx := context.Background()

	event := models.LocationEvent{
		DriverID: "d1",
		Lat:      12.0,
		Lng:      77.0,
		Speed:    50.0,
	}

	if err := SetLatestLocation(ctx, rdb, event); err != nil {
		t.Fatalf("SetLatestLocation failed: %v", err)
	}

	got, err := GetLatestLocation(ctx, rdb, "d1")
	if err != nil {
		t.Fatalf("GetLatestLocation failed: %v", err)
	}

	if got == nil || got.DriverID != "d1" {
		t.Errorf("got %v, want driver_id d1", got)
	}
}
