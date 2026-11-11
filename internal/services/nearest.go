package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/natsclient"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const (
	telemetryLocationSubject = "telemetry.location."
	defaultGeoRadiusMeters   = 50000
	telemetryHandleTimeout   = 3 * time.Second
)

// DutyCache refreshes resource:operator:<resourceID> when a shift starts or ends.
type DutyCache interface {
	RefreshOperator(ctx context.Context, resourceID uuid.UUID) error
}

// ResourceStateCache keeps the cached DSP-01 status used by FindNearestAvailable.
type ResourceStateCache interface {
	RememberResource(ctx context.Context, resourceID uuid.UUID, status string, resourceTypeID uuid.UUID) error
	ForgetResource(ctx context.Context, resourceID uuid.UUID) error
}

// resourceGeo is the Redis side of the hot path.
type resourceGeo interface {
	SavePosition(ctx context.Context, departmentID, resourceID uuid.UUID, latitude, longitude float64, fixTime time.Time) (bool, error)
	Search(ctx context.Context, departmentID uuid.UUID, latitude, longitude, radiusMeters float64) ([]models.GeoCandidate, error)
	Operator(ctx context.Context, resourceID uuid.UUID) (*uuid.UUID, bool, error)
	PutOperator(ctx context.Context, resourceID uuid.UUID, operatorID *uuid.UUID) error
	ResourceState(ctx context.Context, resourceID uuid.UUID) (status string, resourceTypeID uuid.UUID, hit bool, err error)
	PutResourceState(ctx context.Context, resourceID uuid.UUID, status string, resourceTypeID uuid.UUID) error
	Forget(ctx context.Context, resourceID uuid.UUID) error
}

// NearestService stores live resource positions and answers FindNearestAvailable.
type NearestService struct {
	store  resourceGeo
	lookup repository.ResourceLookup
	radius float64

	subMu sync.Mutex
	sub   *nats.Subscription
}

func NewNearestService(store resourceGeo, lookup repository.ResourceLookup, radiusMeters float64) *NearestService {
	if radiusMeters <= 0 {
		radiusMeters = defaultGeoRadiusMeters
	}
	return &NearestService{store: store, lookup: lookup, radius: radiusMeters}
}

// Listen subscribes to telemetry.location.<resourceID> and resubscribes when NATS reconnects.
func (s *NearestService) Listen(nc *natsclient.Client) {
	if s == nil || nc == nil {
		return
	}
	nc.AfterConnect(func() {
		s.subscribe(nc)
	})
}

func (s *NearestService) subscribe(nc *natsclient.Client) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
		s.sub = nil
	}
	sub, err := nc.SubscribeCore(telemetryLocationSubject+"*", "telemetry-location", func(msg *nats.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), telemetryHandleTimeout)
		defer cancel()
		if err := s.HandleTelemetry(ctx, msg.Subject, msg.Data); err != nil {
			log.Printf("registry: telemetry failed subject=%s err=%v", msg.Subject, err)
		}
	})
	if err != nil {
		log.Printf("registry: telemetry subscribe failed: %v", err)
		return
	}
	s.sub = sub
	log.Printf("registry: listening on %s*", telemetryLocationSubject)
}

// Close drops the telemetry subscription.
func (s *NearestService) Close() {
	if s == nil {
		return
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
		s.sub = nil
	}
}

type telemetryFix struct {
	Lat       *float64     `json:"lat"`
	Lng       *float64     `json:"lng"`
	Latitude  *float64     `json:"latitude"`
	Longitude *float64     `json:"longitude"`
	Location  *locationFix `json:"location"`
	FixTime   string       `json:"fixTime"`
	FixTime2  string       `json:"fix_time"`
	Timestamp string       `json:"timestamp"`
	Time      string       `json:"time"`
	Valid     *bool        `json:"valid"`
}

type locationFix struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

// HandleTelemetry applies one telemetry.location.<resourceID> message.
// An older fixTime does not replace the stored point.
func (s *NearestService) HandleTelemetry(ctx context.Context, subject string, data []byte) error {
	if s == nil || s.store == nil || s.lookup == nil {
		return errors.New("nearest service is not configured")
	}
	resourceID, ok := resourceIDFromSubject(subject)
	if !ok {
		log.Printf("registry: telemetry ignored reason=subject subject=%s", subject)
		return nil
	}
	lat, lng, when, ok := parseTelemetry(data)
	if !ok {
		log.Printf("registry: telemetry ignored reason=payload resource_id=%s", resourceID)
		return nil
	}
	loc, err := s.lookup.Location(ctx, resourceID)
	if err != nil {
		return err
	}
	if loc == nil {
		log.Printf("registry: telemetry ignored reason=unknown resource resource_id=%s", resourceID)
		return nil
	}
	wrote, err := s.store.SavePosition(ctx, loc.DepartmentID, resourceID, lat, lng, when)
	if err != nil {
		return err
	}
	if !wrote {
		log.Printf("registry: resource position not stored reason=stale resource_id=%s fix_time=%s", resourceID, when.Format(time.RFC3339))
		return nil
	}
	log.Printf("registry: resource position stored resource_id=%s department_id=%s lat=%v lng=%v fix_time=%s",
		resourceID, loc.DepartmentID, lat, lng, when.Format(time.RFC3339))
	return nil
}

// FindNearestAvailable returns up to limit on-duty resources with status available,
// inside the configured radius, nearest first.
// An off-duty resource and a resource whose status is not available are left out
// even when their last position is closer than a resource that is returned.
func (s *NearestService) FindNearestAvailable(ctx context.Context, lat, lng float64, resourceTypeFilter *uuid.UUID, departmentID uuid.UUID, limit int) ([]models.NearestResource, error) {
	if s == nil || s.store == nil || s.lookup == nil {
		return nil, errors.New("nearest service is not configured")
	}
	fields := map[string]string{}
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		fields["lat"] = "lat must be between -90 and 90"
	}
	if math.IsNaN(lng) || math.IsInf(lng, 0) || lng < -180 || lng > 180 {
		fields["lng"] = "lng must be between -180 and 180"
	}
	if departmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	if limit < 1 {
		fields["limit"] = "limit must be at least 1"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}

	hits, err := s.store.Search(ctx, departmentID, lat, lng, s.radius)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].DistanceM < hits[j].DistanceM
	})
	out := make([]models.NearestResource, 0)
	for _, hit := range hits {
		if len(out) >= limit {
			break
		}
		item, ok, err := s.allow(ctx, hit, resourceTypeFilter)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// RefreshOperator reloads resource:operator:<resourceID> from resource_shift_assignment.
func (s *NearestService) RefreshOperator(ctx context.Context, resourceID uuid.UUID) error {
	if s == nil || s.store == nil || s.lookup == nil || resourceID == uuid.Nil {
		return nil
	}
	operatorID, err := s.lookup.ActiveOperator(ctx, resourceID)
	if err != nil {
		return err
	}
	return s.store.PutOperator(ctx, resourceID, operatorID)
}

// RememberResource caches the DSP-01 status and resource type.
func (s *NearestService) RememberResource(ctx context.Context, resourceID uuid.UUID, status string, resourceTypeID uuid.UUID) error {
	if s == nil || s.store == nil || resourceID == uuid.Nil {
		return nil
	}
	return s.store.PutResourceState(ctx, resourceID, status, resourceTypeID)
}

// ForgetResource removes a deleted resource from the GEO set and the duty and status caches.
func (s *NearestService) ForgetResource(ctx context.Context, resourceID uuid.UUID) error {
	if s == nil || s.store == nil || resourceID == uuid.Nil {
		return nil
	}
	return s.store.Forget(ctx, resourceID)
}

func (s *NearestService) allow(ctx context.Context, hit models.GeoCandidate, resourceTypeFilter *uuid.UUID) (models.NearestResource, bool, error) {
	operatorID, err := s.operatorOnDuty(ctx, hit.ResourceID)
	if err != nil {
		return models.NearestResource{}, false, err
	}
	if operatorID == nil {
		return models.NearestResource{}, false, nil
	}
	status, typeID, err := s.resourceState(ctx, hit.ResourceID)
	if err != nil {
		return models.NearestResource{}, false, err
	}
	if status == "" {
		return models.NearestResource{}, false, nil
	}
	if status != models.ResourceAvailable {
		return models.NearestResource{}, false, nil
	}
	if resourceTypeFilter != nil && typeID != *resourceTypeFilter {
		return models.NearestResource{}, false, nil
	}
	return models.NearestResource{
		ResourceID:     hit.ResourceID,
		ResourceTypeID: typeID,
		OperatorID:     *operatorID,
		Status:         status,
		DistanceMeters: hit.DistanceM,
	}, true, nil
}

func (s *NearestService) operatorOnDuty(ctx context.Context, resourceID uuid.UUID) (*uuid.UUID, error) {
	operatorID, hit, err := s.store.Operator(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if hit {
		return operatorID, nil
	}
	operatorID, err = s.lookup.ActiveOperator(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if err := s.store.PutOperator(ctx, resourceID, operatorID); err != nil {
		log.Printf("registry: operator cache fill failed resource_id=%s err=%v", resourceID, err)
	}
	return operatorID, nil
}

func (s *NearestService) resourceState(ctx context.Context, resourceID uuid.UUID) (string, uuid.UUID, error) {
	status, typeID, hit, err := s.store.ResourceState(ctx, resourceID)
	if err != nil {
		return "", uuid.Nil, err
	}
	if hit {
		return status, typeID, nil
	}
	loc, err := s.lookup.Location(ctx, resourceID)
	if err != nil {
		return "", uuid.Nil, err
	}
	if loc == nil {
		if err := s.store.Forget(ctx, resourceID); err != nil {
			log.Printf("registry: resource geo forget failed resource_id=%s err=%v", resourceID, err)
		}
		return "", uuid.Nil, nil
	}
	if err := s.store.PutResourceState(ctx, resourceID, loc.Status, loc.ResourceTypeID); err != nil {
		log.Printf("registry: resource state cache fill failed resource_id=%s err=%v", resourceID, err)
	}
	return loc.Status, loc.ResourceTypeID, nil
}

func resourceIDFromSubject(subject string) (uuid.UUID, bool) {
	if !strings.HasPrefix(subject, telemetryLocationSubject) {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(subject[len(telemetryLocationSubject):]))
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func parseTelemetry(data []byte) (float64, float64, time.Time, bool) {
	var body telemetryFix
	if err := json.Unmarshal(data, &body); err != nil {
		return 0, 0, time.Time{}, false
	}
	if body.Valid != nil && !*body.Valid {
		return 0, 0, time.Time{}, false
	}
	lat, lng, ok := telemetryCoords(body)
	if !ok {
		return 0, 0, time.Time{}, false
	}
	when, ok := telemetryTime(body)
	if !ok {
		return 0, 0, time.Time{}, false
	}
	return lat, lng, when, true
}

func telemetryCoords(body telemetryFix) (float64, float64, bool) {
	lat, lng := body.Lat, body.Lng
	if lat == nil {
		lat = body.Latitude
	}
	if lng == nil {
		lng = body.Longitude
	}
	if (lat == nil || lng == nil) && body.Location != nil {
		if lat == nil {
			lat = body.Location.Lat
		}
		if lng == nil {
			lng = body.Location.Lng
		}
	}
	if lat == nil || lng == nil {
		return 0, 0, false
	}
	if math.IsNaN(*lat) || math.IsInf(*lat, 0) || *lat < -90 || *lat > 90 {
		return 0, 0, false
	}
	if math.IsNaN(*lng) || math.IsInf(*lng, 0) || *lng < -180 || *lng > 180 {
		return 0, 0, false
	}
	return *lat, *lng, true
}

func telemetryTime(body telemetryFix) (time.Time, bool) {
	raw := strings.TrimSpace(body.FixTime)
	if raw == "" {
		raw = strings.TrimSpace(body.FixTime2)
	}
	if raw == "" {
		raw = strings.TrimSpace(body.Timestamp)
	}
	if raw == "" {
		raw = strings.TrimSpace(body.Time)
	}
	if raw == "" {
		return time.Now().UTC(), true
	}
	when, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return when.UTC(), true
}
