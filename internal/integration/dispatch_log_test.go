package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestDispatchLog(t *testing.T) {
	truncate(t)
	app := newApp(nil)
	dept := uuid.New().String()
	incident := uuid.New().String()

	typeID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"Engine","department_id":"%s"}`, dept)), http.StatusCreated))
	thingID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Engine 12","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	resourceID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s"}`, thingID, typeID)), http.StatusCreated))
	pointID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"Station 9","department_id":"%s","radio_id":"S9","location":{"lat":12.97,"lng":77.59}}`, dept)), http.StatusCreated))
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/incidents/"+incident+"/fire-station-alerts", fmt.Sprintf(
		`{"department_id":"%s","all":false,"service_point_ids":["%s"]}`, dept, pointID)), http.StatusOK)

	empty := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id="+incident, ""), http.StatusOK)
	if rows, ok := empty["data"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("empty log %#v", empty["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id=nope", ""), http.StatusBadRequest)

	body := fmt.Sprintf(`{"incident_id":"%s","resource_id":"%s","role":"master","status":"offered"}`, incident, resourceID)
	created := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/assignments", body), http.StatusCreated)
	assignmentID := dataID(t, created)

	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/assignments/"+assignmentID, body), http.StatusOK)
	acceptedBy := uuid.New().String()
	accepted := fmt.Sprintf(`{"incident_id":"%s","resource_id":"%s","role":"master","status":"accepted","accepted_by_id":"%s","accepted_by_name":"Amina Noor"}`, incident, resourceID, acceptedBy)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/assignments/"+assignmentID, accepted), http.StatusOK)
	supporting := fmt.Sprintf(`{"incident_id":"%s","resource_id":"%s","role":"supporting","status":"accepted"}`, incident, resourceID)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/assignments/"+assignmentID, supporting), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/assignments/"+assignmentID, ""), http.StatusOK)

	listed := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id="+incident, ""), http.StatusOK)
	rows, _ := listed["data"].([]any)
	if len(rows) != 4 {
		t.Fatalf("log rows = %d %#v", len(rows), listed["data"])
	}
	want := []struct{ action, from, to, role string }{
		{"created", "", "offered", "master"},
		{"status_changed", "offered", "accepted", "master"},
		{"updated", "accepted", "accepted", "supporting"},
		{"deleted", "accepted", "", "supporting"},
	}
	for i, row := range rows {
		item, _ := row.(map[string]any)
		if item["action"] != want[i].action || item["role"] != want[i].role || item["assignment_id"] != assignmentID || item["resource_id"] != resourceID {
			t.Fatalf("row %d %#v", i, item)
		}
		if item["fire_station_id"] != pointID || item["fire_station_name"] != "Station 9" || item["fire_stations"] != nil {
			t.Fatalf("row %d station %#v", i, item)
		}
		if i == 1 {
			if item["accepted_by_id"] != acceptedBy || item["accepted_by_name"] != "Amina Noor" {
				t.Fatalf("row %d acceptor %#v", i, item)
			}
		} else if item["accepted_by_id"] != nil || item["accepted_by_name"] != nil {
			t.Fatalf("row %d should not name an acceptor %#v", i, item)
		}
		if got, _ := item["from_status"].(string); got != want[i].from {
			t.Fatalf("row %d from_status %#v", i, item["from_status"])
		}
		if got, _ := item["to_status"].(string); got != want[i].to {
			t.Fatalf("row %d to_status %#v", i, item["to_status"])
		}
	}

	byAssignment := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?assignment_id="+assignmentID, ""), http.StatusOK)
	if rows, _ := byAssignment["data"].([]any); len(rows) != 4 {
		t.Fatalf("assignment log %#v", byAssignment["data"])
	}
	other := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id="+uuid.New().String(), ""), http.StatusOK)
	if rows, _ := other["data"].([]any); len(rows) != 0 {
		t.Fatalf("other incident %#v", other["data"])
	}
}
