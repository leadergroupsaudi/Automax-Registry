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
		`{"thing_id":"%s","resource_type_id":"%s","name":"Engine 12","unique_id":"UNIT-12","plate_number":"ENG-1012","chassis_number":"CHS1012"}`, thingID, typeID)), http.StatusCreated))
	pointID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"Station 9","department_id":"%s","radio_id":"S9","location":{"lat":12.97,"lng":77.59}}`, dept)), http.StatusCreated))
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/incidents/"+incident+"/fire-station-alerts", fmt.Sprintf(
		`{"department_id":"%s","all":false,"service_point_ids":["%s"]}`, dept, pointID)), http.StatusOK)

	assignedStation := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id="+incident, ""), http.StatusOK)
	assignedRows, _ := assignedStation["data"].([]any)
	if len(assignedRows) != 1 {
		t.Fatalf("incident station log %#v", assignedStation["data"])
	}
	assignedItem, _ := assignedRows[0].(map[string]any)
	if assignedItem["action"] != "incident_assigned" || assignedItem["fire_station_id"] != pointID || assignedItem["fire_station_name"] != "Station 9" || assignedItem["resource_id"] != nil || assignedItem["assignment_id"] != nil {
		t.Fatalf("incident station row %#v", assignedItem)
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
	if len(rows) != 5 {
		t.Fatalf("log rows = %d %#v", len(rows), listed["data"])
	}
	rows = rows[1:]
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

func TestDispatchLogStationDrop(t *testing.T) {
	truncate(t)
	app := newApp(nil)
	dept := uuid.New().String()
	typeID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"Engine","department_id":"%s"}`, dept)), http.StatusCreated))
	thingID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	resourceID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s","name":"Engine 4","unique_id":"UNIT-4","plate_number":"ENG-1004","chassis_number":"CHS1004"}`, thingID, typeID)), http.StatusCreated))
	stationA := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"Station A","department_id":"%s","radio_id":"A1","location":{"lat":12.97,"lng":77.59}}`, dept)), http.StatusCreated))
	stationB := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"Station B","department_id":"%s","radio_id":"B1","location":{"lat":12.98,"lng":77.60}}`, dept)), http.StatusCreated))

	none := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?resource_id="+resourceID, ""), http.StatusOK)
	if rows, _ := none["data"].([]any); len(rows) != 0 {
		t.Fatalf("log before drop %#v", none["data"])
	}

	putStation := func(id, name, resources string) {
		t.Helper()
		requireStatus(t, do(t, app, http.MethodPut, "/dispatch/service-points/"+id, fmt.Sprintf(
			`{"name":"%s","department_id":"%s","location":{"lat":12.97,"lng":77.59},"resource_ids":[%s]}`, name, dept, resources)), http.StatusOK)
	}
	putStation(stationA, "Station A", `"`+resourceID+`"`)
	dropped := logRows(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?resource_id="+resourceID, ""), http.StatusOK))
	if len(dropped) != 1 || dropped[0]["action"] != "station_assigned" || dropped[0]["fire_station_id"] != stationA || dropped[0]["fire_station_name"] != "Station A" || dropped[0]["assignment_id"] != nil {
		t.Fatalf("drop %#v", dropped)
	}

	putStation(stationB, "Station B", `"`+resourceID+`"`)
	moved := logRows(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?resource_id="+resourceID, ""), http.StatusOK))
	if !hasLog(moved, "station_unassigned", stationA) || !hasLog(moved, "station_assigned", stationB) {
		t.Fatalf("move %#v", moved)
	}

	putStation(stationB, "Station B", "")
	removed := logRows(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?fire_station_id="+stationB, ""), http.StatusOK))
	if !hasLog(removed, "station_assigned", stationB) || !hasLog(removed, "station_unassigned", stationB) {
		t.Fatalf("remove %#v", removed)
	}

	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resources/"+resourceID, fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s","name":"Engine 4","unique_id":"UNIT-4","plate_number":"ENG-1004","chassis_number":"CHS1004","service_point_id":"%s"}`, thingID, typeID, stationA)), http.StatusOK)
	again := logRows(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?resource_id="+resourceID, ""), http.StatusOK))
	if countLog(again, "station_assigned", stationA) != 2 {
		t.Fatalf("unit update %#v", again)
	}

	incident := uuid.New().String()
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/incidents/"+incident+"/fire-station-alerts", fmt.Sprintf(
		`{"department_id":"%s","all":false,"service_point_ids":["%s"]}`, dept, stationA)), http.StatusOK)
	incidentRows := logRows(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/logs?incident_id="+incident, ""), http.StatusOK))
	if len(incidentRows) != 1 || incidentRows[0]["action"] != "incident_assigned" || incidentRows[0]["fire_station_id"] != stationA || incidentRows[0]["fire_station_name"] != "Station A" {
		t.Fatalf("incident assign %#v", incidentRows)
	}
}

func logRows(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, _ := body["data"].([]any)
	rows := make([]map[string]any, 0, len(raw))
	for _, row := range raw {
		item, _ := row.(map[string]any)
		rows = append(rows, item)
	}
	return rows
}

func hasLog(rows []map[string]any, action, stationID string) bool {
	return countLog(rows, action, stationID) > 0
}

func countLog(rows []map[string]any, action, stationID string) int {
	n := 0
	for _, row := range rows {
		if row["action"] == action && row["fire_station_id"] == stationID {
			n++
		}
	}
	return n
}
