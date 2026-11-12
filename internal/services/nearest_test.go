package services

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

func TestFindNearestAvailableFiltersDutyStatusAndType(t *testing.T) {
	dept := uuid.New()
	otherDept := uuid.New()
	engine := uuid.New()
	medic := uuid.New()
	offDuty := uuid.New()
	dispatched := uuid.New()
	available := uuid.New()
	otherType := uuid.New()
	far := uuid.New()
	dispOp := uuid.New()
	availOp := uuid.New()
	otherOp := uuid.New()
	farOp := uuid.New()

	lookup := &fakeLookup{loc: map[uuid.UUID]*repository.ResourceLocation{
		offDuty:    {DepartmentID: dept, ResourceTypeID: engine, Status: models.ResourceAvailable},
		dispatched: {DepartmentID: dept, ResourceTypeID: engine, Status: models.ResourceDispatched},
		available:  {DepartmentID: dept, ResourceTypeID: engine, Status: models.ResourceAvailable},
		otherType:  {DepartmentID: dept, ResourceTypeID: medic, Status: models.ResourceAvailable},
		far:        {DepartmentID: dept, ResourceTypeID: engine, Status: models.ResourceAvailable},
	}, op: map[uuid.UUID]*uuid.UUID{
		dispatched: &dispOp,
		available:  &availOp,
		otherType:  &otherOp,
		far:        &farOp,
	}}
	store := newMemGeo()
	svc := NewNearestService(store, lookup, 10000)
	ctx := context.Background()
	when := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	originLat, originLng := 12.9716, 77.5946

	for _, row := range []struct {
		id  uuid.UUID
		lat float64
	}{
		{offDuty, originLat + 0.0001},
		{dispatched, originLat + 0.0004},
		{available, originLat + 0.0008},
		{otherType, originLat + 0.0012},
		{far, originLat + 0.02},
	} {
		if _, err := store.SavePosition(ctx, dept, row.id, row.lat, originLng, when); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SavePosition(ctx, otherDept, uuid.New(), originLat, originLng, when); err != nil {
		t.Fatal(err)
	}

	got, err := svc.FindNearestAvailable(ctx, originLat, originLng, nil, dept, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ResourceID != available || got[1].ResourceID != otherType || got[2].ResourceID != far {
		t.Fatalf("nearest = %#v", idsOf(got))
	}
	if got[0].OperatorID != availOp || got[0].Status != models.ResourceAvailable {
		t.Fatalf("nearest row %#v", got[0])
	}
	if got[0].DistanceMeters >= got[1].DistanceMeters {
		t.Fatalf("order distances %v %v", got[0].DistanceMeters, got[1].DistanceMeters)
	}
	if lookup.opCalls[available] != 1 {
		t.Fatalf("operator lookups = %d, want one fill then a cache hit", lookup.opCalls[available])
	}
	one, err := svc.FindNearestAvailable(ctx, originLat, originLng, nil, dept, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].ResourceID != available {
		t.Fatalf("limit 1 = %#v", idsOf(one))
	}
	if lookup.opCalls[available] != 1 {
		t.Fatalf("second query hit postgres again: %d", lookup.opCalls[available])
	}

	filtered, err := svc.FindNearestAvailable(ctx, originLat, originLng, &engine, dept, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 || filtered[0].ResourceID != available {
		t.Fatalf("type filter %#v", idsOf(filtered))
	}
	medics, err := svc.FindNearestAvailable(ctx, originLat, originLng, &medic, dept, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(medics) != 1 || medics[0].ResourceID != otherType {
		t.Fatalf("medic filter %#v", idsOf(medics))
	}

	lookup.op[available] = nil
	if err := svc.RefreshOperator(ctx, available); err != nil {
		t.Fatal(err)
	}
	afterEnd, err := svc.FindNearestAvailable(ctx, originLat, originLng, nil, dept, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEnd) != 2 || afterEnd[0].ResourceID != otherType || afterEnd[1].ResourceID != far {
		t.Fatalf("after shift end %#v", idsOf(afterEnd))
	}

	if err := svc.RememberResource(ctx, far, models.ResourceMaintenance, engine); err != nil {
		t.Fatal(err)
	}
	afterStatus, err := svc.FindNearestAvailable(ctx, originLat, originLng, nil, dept, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterStatus) != 1 || afterStatus[0].ResourceID != otherType {
		t.Fatalf("maintenance resource still returned %#v", idsOf(afterStatus))
	}
}

func TestTelemetryKeepsTheNewerPosition(t *testing.T) {
	dept := uuid.New()
	resourceID := uuid.New()
	lookup := &fakeLookup{loc: map[uuid.UUID]*repository.ResourceLocation{
		resourceID: {DepartmentID: dept, ResourceTypeID: uuid.New(), Status: models.ResourceAvailable},
	}}
	store := newMemGeo()
	svc := NewNearestService(store, lookup, 50000)
	ctx := context.Background()
	subject := "telemetry.location." + resourceID.String()

	newer, _ := json.Marshal(map[string]any{
		"lat": 12.9716, "lng": 77.5946, "fixTime": "2026-11-03T10:00:00Z",
	})
	older, _ := json.Marshal(map[string]any{
		"latitude": 1.0, "longitude": 2.0, "timestamp": "2026-11-03T09:00:00Z",
	})
	if err := svc.HandleTelemetry(ctx, subject, newer); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleTelemetry(ctx, subject, older); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	got := store.pos[resourceID]
	store.mu.Unlock()
	if math.Abs(got.lat-12.9716) > 0.00001 || math.Abs(got.lng-77.5946) > 0.00001 {
		t.Fatalf("position lat=%v lng=%v", got.lat, got.lng)
	}
	if err := svc.HandleTelemetry(ctx, "telemetry.location.not-a-uuid", newer); err != nil {
		t.Fatal(err)
	}
}

type memPos struct {
	dept uuid.UUID
	lat  float64
	lng  float64
	ms   int64
}

type memOp struct {
	hit bool
	id  *uuid.UUID
}

type memState struct {
	hit    bool
	status string
	typeID uuid.UUID
}

type memGeo struct {
	mu    sync.Mutex
	pos   map[uuid.UUID]memPos
	op    map[uuid.UUID]memOp
	state map[uuid.UUID]memState
}

func newMemGeo() *memGeo {
	return &memGeo{
		pos:   map[uuid.UUID]memPos{},
		op:    map[uuid.UUID]memOp{},
		state: map[uuid.UUID]memState{},
	}
}

func (m *memGeo) SavePosition(_ context.Context, departmentID, resourceID uuid.UUID, latitude, longitude float64, fixTime time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := fixTime.UTC().UnixMilli()
	if prev, ok := m.pos[resourceID]; ok && prev.ms >= ms {
		return false, nil
	}
	m.pos[resourceID] = memPos{dept: departmentID, lat: latitude, lng: longitude, ms: ms}
	return true, nil
}

func (m *memGeo) Search(_ context.Context, departmentID uuid.UUID, latitude, longitude, radiusMeters float64) ([]models.GeoCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.GeoCandidate, 0)
	for id, pos := range m.pos {
		if pos.dept != departmentID {
			continue
		}
		dist := haversine(latitude, longitude, pos.lat, pos.lng)
		if dist > radiusMeters {
			continue
		}
		out = append(out, models.GeoCandidate{ResourceID: id, DistanceM: dist})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceM < out[j].DistanceM })
	return out, nil
}

func (m *memGeo) Operator(_ context.Context, resourceID uuid.UUID) (*uuid.UUID, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.op[resourceID]
	if !ok || !row.hit {
		return nil, false, nil
	}
	return row.id, true, nil
}

func (m *memGeo) PutOperator(_ context.Context, resourceID uuid.UUID, operatorID *uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if operatorID == nil || *operatorID == uuid.Nil {
		delete(m.op, resourceID)
		return nil
	}
	m.op[resourceID] = memOp{hit: true, id: operatorID}
	return nil
}

func (m *memGeo) ResourceState(_ context.Context, resourceID uuid.UUID) (string, uuid.UUID, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.state[resourceID]
	if !ok || !row.hit {
		return "", uuid.Nil, false, nil
	}
	return row.status, row.typeID, true, nil
}

func (m *memGeo) PutResourceState(_ context.Context, resourceID uuid.UUID, status string, resourceTypeID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state[resourceID] = memState{hit: true, status: status, typeID: resourceTypeID}
	return nil
}

func (m *memGeo) Forget(_ context.Context, resourceID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pos, resourceID)
	delete(m.op, resourceID)
	delete(m.state, resourceID)
	return nil
}

type fakeLookup struct {
	mu      sync.Mutex
	loc     map[uuid.UUID]*repository.ResourceLocation
	op      map[uuid.UUID]*uuid.UUID
	opCalls map[uuid.UUID]int
	target  *repository.ResourceFixTarget
}

func (f *fakeLookup) Location(_ context.Context, id uuid.UUID) (*repository.ResourceLocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := f.loc[id]
	if row == nil {
		return nil, nil
	}
	copy := *row
	return &copy, nil
}

func (f *fakeLookup) FixTargetByThing(context.Context, uuid.UUID) (*repository.ResourceFixTarget, error) {
	if f.target == nil {
		return nil, nil
	}
	copy := *f.target
	return &copy, nil
}

func (f *fakeLookup) ActiveOperator(_ context.Context, resourceID uuid.UUID) (*uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.opCalls == nil {
		f.opCalls = map[uuid.UUID]int{}
	}
	f.opCalls[resourceID]++
	id := f.op[resourceID]
	if id == nil {
		return nil, nil
	}
	copy := *id
	return &copy, nil
}

func idsOf(rows []models.NearestResource) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		out[i] = row.ResourceID
	}
	return out
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const earth = 6371000.0
	r1 := lat1 * math.Pi / 180
	r2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(r1)*math.Cos(r2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earth * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
