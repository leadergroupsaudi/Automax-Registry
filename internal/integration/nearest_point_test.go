package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestNearestServicePointsOrderAndTypeFilter(t *testing.T) {
	truncate(t)
	app := newApp(nil)
	dept := uuid.New().String()

	fireID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Fire Truck","department_id":"%s"}`, dept)), http.StatusCreated))
	medicID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Ambulance","department_id":"%s"}`, dept)), http.StatusCreated))
	nearID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Near Station","department_id":"%s","radio_id":"NEAR","location":{"lat":12.9720,"lng":77.5946}}`, dept)), http.StatusCreated))
	farID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Far Station","department_id":"%s","radio_id":"FAR","location":{"lat":13.0500,"lng":77.5946}}`, dept)), http.StatusCreated))

	nearThing := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"Engine Near","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	farThing := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"Medic Far","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	idleThing := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"Engine Idle","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","service_point_id":"%s","status":"available"}`, nearThing, fireID, nearID)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","service_point_id":"%s","status":"available"}`, farThing, medicID, farID)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","service_point_id":"%s","status":"maintenance"}`, idleThing, fireID, farID)), http.StatusCreated)

	body := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/service-points/nearest?lat=12.9716&lng=77.5946&limit=5", ""), http.StatusOK)
	rows, _ := body["data"].([]any)
	if len(rows) != 2 {
		t.Fatalf("nearest %#v", body["data"])
	}
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if first["id"] != nearID || second["id"] != farID {
		t.Fatalf("order %#v", body["data"])
	}
	if first["distance_meters"].(float64) >= second["distance_meters"].(float64) {
		t.Fatalf("distances %#v", body["data"])
	}

	filtered := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/service-points/nearest?lat=12.9716&lng=77.5946&limit=5&resourceTypeId="+fireID, ""), http.StatusOK)
	only, _ := filtered["data"].([]any)
	if len(only) != 1 || only[0].(map[string]any)["id"] != nearID {
		t.Fatalf("fire filter %#v", filtered["data"])
	}

	bad := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/service-points/nearest?lat=91&lng=0", ""), http.StatusBadRequest)
	if errorFields(t, bad)["lat"] == nil {
		t.Fatalf("validation %#v", bad)
	}
}
