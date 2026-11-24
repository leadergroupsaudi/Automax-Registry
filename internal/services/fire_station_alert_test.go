package services

import (
	"context"
	"testing"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestFireStationAlertAllActiveStationsInTheDepartment(t *testing.T) {
	dept := uuid.New()
	other := uuid.New()
	active := station("Gufran", dept, true)
	inactive := station("Closed", dept, false)
	away := station("Manga", other, true)
	h := newAlertHarness(active, inactive, away)

	got, err := h.svc.Alert(context.Background(), uuid.New(), models.FireStationAlertRequest{DepartmentID: dept, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Alerted) != 1 || got.Alerted[0].ServicePointID != active.ID || got.Alerted[0].Name != "Gufran" || got.Alerted[0].RadioID != "R1" {
		t.Fatalf("alerted %#v", got.Alerted)
	}
	if len(h.alerts.rows) != 1 {
		t.Fatalf("stored %d", len(h.alerts.rows))
	}
}

func TestFireStationAlertSelectedStations(t *testing.T) {
	dept := uuid.New()
	first := station("Gufran", dept, true)
	second := station("Manga", dept, true)
	h := newAlertHarness(first, second)
	incident := uuid.New()

	got, err := h.svc.Alert(context.Background(), incident, models.FireStationAlertRequest{
		DepartmentID: dept, ServicePointIDs: []uuid.UUID{second.ID, second.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Alerted) != 1 || got.Alerted[0].Name != "Manga" {
		t.Fatalf("alerted %#v", got.Alerted)
	}
	_, err = h.svc.Alert(context.Background(), incident, models.FireStationAlertRequest{
		DepartmentID: dept, ServicePointIDs: []uuid.UUID{second.ID},
	})
	if err == nil || err.Error() != "incident is already assigned to this fire station" {
		t.Fatal(err)
	}
	if se, ok := err.(*Error); !ok || se.Kind != KindConflict {
		t.Fatalf("kind %#v", err)
	}
	if len(h.alerts.rows) != 1 {
		t.Fatalf("stored %d", len(h.alerts.rows))
	}
	added, err := h.svc.Alert(context.Background(), incident, models.FireStationAlertRequest{
		DepartmentID: dept, ServicePointIDs: []uuid.UUID{first.ID},
	})
	if err != nil || len(added.Alerted) != 1 || added.Alerted[0].Name != "Gufran" || len(h.alerts.rows) != 2 {
		t.Fatalf("added %#v stored %d err %v", added, len(h.alerts.rows), err)
	}
}

func TestFireStationAlertRejectsABadSelectionBeforeSaving(t *testing.T) {
	dept := uuid.New()
	active := station("Gufran", dept, true)
	inactive := station("Closed", dept, false)
	away := station("Manga", uuid.New(), true)
	missing := uuid.New()
	h := newAlertHarness(active, inactive, away)

	_, err := h.svc.Alert(context.Background(), uuid.New(), models.FireStationAlertRequest{
		DepartmentID: dept, ServicePointIDs: []uuid.UUID{active.ID, inactive.ID, away.ID, missing},
	})
	fields := errFields(t, err)
	if fields[inactive.ID.String()] != "fire station is not active" || fields[away.ID.String()] != "fire station is not in this department" || fields[missing.String()] != "fire station not found" {
		t.Fatal(err)
	}
	if len(h.alerts.rows) != 0 {
		t.Fatalf("stored %d", len(h.alerts.rows))
	}
}

func TestFireStationAlertRequiresASelectionOrActiveStations(t *testing.T) {
	dept := uuid.New()
	h := newAlertHarness()
	_, err := h.svc.Alert(context.Background(), uuid.New(), models.FireStationAlertRequest{DepartmentID: dept})
	if errFields(t, err)["service_point_ids"] != "service_point_ids is required" {
		t.Fatal(err)
	}
	_, err = h.svc.Alert(context.Background(), uuid.New(), models.FireStationAlertRequest{DepartmentID: dept, All: true})
	if errFields(t, err)["department_id"] != "no active fire stations" {
		t.Fatal(err)
	}
	_, err = h.svc.Alert(context.Background(), uuid.Nil, models.FireStationAlertRequest{DepartmentID: dept, All: true})
	if errFields(t, err)["incident_id"] != "incident_id is required" {
		t.Fatal(err)
	}
}

func TestFireStationAlertSelectionBeforeTheIncidentExists(t *testing.T) {
	dept := uuid.New()
	first := station("Gufran", dept, true)
	second := station("Manga", dept, true)
	h := newAlertHarness(first, second)

	got, err := h.svc.AlertSelection(context.Background(), models.FireStationAlertRequest{ServicePointIDs: []uuid.UUID{first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if got.IncidentID != nil || len(got.Alerted) != 1 || got.Alerted[0].Name != "Gufran" {
		t.Fatalf("result %#v", got)
	}
	if _, err := h.svc.AlertSelection(context.Background(), models.FireStationAlertRequest{ServicePointIDs: []uuid.UUID{first.ID}}); err != nil {
		t.Fatal(err)
	}
	if len(h.alerts.rows) != 1 {
		t.Fatalf("stored %d", len(h.alerts.rows))
	}
}

func TestFireStationAlertList(t *testing.T) {
	dept := uuid.New()
	point := station("Gufran", dept, true)
	h := newAlertHarness(point)
	incident := uuid.New()
	if _, err := h.svc.Alert(context.Background(), incident, models.FireStationAlertRequest{DepartmentID: dept, All: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := h.svc.List(context.Background(), incident)
	if err != nil || len(rows) != 1 || rows[0].Name != "Gufran" {
		t.Fatalf("rows %#v err %v", rows, err)
	}
	empty, err := h.svc.List(context.Background(), uuid.New())
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty %#v err %v", empty, err)
	}
}

type alertHarness struct {
	svc    *FireStationAlertService
	alerts *memAlerts
}

func newAlertHarness(points ...models.ServicePoint) *alertHarness {
	stored := map[uuid.UUID]models.ServicePoint{}
	for _, point := range points {
		stored[point.ID] = point
	}
	repo := &memPoints{rows: stored}
	alerts := &memAlerts{points: repo}
	return &alertHarness{svc: NewFireStationAlertService(repo, alerts), alerts: alerts}
}

func station(name string, dept uuid.UUID, active bool) models.ServicePoint {
	return models.ServicePoint{ID: uuid.New(), Name: name, DepartmentID: dept, RadioID: "R1", IsActive: active}
}

type memPoints struct {
	rows map[uuid.UUID]models.ServicePoint
}

func (m *memPoints) Create(context.Context, *models.ServicePoint) error { return nil }
func (m *memPoints) Update(context.Context, *models.ServicePoint) error { return nil }
func (m *memPoints) Delete(context.Context, uuid.UUID) error            { return nil }
func (m *memPoints) AssignResources(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (m *memPoints) Exists(context.Context, uuid.UUID) (bool, error) { return false, nil }
func (m *memPoints) ResourceCount(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *memPoints) Nearest(context.Context, float64, float64, *uuid.UUID, int) ([]models.NearestServicePoint, error) {
	return nil, nil
}
func (m *memPoints) FindByID(_ context.Context, id uuid.UUID) (*models.ServicePoint, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}
func (m *memPoints) List(_ context.Context, filter models.ServicePointListFilter) ([]models.ServicePoint, int64, error) {
	var rows []models.ServicePoint
	for _, row := range m.rows {
		if filter.DepartmentID != nil && row.DepartmentID != *filter.DepartmentID {
			continue
		}
		if filter.Active != nil && row.IsActive != *filter.Active {
			continue
		}
		rows = append(rows, row)
	}
	return rows, int64(len(rows)), nil
}

type memAlerts struct {
	rows   []models.FireStationAlert
	points *memPoints
}

func (m *memAlerts) Save(_ context.Context, rows []models.FireStationAlert) error {
	for _, row := range rows {
		for _, existing := range m.rows {
			if existing.ServicePointID == row.ServicePointID && sameIncident(existing.IncidentID, row.IncidentID) {
				goto next
			}
		}
		if row.ID == uuid.Nil {
			row.ID = uuid.New()
		}
		m.rows = append(m.rows, row)
	next:
	}
	return nil
}

func sameIncident(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (m *memAlerts) ListSelected(_ context.Context, incidentID *uuid.UUID, stationIDs []uuid.UUID) ([]models.FireStationAlert, error) {
	want := map[uuid.UUID]struct{}{}
	for _, id := range stationIDs {
		want[id] = struct{}{}
	}
	var rows []models.FireStationAlert
	for _, row := range m.rows {
		if _, ok := want[row.ServicePointID]; !ok {
			continue
		}
		if incidentID == nil {
			if row.IncidentID != nil {
				continue
			}
		} else if row.IncidentID == nil || *row.IncidentID != *incidentID {
			continue
		}
		if point, ok := m.points.rows[row.ServicePointID]; ok {
			row.Name = point.Name
			row.RadioID = point.RadioID
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (m *memAlerts) ListByIncident(_ context.Context, incidentID uuid.UUID) ([]models.FireStationAlert, error) {
	var rows []models.FireStationAlert
	for _, row := range m.rows {
		if row.IncidentID == nil || *row.IncidentID != incidentID {
			continue
		}
		if point, ok := m.points.rows[row.ServicePointID]; ok {
			row.Name = point.Name
			row.RadioID = point.RadioID
		}
		rows = append(rows, row)
	}
	return rows, nil
}

var _ repository.ServicePointRepository = (*memPoints)(nil)
var _ repository.FireStationAlertRepository = (*memAlerts)(nil)
