package db

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/pranav/location-tracker/models"
)

// NewSession creates and returns a Cassandra session.
func NewSession(host string) (*gocql.Session, error) {
	cluster := gocql.NewCluster(host)
	cluster.Keyspace = "rideshare"
	cluster.Consistency = gocql.Quorum
	cluster.ProtoVersion = 4
	cluster.ConnectTimeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("cassandra connect: %w", err)
	}
	return session, nil
}

// InsertLocation writes a single GPS event to Cassandra.
// Cassandra handles TTL automatically (set in schema).
func InsertLocation(session *gocql.Session, e models.LocationEvent) error {
	driverUUID, err := gocql.ParseUUID(e.DriverID)
	if err != nil {
		return fmt.Errorf("invalid driver_id uuid: %w", err)
	}

	return session.Query(`
		INSERT INTO driver_locations (driver_id, recorded_at, lat, lng, speed)
		VALUES (?, ?, ?, ?, ?)`,
		driverUUID,
		e.RecordedAt,
		e.Lat,
		e.Lng,
		e.Speed,
	).Exec()
}

// GetRecentLocations fetches GPS events for a driver in the last N minutes.
func GetRecentLocations(session *gocql.Session, driverID string, minutes int) ([]models.LocationEvent, error) {
	driverUUID, err := gocql.ParseUUID(driverID)
	if err != nil {
		return nil, fmt.Errorf("invalid driver_id: %w", err)
	}

	since := time.Now().Add(-time.Duration(minutes) * time.Minute)

	iter := session.Query(`
		SELECT recorded_at, lat, lng, speed
		FROM driver_locations
		WHERE driver_id = ? AND recorded_at >= ?`,
		driverUUID, since,
	).Iter()

	var results []models.LocationEvent
	var e models.LocationEvent
	e.DriverID = driverID

	for iter.Scan(&e.RecordedAt, &e.Lat, &e.Lng, &e.Speed) {
		results = append(results, e)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("cassandra iter: %w", err)
	}
	return results, nil
}
