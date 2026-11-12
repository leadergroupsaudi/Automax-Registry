package integration

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestDispatchScreenFields(t *testing.T) {
	truncate(t)
	app := newApp(nil)
	dept := uuid.New().String()
	driver := uuid.New().String()
	partner := uuid.New().String()
	location := uuid.New().String()
	classID := uuid.New().String()

	typeID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resource-types", fmt.Sprintf(
		`{"name":"Engine","department_id":"%s"}`, dept)), http.StatusCreated))
	shiftID := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/shifts", fmt.Sprintf(
		`{"name":"Day","department_id":"%s","start_time":"08:00","end_time":"16:00","days_of_week":"MON"}`, dept)), http.StatusCreated))

	requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(
		`{"name":"No Radio","department_id":"%s","location":{"lat":1,"lng":1}}`, dept)), http.StatusBadRequest)

	station := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{
		"name":"Manga","department_id":"%s","radio_id":"FS-1","location_id":"%s",
		"active_till_date":"2026-12-31","location":{"lat":29.106,"lng":48.12}
	}`, dept, location)), http.StatusCreated)
	point := dataObject(t, station)
	if point["location_id"] != location || point["active_till_date"] != "2026-12-31" || point["radio_id"] != "FS-1" {
		t.Fatalf("station fields %#v", point)
	}
	pointID := dataID(t, station)

	dupRadio := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{
		"name":"Other","department_id":"%s","radio_id":"FS-1","location":{"lat":1,"lng":1}
	}`, dept)), http.StatusConflict)
	if dupRadio["error"] != "a service point with this radio id already exists in the department" {
		t.Fatalf("dup radio %#v", dupRadio)
	}

	unit := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(`{
		"name":"Engine 9","resource_type_id":"%s","service_point_id":"%s",
		"unit_code":"U-9","location_id":"%s","classification_ids":["%s"],
		"gprs":"g1","sms":"s1","sms_vip":"v1","sim":"sim1",
		"driver_user_id":"%s","shift_id":"%s","partner_user_id":"%s",
		"active_till_date":"2026-12-31"
	}`, typeID, pointID, location, classID, driver, shiftID, partner)), http.StatusCreated)
	row := dataObject(t, unit)
	if row["unit_code"] != "U-9" || row["gprs"] != "g1" || row["sms_vip"] != "v1" || row["driver_user_id"] != driver || row["location_id"] != location {
		t.Fatalf("unit fields %#v", row)
	}
	if row["active_till_date"] != "2026-12-31" || row["shift_id"] != shiftID || row["partner_user_id"] != partner {
		t.Fatalf("crew or date %#v", row)
	}
	unitID := dataID(t, unit)

	listed := requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources?driver_user_id="+driver+"&location_id="+location+"&classification_id="+classID+"&active_till_date=2026-12-31", ""), http.StatusOK)
	rows, _ := listed["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("filtered list %#v", listed["data"])
	}

	other := dataID(t, requireStatus(t, do(t, app, http.MethodPost, "/dispatch/resources", fmt.Sprintf(
		`{"name":"Engine 10","resource_type_id":"%s"}`, typeID)), http.StatusCreated))
	moved := requireStatus(t, do(t, app, http.MethodPost, "/dispatch/service-points", fmt.Sprintf(`{
		"name":"South","department_id":"%s","radio_id":"FS-2","location":{"lat":29.2,"lng":48.2},
		"resource_ids":["%s"]
	}`, dept, unitID)), http.StatusCreated)
	ids, _ := dataObject(t, moved)["resource_ids"].([]any)
	if len(ids) != 1 || ids[0] != unitID {
		t.Fatalf("created assignment %#v", moved)
	}
	home := dataObject(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources/"+unitID, ""), http.StatusOK))
	if home["service_point_id"] != dataID(t, moved) {
		t.Fatalf("reassigned %#v", home["service_point_id"])
	}

	blocked := requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/service-points/"+dataID(t, moved), ""), http.StatusConflict)
	if blocked["error"] != "service point is the home station of one or more resources" {
		t.Fatalf("blocked %#v", blocked)
	}
	requireStatus(t, do(t, app, http.MethodDelete, "/dispatch/service-points/"+dataID(t, moved)+"?force=true", ""), http.StatusOK)
	cleared := dataObject(t, requireStatus(t, do(t, app, http.MethodGet, "/dispatch/resources/"+unitID, ""), http.StatusOK))
	if cleared["service_point_id"] != nil {
		t.Fatalf("still assigned %#v", cleared["service_point_id"])
	}
	_ = other

	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "truck.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/dispatch/icons", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	stored := requireStatus(t, resp, http.StatusCreated)
	icon := dataObject(t, stored)
	path, _ := icon["path"].(string)
	getReq := httptest.NewRequest(http.MethodGet, path, nil)
	got, err := app.Test(getReq, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	raw, _ := io.ReadAll(got.Body)
	if got.StatusCode != http.StatusOK || got.Header.Get("Content-Type") != "image/png" || !bytes.Equal(raw, png) {
		t.Fatalf("icon get %d %s %d bytes", got.StatusCode, got.Header.Get("Content-Type"), len(raw))
	}
}
