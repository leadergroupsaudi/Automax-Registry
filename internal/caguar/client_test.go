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
	if deviceBody["name"] != "Test Device Go" || deviceBody["uniqueId"] != "TESTGO123" || deviceBody["trackerType"] != "Vehicle" || deviceBody["plateNumber"] != "ABC-1234" {
		t.Fatalf("device %#v", deviceBody)
	}
	if _, ok := deviceBody["groupIds"].([]any); !ok {
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
