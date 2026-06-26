package main

import (
	"testing"
	"github.com/pranav/location-tracker/models"
)

func TestValidateLocation(t *testing.T) {
	tests := []struct {
		name    string
		event   models.LocationEvent
		wantErr bool
	}{
		{"valid", models.LocationEvent{DriverID: "d1", Lat: 12.0, Lng: 77.0}, false},
		{"empty driver", models.LocationEvent{DriverID: "", Lat: 12.0, Lng: 77.0}, true},
		{"invalid lat", models.LocationEvent{DriverID: "d1", Lat: 100.0, Lng: 77.0}, true},
		{"invalid lng", models.LocationEvent{DriverID: "d1", Lat: 12.0, Lng: 200.0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateLocation(tt.event); (err != nil) != tt.wantErr {
				t.Errorf("validateLocation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
