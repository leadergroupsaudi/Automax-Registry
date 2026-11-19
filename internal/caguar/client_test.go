package caguar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterDeviceAndDriver(t *testing.T) {
	var deviceBody map[string]any
	var driverBody map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		var dest *map[string]any
		switch r.URL.Path {
		case "/api/devices":
			dest = &deviceBody
		case "/api/drivers":
			dest = &driverBody
		default:
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL, "secret-token")
	err := client.RegisterDevice(context.Background(), Device{
		Name:          "Test Device Go",
		UniqueID:      "TESTGO123",
		Status:        "online",
		Category:      "default",
		TrackerType:   "Vehicle",
		PlateNumber:   "ABC-1234",
		ChassisNumber: "CHS-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if deviceBody["name"] != "Test Device Go" || deviceBody["uniqueId"] != "TESTGO123" || deviceBody["trackerType"] != "Vehicle" || deviceBody["plateNumber"] != "ABC-1234" || deviceBody["product"] != ProductAutomax {
		t.Fatalf("device %#v", deviceBody)
	}
	if _, ok := deviceBody["groupIds"]; ok {
		t.Fatalf("groupIds %#v", deviceBody["groupIds"])
	}
	if auth != "Bearer secret-token" {
		t.Fatalf("auth %q", auth)
	}

	if err := client.RegisterDriver(context.Background(), Driver{Name: "Test Driver Go", UniqueID: "DRV001"}); err != nil {
		t.Fatal(err)
	}
	if driverBody["name"] != "Test Driver Go" || driverBody["uniqueId"] != "DRV001" {
		t.Fatalf("driver %#v", driverBody)
	}
}

func TestLoginIssuesBearerToken(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session":
			if r.FormValue("email") != "admin" || r.FormValue("password") != "secret" {
				t.Errorf("session form email=%q", r.FormValue("email"))
			}
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "abc", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case "/api/session/token":
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil || cookie.Value != "abc" {
				t.Errorf("session cookie %v", err)
			}
			w.Write([]byte("issued-token"))
		case "/api/devices":
			auth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	err := NewAccount(srv.URL, "", "admin", "secret").RegisterDevice(context.Background(), Device{Name: "A", UniqueID: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer issued-token" {
		t.Fatalf("auth %q", auth)
	}
}

func TestLoginReadsTokenFromJSON(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"token":"session-token"}`))
		case "/api/session/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"status_code":200,"message":"Token generated successfully.","data":"issued-token"}`))
		case "/api/devices":
			auth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	err := NewAccount(srv.URL, "", "admin", "secret").RegisterDevice(context.Background(), Device{Name: "A", UniqueID: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer issued-token" {
		t.Fatalf("auth %q", auth)
	}
}

func TestUpdateDeviceKeepsPortalGroups(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			if r.URL.Query().Get("uniqueId") != "OLD-1" {
				t.Errorf("uniqueId %q", r.URL.Query().Get("uniqueId"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success":true,"data":[{"id":44,"name":"Old","uniqueId":"OLD-1","status":"online","groupIds":[7],"zoneIds":[3],"calendarId":2,"attributes":{"keep":true}}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/devices/44":
			method = r.Method
			path = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	err := New(srv.URL, "secret-token").UpdateDevice(context.Background(), "OLD-1", Device{
		Name: "Engine 12", UniqueID: "NEW-1", Category: "truck", TrackerType: "gps",
		PlateNumber: "ABC-1234", ChassisNumber: "CH-100", Phone: "+966500000000", Disabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/api/devices/44" {
		t.Fatalf("method %s path %s", method, path)
	}
	if body["uniqueId"] != "NEW-1" || body["status"] != "online" || body["disabled"] != true {
		t.Fatalf("body %#v", body)
	}
	if _, sent := body["product"]; sent {
		t.Fatalf("update must not send product, body %#v", body)
	}
	groups, ok := body["groupIds"].([]any)
	if !ok || len(groups) != 1 || groups[0] != float64(7) {
		t.Fatalf("groups %#v", body["groupIds"])
	}
}

func TestUpdateDeviceFallsBackWhenLookupIsRestricted(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("uniqueId") != "":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"This device is registered as an AutoMax device and cannot be accessed through this system.","status":"RESTRICTED_PRODUCT"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			w.Write([]byte(`[{"id":44,"name":"Old","uniqueId":"UNIT1322","groupIds":[7],"zoneIds":[],"attributes":{}}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/devices/44":
			method = r.Method
			path = r.URL.Path
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	err := New(srv.URL, "secret-token").UpdateDevice(context.Background(), "UNIT1322", Device{
		Name: "Engine 1355", UniqueID: "UNIT1322", Category: "car", TrackerType: "Vehicle",
		PlateNumber: "ENG-1322", ChassisNumber: "CHS1322", Product: ProductAutomax,
	})
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/api/devices/44" || body["name"] != "Engine 1355" {
		t.Fatalf("method %s path %s body %#v", method, path, body)
	}
	if _, sent := body["product"]; sent {
		t.Fatalf("product %#v", body["product"])
	}
}

func TestDeleteDevice(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":44,"name":"Engine","uniqueId":"UNIT1"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/devices/44":
			method = r.Method
			path = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	if err := New(srv.URL, "secret-token").DeleteDevice(context.Background(), "UNIT1"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/api/devices/44" {
		t.Fatalf("method %s path %s", method, path)
	}
}

func TestRegisterDeviceRejectsPortalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "duplicate", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	err := New(srv.URL, "").RegisterDevice(context.Background(), Device{Name: "A", UniqueID: "B"})
	remote, ok := err.(*RemoteError)
	if !ok || remote.Status != http.StatusBadRequest {
		t.Fatalf("err %#v", err)
	}
}
