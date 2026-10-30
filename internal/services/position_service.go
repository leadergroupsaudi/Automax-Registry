package services

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/automax/registry/internal/models"
)

// PositionStore writes the latest position for a Caugar device id.
// wrote is false when fixTime is not newer than the stored point.
type PositionStore interface {
	SaveLatest(ctx context.Context, deviceID, deviceName string, latitude, longitude float64, fixTime time.Time) (wrote bool, err error)
}

// ThingCoordinateWriter copies a stored live fix onto the linked registry thing.
type ThingCoordinateWriter interface {
	Apply(ctx context.Context, deviceID string, latitude, longitude float64) error
}

type PositionService struct {
	store  PositionStore
	things ThingCoordinateWriter
	now    func() time.Time
}

func NewPositionService(store PositionStore) *PositionService {
	return &PositionService{store: store, now: time.Now}
}

// UseThingCoordinates updates the linked thing after a newer fix is stored.
func (s *PositionService) UseThingCoordinates(w ThingCoordinateWriter) {
	if s == nil {
		return
	}
	s.things = w
}

// Ingest accepts one live fix. A false valid flag and an older fix both succeed without writing.
func (s *PositionService) Ingest(ctx context.Context, req models.PositionIngestRequest) error {
	deviceID := strings.TrimSpace(string(req.DeviceID))
	if deviceID == "" {
		return Invalid("validation failed", map[string]string{"deviceId": "deviceId is required"})
	}
	if req.Valid != nil && !*req.Valid {
		log.Printf("registry: position not stored reason=invalid device_id=%s", deviceID)
		return nil
	}

	fields := map[string]string{}
	deviceName := strings.TrimSpace(req.DeviceName)
	if utf8.RuneCountInString(deviceName) > 255 {
		fields["deviceName"] = "deviceName must be at most 255 characters"
	}
	lat, lng := validateCoords(fields, req.Latitude, req.Longitude)
	fix, fixErr := parseFixTime(req.FixTime)
	if strings.TrimSpace(req.FixTime) == "" {
		fix = s.now().UTC()
		log.Printf("registry: fixTime missing; using server time device_id=%s fix_time=%s", deviceID, fix.Format(time.RFC3339))
	} else if fixErr != nil {
		fields["fixTime"] = "fixTime must be RFC3339"
	}
	if len(fields) > 0 {
		return Invalid("validation failed", fields)
	}

	if s.store == nil {
		return errors.New("position store is not configured")
	}

	wrote, err := s.store.SaveLatest(ctx, deviceID, deviceName, lat, lng, fix)
	if err != nil {
		return err
	}
	if !wrote {
		log.Printf("registry: position not stored reason=stale device_id=%s fix_time=%s", deviceID, fix.Format(time.RFC3339))
		return nil
	}
	log.Printf("registry: position stored device_id=%s device_name=%s lat=%v lng=%v fix_time=%s",
		deviceID, deviceName, lat, lng, fix.Format(time.RFC3339))
	if s.things == nil {
		return nil
	}
	if err := s.things.Apply(ctx, deviceID, lat, lng); err != nil {
		log.Printf("registry: thing coordinates failed device_id=%s err=%v", deviceID, err)
		return err
	}
	return nil
}

func validateCoords(fields map[string]string, lat, lng *float64) (float64, float64) {
	var latV, lngV float64
	if lat == nil {
		fields["latitude"] = "latitude is required"
	} else if math.IsNaN(*lat) || math.IsInf(*lat, 0) || *lat < -90 || *lat > 90 {
		fields["latitude"] = "latitude must be between -90 and 90"
	} else {
		latV = *lat
	}
	if lng == nil {
		fields["longitude"] = "longitude is required"
	} else if math.IsNaN(*lng) || math.IsInf(*lng, 0) || *lng < -180 || *lng > 180 {
		fields["longitude"] = "longitude must be between -180 and 180"
	} else {
		lngV = *lng
	}
	return latV, lngV
}

func parseFixTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	fix, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return fix.UTC(), nil
}
