package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"math"

	"github.com/automax/registry/internal/handlers"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/automax/registry/internal/services"
	"github.com/google/uuid"
)

type memStore struct {
	mu         sync.Mutex
	deviceID   string
	deviceName string
	lat        float64
	lng        float64
	fix        time.Time
	saved      bool
}

func (m *memStore) SaveLatest(_ context.Context, deviceID, deviceName string, latitude, longitude float64, fixTime time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saved && !fixTime.After(m.fix) {
		return false, nil
	}
	m.deviceID = deviceID
	m.deviceName = deviceName
	m.lat = latitude
	m.lng = longitude
	m.fix = fixTime
	m.saved = true
	return true, nil
}

func (m *memStore) snapshot() (string, float64, float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deviceID, m.lat, m.lng, m.saved
}

func doToken(t *testing.T, app interface {
	Test(req *http.Request, msTimeout ...int) (*http.Response, error)
}, method, path, body, token string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPositionIngest(t *testing.T) {
	store := &memStore{}
	app := handlers.NewAdapter("test-token", store)
	requireStatus(t, do(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":1,"longitude":2,"fixTime":"2026-10-09T08:30:00Z"}`), http.StatusUnauthorized)
	requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":1,"longitude":2,"fixTime":"2026-10-09T08:30:00Z"}`, "wrong"), http.StatusUnauthorized)

	requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","deviceName":"Unit 1","latitude":12.9716,"longitude":77.5946,"fixTime":"2026-10-09T08:30:00Z"}`, "test-token"), http.StatusNoContent)
	gotDevice, lat, lng, saved := store.snapshot()
	if !saved || gotDevice != "428976543" || store.deviceName != "Unit 1" || lat != 12.9716 || lng != 77.5946 {
		t.Fatalf("stored device=%s name=%s lat=%v lng=%v saved=%v", gotDevice, store.deviceName, lat, lng, saved)
	}

	requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":1,"longitude":2,"fixTime":"2026-10-09T07:00:00Z"}`, "test-token"), http.StatusNoContent)
	_, lat, lng, _ = store.snapshot()
	if lat != 12.9716 || lng != 77.5946 {
		t.Fatalf("stale fix overwrote lat=%v lng=%v", lat, lng)
	}

	requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","valid":false}`, "test-token"), http.StatusNoContent)
	_, lat, lng, _ = store.snapshot()
	if lat != 12.9716 || lng != 77.5946 {
		t.Fatalf("invalid fix overwrote lat=%v lng=%v", lat, lng)
	}

	requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"position":{"deviceId":"428976543","latitude":13.01,"longitude":77.60,"fixTime":"2026-10-09T09:00:00Z","valid":true}}`, "test-token"), http.StatusNoContent)
	_, lat, lng, _ = store.snapshot()
	if lat != 13.01 || lng != 77.60 {
		t.Fatalf("traccar envelope lat=%v lng=%v", lat, lng)
	}

	bad := requireStatus(t, doToken(t, app, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":99,"longitude":77.5946,"fixTime":"2026-10-09T10:00:00Z"}`, "test-token"), http.StatusBadRequest)
	if errorFields(t, bad)["latitude"] == nil {
		t.Fatalf("validation %#v", bad)
	}
}

func TestThingCoordinatesFromLiveFix(t *testing.T) {
	truncate(t)
	api := newApp(nil)
	dept := uuid.New()
	created := requireStatus(t, do(t, api, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"van","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	thing := dataObject(t, created)
	if thing["latitude"] != nil || thing["longitude"] != nil {
		t.Fatalf("new thing coordinates %#v", thing)
	}
	thingID := dataID(t, created)
	requireStatus(t, do(t, api, http.MethodPost, "/registry/things/"+thingID+"/external-refs", `{"external_id":"428976543"}`), http.StatusCreated)

	store := &memStore{}
	writer := services.NewThingCoordinateWriter(
		models.ExternalSystemTraccar,
		repository.NewExternalRefRepository(db),
		repository.NewThingRepository(db),
	)
	adapter := handlers.NewAdapterWithThings("test-token", store, writer)
	requireStatus(t, doToken(t, adapter, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":12.9716,"longitude":77.5946,"fixTime":"2026-10-09T08:30:00Z"}`, "test-token"), http.StatusNoContent)

	got := dataObject(t, requireStatus(t, do(t, api, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusOK))
	near(t, got["latitude"], 12.9716)
	near(t, got["longitude"], 77.5946)

	requireStatus(t, doToken(t, adapter, http.MethodPost, "/ingest/positions", `{"deviceId":"428976543","latitude":1,"longitude":2,"fixTime":"2026-10-09T07:00:00Z"}`, "test-token"), http.StatusNoContent)
	got = dataObject(t, requireStatus(t, do(t, api, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusOK))
	near(t, got["latitude"], 12.9716)
	near(t, got["longitude"], 77.5946)

	requireStatus(t, do(t, api, http.MethodPut, "/registry/things/"+thingID, fmt.Sprintf(
		`{"name":"van-renamed","type":"vehicle","department_id":"%s"}`, dept)), http.StatusOK)
	got = dataObject(t, requireStatus(t, do(t, api, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusOK))
	if got["name"] != "van-renamed" {
		t.Fatalf("rename %#v", got)
	}
	near(t, got["latitude"], 12.9716)
	near(t, got["longitude"], 77.5946)

	requireStatus(t, doToken(t, adapter, http.MethodPost, "/ingest/positions", `{"deviceId":"not-linked","latitude":3,"longitude":4,"fixTime":"2026-10-09T11:00:00Z"}`, "test-token"), http.StatusNoContent)
}

func near(t *testing.T, got any, want float64) {
	t.Helper()
	n, ok := got.(float64)
	if !ok || math.Abs(n-want) > 0.000001 {
		t.Fatalf("got %#v want %v", got, want)
	}
}

func TestExternalRefRejectsDuplicateDevice(t *testing.T) {
	truncate(t)
	app := newApp(nil)
	dept := uuid.New()
	first := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"a","type":"vehicle","is_trackable":true,"department_id":"%s"}`, dept)), http.StatusCreated)
	second := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"b","type":"vehicle","is_trackable":true,"department_id":"%s"}`, dept)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/things/"+dataID(t, first)+"/external-refs", `{"external_id":"42"}`), http.StatusCreated)
	conflict := requireStatus(t, do(t, app, http.MethodPost, "/registry/things/"+dataID(t, second)+"/external-refs", `{"external_id":"42"}`), http.StatusConflict)
	if conflict["error"] != "external id is already linked to another thing" {
		t.Fatalf("conflict %#v", conflict)
	}
}
