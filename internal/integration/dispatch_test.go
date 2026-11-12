package integration

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDispatchRoutes(t *testing.T) {
	truncate(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(nil)
	dept := uuid.New().String()
	otherDept := uuid.New().String()

	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/dispatch-assignments", ""), http.StatusNotFound)
	badID := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources/not-a-uuid", ""), http.StatusBadRequest)
	if badID["error"] != "resource id must be a UUID" {
		t.Fatalf("bad resource id %#v", badID)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", `{`), http.StatusBadRequest)

	missing := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", `{}`), http.StatusBadRequest)
	if errorFields(t, missing)["name"] == nil || errorFields(t, missing)["department_id"] == nil {
		t.Fatalf("type validation %#v", missing)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Fire Truck","department_id":"%s","acceptance_timeout_seconds":0}`, dept)), http.StatusBadRequest)

	typeBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Fire Truck","department_id":"%s"}`, dept)), http.StatusCreated)
	typeID := dataID(t, typeBody)
	typeRow := dataObject(t, typeBody)
	if typeRow["acceptance_timeout_seconds"] != float64(120) || typeRow["name"] != "Fire Truck" {
		t.Fatalf("resource type %#v", typeRow)
	}
	otherType := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Ambulance","department_id":"%s","acceptance_timeout_seconds":90}`, otherDept)), http.StatusCreated)
	otherTypeID := dataID(t, otherType)

	filtered := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types?department_id="+dept, ""), http.StatusOK)
	if rows, _ := filtered["data"].([]any); len(rows) != 1 {
		t.Fatalf("filtered types %#v", filtered["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types?department_id=nope", ""), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resource-types/"+typeID, fmt.Sprintf(`{"name":"Engine","department_id":"%s","acceptance_timeout_seconds":60}`, dept)), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types/"+uuid.New().String(), ""), http.StatusNotFound)

	pointMissing := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Station 1","department_id":"%s"}`, dept)), http.StatusBadRequest)
	if errorFields(t, pointMissing)["location"] == nil {
		t.Fatalf("location %#v", pointMissing)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Station 1","department_id":"%s","location":{"lat":91,"lng":0}}`, dept)), http.StatusBadRequest)
	pointBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Station 1","department_id":"%s","radio_id":"S1","location":{"lat":12.9716,"lng":77.5946}}`, dept)), http.StatusCreated)
	pointID := dataID(t, pointBody)
	loc, _ := dataObject(t, pointBody)["location"].(map[string]any)
	near(t, loc["lat"], 12.9716)
	near(t, loc["lng"], 77.5946)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/service-points", ""), http.StatusOK)

	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Morning","department_id":"%s","start_time":"09:00","end_time":"12:00","days_of_week":"FUNDAY"}`, dept)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Morning","department_id":"%s","start_time":"09:00","end_time":"09:00","days_of_week":"MON"}`, dept)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Week","department_id":"%s","start_time":"09:00","end_time":"17:00","days_of_week":"MON,TUE,WED,THU,FRI,SAT"}`, dept)), http.StatusBadRequest)
	shiftBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Morning","department_id":"%s","start_time":"09:00","end_time":"12:00","days_of_week":"mon,tue"}`, dept)), http.StatusCreated)
	shiftID := dataID(t, shiftBody)
	shiftRow := dataObject(t, shiftBody)
	if shiftRow["days_of_week"] != "MON,TUE" || shiftRow["start_time"] != "09:00:00" || shiftRow["end_time"] != "12:00:00" || shiftRow["is_active"] != true {
		t.Fatalf("shift %#v", shiftRow)
	}
	lateBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Late","department_id":"%s","start_time":"11:00","end_time":"13:00","days_of_week":"MON","is_active":false}`, dept)), http.StatusCreated)
	lateID := dataID(t, lateBody)
	afternoon := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(`{"name":"Afternoon","department_id":"%s","start_time":"13:00","end_time":"17:00","days_of_week":"MON"}`, dept)), http.StatusCreated)
	afternoonID := dataID(t, afternoon)

	thingBody := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"Engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	thingID := dataID(t, thingBody)
	secondThing := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"Engine 5","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))

	unknownThing := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s"}`, uuid.New(), typeID)), http.StatusBadRequest)
	if errorFields(t, unknownThing)["thing_id"] != "thing_id does not match a registry thing" {
		t.Fatalf("unknown thing %#v", unknownThing)
	}
	resourceBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","service_point_id":"%s"}`, thingID, typeID, pointID)), http.StatusCreated)
	resourceID := dataID(t, resourceBody)
	resourceRow := dataObject(t, resourceBody)
	if resourceRow["thing_id"] != thingID || resourceRow["name"] != "Engine 4" || resourceRow["status"] != "available" || resourceRow["service_point_id"] != pointID {
		t.Fatalf("resource %#v", resourceRow)
	}
	assertThingResource(t, thingID, typeID, pointID, "available")
	dup := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s"}`, thingID, typeID)), http.StatusConflict)
	if dup["error"] != "a resource already exists for this thing" {
		t.Fatalf("duplicate %#v", dup)
	}
	secondResource := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","status":"maintenance"}`, secondThing, otherTypeID)), http.StatusCreated))

	updated := requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resources/"+resourceID, fmt.Sprintf(`{"thing_id":"%s","resource_type_id":"%s","service_point_id":"%s","status":"dispatched"}`, thingID, typeID, pointID)), http.StatusOK)
	if dataObject(t, updated)["status"] != "dispatched" {
		t.Fatalf("updated %#v", updated)
	}
	assertThingResource(t, thingID, typeID, pointID, "dispatched")
	listed := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?status=dispatched&resource_type_id="+typeID, ""), http.StatusOK)
	if rows, _ := listed["data"].([]any); len(rows) != 1 {
		t.Fatalf("filtered resources %#v", listed["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?status=nope", ""), http.StatusBadRequest)

	blockedThing := requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID, ""), http.StatusConflict)
	if blockedThing["error"] != "thing is a resource" {
		t.Fatalf("thing delete %#v", blockedThing)
	}

	userA := uuid.New().String()
	userB := uuid.New().String()
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s"}`, resourceID, userA)), http.StatusBadRequest)
	overlap := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","shift_id":"%s"}`, resourceID, userA, shiftID)), http.StatusCreated)
	assignmentID := dataID(t, overlap)
	conflict := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","shift_id":"%s","status":"active"}`, resourceID, userB, lateID)), http.StatusConflict)
	if conflict["error"] != "resource already has an operator during this time" {
		t.Fatalf("overlap %#v", conflict)
	}
	other := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","shift_id":"%s"}`, secondResource, userB, shiftID)), http.StatusCreated)
	otherAssignment := dataID(t, other)
	afternoonAssignment := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","shift_id":"%s"}`, resourceID, userB, afternoonID)), http.StatusCreated))
	adhocConflict := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, resourceID, userB)), http.StatusConflict)
	if adhocConflict["success"] != false {
		t.Fatalf("adhoc %#v", adhocConflict)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","start_at":"2026-10-26T18:00:00Z","end_at":"2026-10-26T19:00:00Z"}`, resourceID, userB)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(`{"resource_id":"%s","user_id":"%s","shift_id":"%s","status":"completed"}`, resourceID, userB, lateID)), http.StatusCreated)

	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resources/"+resourceID, ""), http.StatusConflict)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/shifts/"+shiftID, ""), http.StatusConflict)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/service-points/"+pointID, ""), http.StatusConflict)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-types/"+typeID, ""), http.StatusConflict)

	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-shift-assignments/"+assignmentID, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-shift-assignments/"+afternoonAssignment, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-shift-assignments", ""), http.StatusOK)

	if err := db.Exec(`INSERT INTO dispatch_assignment (incident_id, resource_id) VALUES (?, ?)`, uuid.New(), resourceID).Error; err != nil {
		t.Fatal(err)
	}
	stillBlocked := requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resources/"+resourceID, ""), http.StatusConflict)
	if !strings.Contains(fmt.Sprint(stillBlocked["error"]), "dispatch assignments") {
		t.Fatalf("dispatch block %#v", stillBlocked)
	}
	if err := db.Exec(`DELETE FROM dispatch_assignment WHERE resource_id = ?`, resourceID).Error; err != nil {
		t.Fatal(err)
	}
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-shift-assignments/"+otherAssignment, ""), http.StatusOK)
	// The completed and evening assignments still reference the first resource.
	var leftover []string
	if err := db.Raw(`SELECT id FROM resource_shift_assignment WHERE resource_id = ?`, resourceID).Scan(&leftover).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range leftover {
		requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-shift-assignments/"+id, ""), http.StatusOK)
	}
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resources/"+resourceID, ""), http.StatusOK)
	assertThingResource(t, thingID, "", "", "")

	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resources/"+secondResource, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID, ""), http.StatusOK)
	if !strings.Contains(logs.String(), "registry: resource created") || !strings.Contains(logs.String(), "reason=operator overlap") {
		t.Fatalf("logs missing resource events:\n%s", logs.String())
	}
}

func assertThingResource(t *testing.T, thingID, typeID, pointID, status string) {
	t.Helper()
	var row struct {
		TypeID  *string
		PointID *string
		Status  *string
	}
	err := db.Raw(`SELECT resource_type_id::text AS type_id, service_point_id::text AS point_id, resource_status AS status FROM things WHERE id = ?`, thingID).Scan(&row).Error
	if err != nil {
		t.Fatal(err)
	}
	gotType, gotPoint, gotStatus := "", "", ""
	if row.TypeID != nil {
		gotType = *row.TypeID
	}
	if row.PointID != nil {
		gotPoint = *row.PointID
	}
	if row.Status != nil {
		gotStatus = *row.Status
	}
	if gotType != typeID || gotPoint != pointID || gotStatus != status {
		t.Fatalf("thing resource type=%q point=%q status=%q", gotType, gotPoint, gotStatus)
	}
}
