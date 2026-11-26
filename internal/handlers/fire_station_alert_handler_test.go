package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/automax/registry/internal/services"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestFireStationAlertRoutes(t *testing.T) {
	dept := uuid.New()
	point := models.ServicePoint{ID: uuid.New(), Name: "Gufran Station", DepartmentID: dept, RadioID: "234", IsActive: true}
	points := &alertPoints{rows: map[uuid.UUID]models.ServicePoint{point.ID: point}}
	svc := services.NewFireStationAlertService(points, &alertStore{})
	handler := NewFireStationAlertHandler(svc)
	app := fiber.New()
	app.Post("/dispatch/incidents/:incident_id/fire-station-alerts", handler.Alert)
	app.Get("/dispatch/incidents/:incident_id/fire-station-alerts", handler.List)

	incident := uuid.New()
	body := `{"department_id":"` + dept.String() + `","all":false,"service_point_ids":["` + point.ID.String() + `"]}`
	post := httptest.NewRequest(http.MethodPost, "/dispatch/incidents/"+incident.String()+"/fire-station-alerts", strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(post)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "Gufran Station") || !strings.Contains(string(raw), `"radio_id":"234"`) {
		t.Fatalf("status %d body %s", resp.StatusCode, raw)
	}

	listed, err := app.Test(httptest.NewRequest(http.MethodGet, "/dispatch/incidents/"+incident.String()+"/fire-station-alerts", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(listed.Body)
	if listed.StatusCode != http.StatusOK || !strings.Contains(string(raw), "Gufran Station") {
		t.Fatalf("list status %d body %s", listed.StatusCode, raw)
	}

	bad, err := app.Test(httptest.NewRequest(http.MethodPost, "/dispatch/incidents/not-a-uuid/fire-station-alerts", strings.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(bad.Body)
	if bad.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "incident id must be a UUID") {
		t.Fatalf("bad status %d body %s", bad.StatusCode, raw)
	}
}

func TestFireStationAlertRouteRejectsAnEmptySelection(t *testing.T) {
	svc := services.NewFireStationAlertService(&alertPoints{}, &alertStore{})
	app := fiber.New()
	app.Post("/dispatch/incidents/:incident_id/fire-station-alerts", NewFireStationAlertHandler(svc).Alert)
	req := httptest.NewRequest(http.MethodPost, "/dispatch/incidents/"+uuid.NewString()+"/fire-station-alerts", strings.NewReader(`{"department_id":"`+uuid.NewString()+`","all":false,"service_point_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Errors map[string][]string `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatal(err)
	}
	got := parsed.Errors["service_point_ids"]
	if resp.StatusCode != http.StatusBadRequest || len(got) != 1 || got[0] != "service_point_ids is required" {
		t.Fatalf("status %d errors %#v", resp.StatusCode, parsed.Errors)
	}
}

type alertPoints struct {
	rows map[uuid.UUID]models.ServicePoint
}

func (m *alertPoints) Create(context.Context, *models.ServicePoint) error { return nil }
func (m *alertPoints) Update(context.Context, *models.ServicePoint) error { return nil }
func (m *alertPoints) Delete(context.Context, uuid.UUID) error            { return nil }
func (m *alertPoints) AssignResources(context.Context, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (m *alertPoints) Exists(context.Context, uuid.UUID) (bool, error) { return false, nil }
func (m *alertPoints) ResourceCount(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *alertPoints) Nearest(context.Context, float64, float64, *uuid.UUID, int) ([]models.NearestServicePoint, error) {
	return nil, nil
}
func (m *alertPoints) FindByID(_ context.Context, id uuid.UUID) (*models.ServicePoint, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}
func (m *alertPoints) List(context.Context, models.ServicePointListFilter) ([]models.ServicePoint, int64, error) {
	return nil, 0, nil
}

type alertStore struct {
	rows []models.FireStationAlert
}

func (m *alertStore) Save(_ context.Context, rows []models.FireStationAlert) error {
	for _, row := range rows {
		m.rows = append(m.rows, row)
	}
	return nil
}
func (m *alertStore) ListSelected(_ context.Context, incidentID *uuid.UUID, stationIDs []uuid.UUID) ([]models.FireStationAlert, error) {
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
		row.Name = "Gufran Station"
		row.RadioID = "234"
		rows = append(rows, row)
	}
	return rows, nil
}

func (m *alertStore) ListByIncident(_ context.Context, incidentID uuid.UUID) ([]models.FireStationAlert, error) {
	var rows []models.FireStationAlert
	for _, row := range m.rows {
		if row.IncidentID != nil && *row.IncidentID == incidentID {
			row.Name = "Gufran Station"
			row.RadioID = "234"
			rows = append(rows, row)
		}
	}
	return rows, nil
}

var _ repository.ServicePointRepository = (*alertPoints)(nil)
var _ repository.FireStationAlertRepository = (*alertStore)(nil)
