package services

import (
	"context"
	"testing"
	"time"

	"github.com/automax/registry/internal/models"
)

type fakeStore struct {
	deviceID   string
	deviceName string
	fix        time.Time
	saved      bool
	lat        float64
	lng        float64
}

func (s *fakeStore) SaveLatest(_ context.Context, deviceID, deviceName string, latitude, longitude float64, fixTime time.Time) (bool, error) {
	if !s.fix.IsZero() && !fixTime.After(s.fix) {
		return false, nil
	}
	s.deviceID = deviceID
	s.deviceName = deviceName
	s.fix = fixTime
	s.lat = latitude
	s.lng = longitude
	s.saved = true
	return true, nil
}

func f64(v float64) *float64 { return &v }

func TestIngestStoresLatestFix(t *testing.T) {
	store := &fakeStore{}
	svc := NewPositionService(store)
	err := svc.Ingest(context.Background(), models.PositionIngestRequest{
		DeviceID:   "428976543",
		DeviceName: "Unit 1",
		Latitude:   f64(12.9716),
		Longitude:  f64(77.5946),
		FixTime:    "2026-10-09T08:30:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.saved || store.deviceID != "428976543" || store.deviceName != "Unit 1" || store.lat != 12.9716 || store.lng != 77.5946 {
		t.Fatalf("stored %#v", store)
	}
	if store.fix.Format(time.RFC3339) != "2026-10-09T08:30:00Z" {
		t.Fatalf("fix %s", store.fix)
	}
}

func TestIngestUsesServerTimeWhenFixTimeMissing(t *testing.T) {
	store := &fakeStore{}
	svc := NewPositionService(store)
	serverNow := time.Date(2026, 10, 9, 12, 15, 0, 0, time.UTC)
	svc.now = func() time.Time { return serverNow }
	err := svc.Ingest(context.Background(), models.PositionIngestRequest{
		DeviceID:  "42897654",
		Latitude:  f64(12.9719),
		Longitude: f64(77.5949),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.saved || !store.fix.Equal(serverNow) || store.lat != 12.9719 {
		t.Fatalf("stored %#v", store)
	}
}

func TestIngestSkipsInvalidAndStaleFixes(t *testing.T) {
	store := &fakeStore{}
	svc := NewPositionService(store)
	valid := false
	if err := svc.Ingest(context.Background(), models.PositionIngestRequest{DeviceID: "428976543", Valid: &valid}); err != nil {
		t.Fatal(err)
	}
	if store.saved {
		t.Fatal("invalid fix was stored")
	}

	if err := svc.Ingest(context.Background(), models.PositionIngestRequest{
		DeviceID: "428976543", Latitude: f64(1), Longitude: f64(2), FixTime: "2026-10-09T08:30:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ingest(context.Background(), models.PositionIngestRequest{
		DeviceID: "428976543", Latitude: f64(9), Longitude: f64(9), FixTime: "2026-10-09T08:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if store.lat != 1 || store.lng != 2 {
		t.Fatalf("older fix overwrote latest: %#v", store)
	}
}
