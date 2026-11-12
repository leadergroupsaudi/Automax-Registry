package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

func TestDispatchMasterFields(t *testing.T) {
	truncate(t)
	pub := &recordingPublisher{}
	app := newApp(pub)
	dept := uuid.New().String()

	typeBody := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{
		"name":"Truck","code":"TRK","alt_name":"شاحنة","department_id":"%s",
		"min_speed":0,"max_speed":120,"is_active":true,
		"colors":{"available":"#22A45D","busy":"red"}
	}`, dept)), http.StatusBadRequest)
	if errorFields(t, typeBody)["colors.busy"] == nil {
		t.Fatalf("color %#v", typeBody)
	}

	created := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{
		"name":"Truck","code":"TRK","alt_name":"شاحنة","department_id":"%s",
		"min_speed":0,"max_speed":120,"active_till_date":"2026-12-31",
		"colors":{"available":"#112233"}
	}`, dept)), http.StatusCreated)
	typeRow := dataObject(t, created)
	typeID := dataID(t, created)
	if typeRow["code"] != "TRK" || typeRow["alt_name"] != "شاحنة" || typeRow["is_active"] != true || typeRow["active_till_date"] != "2026-12-31" {
		t.Fatalf("type %#v", typeRow)
	}
	colors, _ := typeRow["colors"].(map[string]any)
	if colors["available"] != "#112233" || colors["busy"] != "#E23B3B" {
		t.Fatalf("colors %#v", colors)
	}
	dupCode := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(`{"name":"Other","code":"TRK","department_id":"%s"}`, dept)), http.StatusConflict)
	if dupCode["error"] != "a resource type with this code already exists" {
		t.Fatalf("dup code %#v", dupCode)
	}

	station := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{
		"name":"Manga","alt_name":"مانجا","department_id":"%s","radio_id":"FS-1",
		"location_name":"Manga","location":{"lat":29.106,"lng":48.120},"is_active":true
	}`, dept)), http.StatusCreated)
	pointID := dataID(t, station)
	pointRow := dataObject(t, station)
	if pointRow["radio_id"] != "FS-1" || pointRow["alt_name"] != "مانجا" {
		t.Fatalf("station %#v", pointRow)
	}

	unit := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{
		"name":"Engine 9","resource_type_id":"%s","service_point_id":"%s",
		"plate_number":"12233","alternate_name":"محرك","radio_id":"234",
		"classification":["إنقاذ","إنقاذ"],"call_code":"C1","description":"wd"
	}`, typeID, pointID)), http.StatusCreated)
	unitRow := dataObject(t, unit)
	if unitRow["name"] != "Engine 9" || unitRow["plate_number"] != "12233" || unitRow["is_active"] != true {
		t.Fatalf("unit %#v", unitRow)
	}
	classes, _ := unitRow["classification"].([]any)
	if len(classes) != 1 || classes[0] != "إنقاذ" {
		t.Fatalf("classes %#v", unitRow["classification"])
	}

	assigned := requireStatus(t, do(t, app, http.MethodPut, "/dispatch/service-points/"+pointID, fmt.Sprintf(`{
		"name":"Manga","department_id":"%s","location":{"lat":29.106,"lng":48.120},
		"resource_ids":["%s"]
	}`, dept, dataID(t, unit))), http.StatusOK)
	ids, _ := dataObject(t, assigned)["resource_ids"].([]any)
	if len(ids) != 1 || ids[0] != dataID(t, unit) {
		t.Fatalf("assigned %#v", assigned)
	}

	paged := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types?page=1&limit=10&search=tru&department_id="+dept, ""), http.StatusOK)
	if paged["total_items"] != float64(1) || paged["page"] != float64(1) {
		t.Fatalf("page %#v", paged)
	}

	var subjects []string
	for _, msg := range pub.msgs {
		subjects = append(subjects, msg.subject)
	}
	want := map[string]bool{
		models.SubjectThingCreated:        false,
		models.SubjectResourceTypeCreated: false,
		models.SubjectResourceCreated:     false,
		models.SubjectServicePointCreated: false,
		models.SubjectServicePointUpdated: false,
	}
	for _, subject := range subjects {
		if _, ok := want[subject]; ok {
			want[subject] = true
		}
	}
	for subject, seen := range want {
		if !seen {
			t.Fatalf("missing %s in %v", subject, subjects)
		}
	}
}
