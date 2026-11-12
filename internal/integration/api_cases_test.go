package integration

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/automax/registry/internal/config"
	"github.com/automax/registry/internal/database"
	"github.com/automax/registry/internal/handlers"
	"github.com/google/uuid"
)

func TestDuplicateNamesAreRejected(t *testing.T) {
	truncate(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(nil)
	dept := uuid.New().String()
	other := uuid.New().String()

	thing := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	thingID := dataID(t, thing)
	second := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusConflict)
	if second["error"] != "a thing with this name already exists" || dataID(t, thing) == "" {
		t.Fatalf("duplicate thing %#v", second)
	}
	if dataID(t, thing) == dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)) {
		t.Fatal("case-different names must be different records")
	}
	requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Engine 4","type":"vehicle","department_id":"%s"}`, other)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/things/"+thingID, fmt.Sprintf(
		`{"name":"engine 4","type":"vehicle","department_id":"%s"}`, dept)), http.StatusConflict)

	requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(
		`{"name":"fleet","department_id":"%s"}`, dept)), http.StatusCreated)
	groupDup := requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(
		`{"name":"fleet","department_id":"%s"}`, dept)), http.StatusConflict)
	if groupDup["error"] != "a group with this name already exists" {
		t.Fatalf("duplicate group %#v", groupDup)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(
		`{"name":"Fleet","department_id":"%s"}`, dept)), http.StatusCreated)

	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{"name":"ops"}`), http.StatusCreated)
	channelDup := requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{"name":"ops"}`), http.StatusConflict)
	if channelDup["error"] != "a channel with this name already exists" {
		t.Fatalf("duplicate channel %#v", channelDup)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{"name":"OPS"}`), http.StatusCreated)

	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"Fire Truck","department_id":"%s"}`, dept)), http.StatusCreated)
	typeDup := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"  Fire Truck  ","department_id":"%s"}`, dept)), http.StatusConflict)
	if typeDup["error"] != "a resource type with this name already exists" {
		t.Fatalf("duplicate type %#v", typeDup)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"fire truck","department_id":"%s"}`, dept)), http.StatusCreated)

	pointBody := fmt.Sprintf(`{"name":"Station 1","department_id":"%s","radio_id":"R1","location":{"lat":0,"lng":0}}`, dept)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", pointBody), http.StatusCreated)
	pointDup := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{"name":"Station 1","department_id":"%s","radio_id":"R2","location":{"lat":0,"lng":0}}`, dept)), http.StatusConflict)
	if pointDup["error"] != "a service point with this name already exists" {
		t.Fatalf("duplicate station %#v", pointDup)
	}

	shiftBody := fmt.Sprintf(`{"name":"Morning","department_id":"%s","start_time":"09:00","end_time":"12:00","days_of_week":"MON"}`, dept)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", shiftBody), http.StatusCreated)
	shiftDup := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", shiftBody), http.StatusConflict)
	if shiftDup["error"] != "a shift with this name already exists" {
		t.Fatalf("duplicate shift %#v", shiftDup)
	}

	if !strings.Contains(logs.String(), "rejected status=409") {
		t.Fatalf("expected conflict logs\n%s", logs.String())
	}
}

func TestDispatchValidationNotFoundAndMethod(t *testing.T) {
	truncate(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(nil)
	dept := uuid.New().String()
	missing := uuid.New().String()

	empty := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types", ""), http.StatusOK)
	if rows, ok := empty["data"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("empty list %#v", empty["data"])
	}
	for _, path := range []string{
		"/dispatch/resources",
		"/dispatch/service-points",
		"/dispatch/shifts",
		"/dispatch/resource-shift-assignments",
	} {
		body := requireStatus(t, do(t, app, http.MethodGet, path, ""), http.StatusOK)
		rows, ok := body["data"].([]any)
		if !ok || len(rows) != 0 {
			t.Fatalf("empty %s %#v", path, body["data"])
		}
	}

	posts := []struct {
		path    string
		badType string
	}{
		{"/dispatch/resource-types", `{"department_id":1}`},
		{"/dispatch/resources", `{"thing_id":1}`},
		{"/dispatch/service-points", `{"department_id":1}`},
		{"/dispatch/shifts", `{"department_id":1}`},
		{"/dispatch/resource-shift-assignments", `{"resource_id":1}`},
	}
	for _, path := range posts {
		bad := requireStatus(t, do(t, app, http.MethodPost, path.path, `{`), http.StatusBadRequest)
		if bad["error"] != "request body must be JSON" {
			t.Fatalf("%s bad json %#v", path.path, bad)
		}
		blank := requireStatus(t, do(t, app, http.MethodPost, path.path, `{}`), http.StatusBadRequest)
		if blank["success"] != false || errorFields(t, blank) == nil {
			t.Fatalf("%s empty body %#v", path.path, blank)
		}
		typed := requireStatus(t, do(t, app, http.MethodPost, path.path, path.badType), http.StatusBadRequest)
		if typed["error"] != "request body must be JSON" {
			t.Fatalf("%s wrong type %#v", path.path, typed)
		}
	}

	ids := map[string]string{
		"/dispatch/resource-types/not-a-uuid":             "resource type id must be a UUID",
		"/dispatch/resources/not-a-uuid":                  "resource id must be a UUID",
		"/dispatch/service-points/not-a-uuid":             "service point id must be a UUID",
		"/dispatch/shifts/not-a-uuid":                     "shift id must be a UUID",
		"/dispatch/resource-shift-assignments/not-a-uuid": "assignment id must be a UUID",
	}
	for path, want := range ids {
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			body := `{"name":"x"}`
			if method != http.MethodPut {
				body = ""
			}
			got := requireStatus(t, do(t, app, method, path, body), http.StatusBadRequest)
			if got["error"] != want {
				t.Fatalf("%s %s %#v", method, path, got)
			}
		}
	}

	for _, path := range []string{
		"/dispatch/resource-types/" + missing,
		"/dispatch/resources/" + missing,
		"/dispatch/service-points/" + missing,
		"/dispatch/shifts/" + missing,
		"/dispatch/resource-shift-assignments/" + missing,
	} {
		requireStatus(t, do(t, app, http.MethodGet, path, ""), http.StatusNotFound)
		requireStatus(t, do(t, app, http.MethodDelete, path, ""), http.StatusNotFound)
	}

	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types?department_id=nope", ""), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?resource_type_id=nope", ""), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?service_point_id=nope", ""), http.StatusBadRequest)
	badStatus := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?status=Available", ""), http.StatusBadRequest)
	if errorFields(t, badStatus)["status"] == nil {
		t.Fatalf("status case %#v", badStatus)
	}
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?status=available", ""), http.StatusOK)

	method := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types/"+missing, `{}`), http.StatusMethodNotAllowed)
	if !strings.Contains(fmt.Sprint(method["error"]), "POST is not allowed") {
		t.Fatalf("method %#v", method)
	}
	collection := requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-types", ""), http.StatusMethodNotAllowed)
	if !strings.Contains(fmt.Sprint(collection["error"]), "DELETE is not allowed") {
		t.Fatalf("delete collection %#v", collection)
	}

	typeID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"Engine","department_id":"%s"}`, dept)), http.StatusCreated))
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s","status":"Available"}`, uuid.New(), typeID)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"North","department_id":"%s","location":{"lat":0,"lng":181}}`, dept)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"South","department_id":"%s","location":{"lng":0}}`, dept)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(
		`{"name":"Night","department_id":"%s","start_time":"22:00","end_time":"06:00","days_of_week":"mon"}`, dept)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resource-types/"+missing, fmt.Sprintf(
		`{"name":"Missing","department_id":"%s"}`, dept)), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/service-points/"+missing, fmt.Sprintf(
		`{"name":"Missing","department_id":"%s","location":{"lat":1,"lng":2}}`, dept)), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/shifts/"+missing, fmt.Sprintf(
		`{"name":"Missing","department_id":"%s","start_time":"01:00","end_time":"02:00","days_of_week":"MON"}`, dept)), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resources/"+missing, fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s"}`, uuid.New(), typeID)), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resource-shift-assignments/"+missing, fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, uuid.New(), uuid.New())), http.StatusNotFound)

	thingID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"Truck","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated))
	resourceID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s"}`, thingID, typeID)), http.StatusCreated))
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"thing_id":"%s","resource_type_id":"%s"}`, thingID, typeID)), http.StatusConflict)
	user := uuid.New().String()
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","status":"Scheduled","start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, resourceID, user)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","start_at":"2026-10-26T12:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, resourceID, user)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","shift_id":"%s"}`, resourceID, user, missing)), http.StatusBadRequest)
	assignmentID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-shift-assignments", fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, resourceID, user)), http.StatusCreated))
	gotAssignment := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-shift-assignments/"+assignmentID, ""), http.StatusOK)
	if dataObject(t, gotAssignment)["status"] != "scheduled" {
		t.Fatalf("assignment %#v", gotAssignment)
	}
	requireStatus(t, do(t, app, http.MethodPut, "/dispatch/resource-shift-assignments/"+assignmentID, fmt.Sprintf(
		`{"resource_id":"%s","user_id":"%s","status":"completed","start_at":"2026-10-26T10:00:00Z","end_at":"2026-10-26T11:00:00Z"}`, resourceID, user)), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/resource-shift-assignments/"+assignmentID, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-shift-assignments/"+assignmentID, ""), http.StatusNotFound)

	text := logs.String()
	for _, part := range []string{
		"rejected status=400",
		"rejected status=404",
		"rejected status=405",
		"rejected status=409",
		"request body must be JSON",
		"registry: resource created",
		"registry: assignment created",
	} {
		if !strings.Contains(text, part) {
			t.Fatalf("log missing %q\n%s", part, text)
		}
	}
}

func TestDatabaseFailureStaysAServerError(t *testing.T) {
	conn, err := database.Connect(config.DatabaseConfig{
		Host: "127.0.0.1", Port: "5441", User: "registry", Password: "registry", DBName: "registry", SSLMode: "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	database.Close(conn)

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := handlers.NewApp(conn, nil, handlers.HealthTargets{DB: conn}, "*", nil, nil)
	body := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resource-types", ""), http.StatusInternalServerError)
	if body["error"] != "internal server error" || body["success"] != false {
		t.Fatalf("body %#v", body)
	}
	if strings.Contains(fmt.Sprint(body), "registry") && strings.Contains(fmt.Sprint(body["error"]), "password") {
		t.Fatalf("response leaked details %#v", body)
	}
	if !strings.Contains(logs.String(), "registry: GET /dispatch/resource-types") {
		t.Fatalf("server error was not logged\n%s", logs.String())
	}
}
