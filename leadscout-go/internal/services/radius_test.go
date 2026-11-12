package services

import (
	"testing"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
)

// Afyonkarahisar centre; one degree of latitude is ~111 km.
const (
	centerLat = 38.6853
	centerLon = 30.6427
)

func TestWithinRadiusKeepsPointInsideCircle(t *testing.T) {
	rows := []*model.Lead{{Name: "x", Latitude: model.Float(38.69), Longitude: model.Float(30.64)}}
	if got := withinRadius(rows, centerLat, centerLon, 5); len(got) != 1 {
		t.Errorf("got %d rows, want 1", len(got))
	}
}

func TestWithinRadiusDropsBBoxCorner(t *testing.T) {
	// ~7 km east: inside the 5 km bounding box corner but outside the circle.
	rows := []*model.Lead{{Name: "x", Latitude: model.Float(38.6853), Longitude: model.Float(30.7273)}}
	if got := withinRadius(rows, centerLat, centerLon, 5); len(got) != 0 {
		t.Errorf("got %d rows, want 0", len(got))
	}
}

func TestWithinRadiusKeepsRowsWithoutCoordinates(t *testing.T) {
	rows := []*model.Lead{{Name: "no coords"}}
	if got := withinRadius(rows, centerLat, centerLon, 5); len(got) != 1 {
		t.Errorf("got %d rows, want 1", len(got))
	}
}
