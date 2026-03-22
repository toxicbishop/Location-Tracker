package models

import "time"

// LocationEvent is the structure published to Kafka
// and persisted to Cassandra.
type LocationEvent struct {
	DriverID   string    `json:"driver_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Speed      float32   `json:"speed"`
	RecordedAt time.Time `json:"recorded_at"`
}
