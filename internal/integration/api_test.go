package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/automax/registry/internal/handlers"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type published struct {
	subject string
	data    []byte
}

type recordingPublisher struct {
	mu   sync.Mutex
	msgs []published
}

func (p *recordingPublisher) Publish(subject string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, published{subject: subject, data: append([]byte(nil), data...)})
	return nil
}

func (p *recordingPublisher) subjects() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.msgs))
	for i, msg := range p.msgs {
		out[i] = msg.subject
	}
	return out
}

type failPublisher struct{}

func (failPublisher) Publish(string, []byte) error { return errors.New("nats down") }

func newApp(pub services.Publisher) *fiber.App {
	if pub == nil {
		pub = &recordingPublisher{}
	}
	return handlers.NewApp(db, pub, handlers.HealthTargets{DB: db}, "*")
}

func do(t *testing.T, app *fiber.App, method, path, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func requireStatus(t *testing.T, resp *http.Response, code int) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("status %d body %s: %v", resp.StatusCode, raw, err)
		}
	}
	if resp.StatusCode != code {
		t.Fatalf("status %d, want %d, body %s", resp.StatusCode, code, raw)
	}
	return body
}

func dataObject(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", body["data"])
	}
	return data
}

func dataID(t *testing.T, body map[string]any) string {
	t.Helper()
	id, _ := dataObject(t, body)["id"].(string)
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("id %q: %v", id, err)
	}
	return id
}

func errorFields(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	fields, ok := body["errors"].(map[string]any)
	if !ok {
		t.Fatalf("errors = %#v", body)
	}
	return fields
}

func TestHealth(t *testing.T) {
	body := requireStatus(t, do(t, newApp(nil), http.MethodGet, "/health", ""), http.StatusOK)
	if body["success"] != true {
		t.Fatalf("body %#v", body)
	}
	data := dataObject(t, body)
	if data["db"] != "up" || data["redis"] != "down" || data["nats"] != "down" {
		t.Fatalf("health %#v", data)
	}
}

func TestThingRoutes(t *testing.T) {
	truncate(t)
	pub := &recordingPublisher{}
	app := newApp(pub)
	dept := uuid.New().String()

	missing := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", `{}`), http.StatusBadRequest)
	if missing["success"] != false || errorFields(t, missing)["name"] == nil || errorFields(t, missing)["department_id"] == nil {
		t.Fatalf("validation body %#v", missing)
	}
	requireStatus(t, do(t, app, http.MethodPost, "/registry/things", `{`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"%s","type":"sensor","department_id":"%s"}`, strings.Repeat("n", 256), dept)), http.StatusBadRequest)

	groupBody := requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(`{"name":"fleet","department_id":"%s"}`, dept)), http.StatusCreated)
	groupID := dataID(t, groupBody)

	requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(`{"name":"van","type":"vehicle","department_id":"%s","group_id":"%s"}`, dept, uuid.New())), http.StatusBadRequest)

	created := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"van","type":"vehicle","is_trackable":true,"department_id":"%s","group_id":"%s","metadata":{"plate":"A1"}}`, dept, groupID)), http.StatusCreated)
	if created["success"] != true || created["message"] != "thing created" {
		t.Fatalf("create body %#v", created)
	}
	thing := dataObject(t, created)
	thingID := dataID(t, created)
	for _, key := range []string{"id", "name", "type", "is_trackable", "department_id", "group_id", "metadata", "created_at", "updated_at"} {
		if _, ok := thing[key]; !ok {
			t.Fatalf("missing %s in %#v", key, thing)
		}
	}
	if thing["name"] != "van" || thing["type"] != "vehicle" || thing["is_trackable"] != true || thing["department_id"] != dept || thing["group_id"] != groupID {
		t.Fatalf("thing %#v", thing)
	}
	meta, ok := thing["metadata"].(map[string]any)
	if !ok || meta["plate"] != "A1" {
		t.Fatalf("metadata %#v", thing["metadata"])
	}
	if subjects := pub.subjects(); len(subjects) != 1 || subjects[0] != models.SubjectThingCreated {
		t.Fatalf("subjects %#v", subjects)
	}
	assertEvent(t, pub.msgs[0], models.SubjectThingCreated, thingID, true)

	other := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"probe","type":"sensor","department_id":"%s"}`, uuid.New())), http.StatusCreated)
	otherID := dataID(t, other)

	listed := requireStatus(t, do(t, app, http.MethodGet, "/registry/things?type=vehicle&department_id="+dept+"&group_id="+groupID, ""), http.StatusOK)
	rows, ok := listed["data"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("filtered list %#v", listed["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/registry/things?department_id=not-a-uuid", ""), http.StatusBadRequest)

	got := requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusOK)
	if dataID(t, got) != thingID || got["message"] != "thing retrieved" {
		t.Fatalf("get %#v", got)
	}
	requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+uuid.New().String(), ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/things/not-a-uuid", ""), http.StatusBadRequest)

	requireStatus(t, do(t, app, http.MethodPut, "/registry/things/"+thingID, `{"name":""}`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/things/"+uuid.New().String(), fmt.Sprintf(
		`{"name":"van","type":"vehicle","department_id":"%s"}`, dept)), http.StatusNotFound)

	updated := requireStatus(t, do(t, app, http.MethodPut, "/registry/things/"+thingID, fmt.Sprintf(
		`{"name":"van 2","type":"vehicle","is_trackable":false,"department_id":"%s","metadata":{}}`, dept)), http.StatusOK)
	updatedThing := dataObject(t, updated)
	if updatedThing["name"] != "van 2" || updatedThing["is_trackable"] != false || updatedThing["group_id"] != nil {
		t.Fatalf("updated %#v", updatedThing)
	}
	assertEvent(t, pub.msgs[len(pub.msgs)-1], models.SubjectThingUpdated, thingID, false)

	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+uuid.New().String(), ""), http.StatusNotFound)
	deleted := requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID, ""), http.StatusOK)
	if deleted["message"] != "thing deleted" {
		t.Fatalf("delete %#v", deleted)
	}
	if _, ok := deleted["data"]; ok {
		t.Fatalf("delete should omit data: %#v", deleted)
	}
	assertEvent(t, pub.msgs[len(pub.msgs)-1], models.SubjectThingDeleted, thingID, false)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusNotFound)

	before := len(pub.msgs)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+otherID, ""), http.StatusOK)
	if len(pub.msgs) != before+1 {
		t.Fatalf("expected one more event, got %#v", pub.subjects())
	}
}

func TestThingCreateSucceedsWhenPublishFails(t *testing.T) {
	truncate(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(failPublisher{})
	body := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"radio","type":"radio","department_id":"%s"}`, uuid.New())), http.StatusCreated)
	if body["success"] != true {
		t.Fatalf("body %#v", body)
	}
	if !strings.Contains(logs.String(), "failed to publish") {
		t.Fatalf("expected publish failure in logs, got %s", logs.String())
	}
}

func TestChannelAndACLRoutes(t *testing.T) {
	truncate(t)
	pub := &recordingPublisher{}
	app := newApp(pub)
	dept := uuid.New().String()

	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{}`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{`), http.StatusBadRequest)

	created := requireStatus(t, do(t, app, http.MethodPost, "/registry/channels", `{"name":"ops"}`), http.StatusCreated)
	channel := dataObject(t, created)
	channelID := dataID(t, created)
	for _, key := range []string{"id", "name", "created_at", "updated_at"} {
		if _, ok := channel[key]; !ok {
			t.Fatalf("missing %s in %#v", key, channel)
		}
	}
	if channel["name"] != "ops" {
		t.Fatalf("channel %#v", channel)
	}

	listed := requireStatus(t, do(t, app, http.MethodGet, "/registry/channels", ""), http.StatusOK)
	if rows, ok := listed["data"].([]any); !ok || len(rows) != 1 {
		t.Fatalf("channels %#v", listed["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/"+channelID, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/"+uuid.New().String(), ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/bad-id", ""), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+channelID, `{"name":""}`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+uuid.New().String(), `{"name":"x"}`), http.StatusNotFound)
	renamed := requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+channelID, `{"name":"ops-2"}`), http.StatusOK)
	if dataObject(t, renamed)["name"] != "ops-2" {
		t.Fatalf("renamed %#v", renamed)
	}

	thing := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"radio","type":"radio","department_id":"%s"}`, dept)), http.StatusCreated)
	thingID := dataID(t, thing)
	eventsBeforeACL := len(pub.msgs)

	requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/"+uuid.New().String()+"/acl", ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+channelID+"/acl", `{}`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+channelID+"/acl", fmt.Sprintf(`{"thing_id":"%s"}`, uuid.New())), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+uuid.New().String()+"/acl", fmt.Sprintf(`{"thing_id":"%s","can_publish":true}`, thingID)), http.StatusNotFound)

	granted := requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+channelID+"/acl", fmt.Sprintf(
		`{"thing_id":"%s","can_publish":true,"can_subscribe":false}`, thingID)), http.StatusCreated)
	acl := dataObject(t, granted)
	aclID := dataID(t, granted)
	for _, key := range []string{"id", "channel_id", "thing_id", "can_publish", "can_subscribe", "created_at"} {
		if _, ok := acl[key]; !ok {
			t.Fatalf("missing %s in %#v", key, acl)
		}
	}
	if acl["channel_id"] != channelID || acl["thing_id"] != thingID || acl["can_publish"] != true || acl["can_subscribe"] != false {
		t.Fatalf("acl %#v", acl)
	}
	if len(pub.msgs) != eventsBeforeACL {
		t.Fatalf("acl grant published %#v", pub.subjects())
	}
	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+channelID+"/acl", fmt.Sprintf(
		`{"thing_id":"%s","can_publish":true}`, thingID)), http.StatusConflict)

	aclList := requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/"+channelID+"/acl", ""), http.StatusOK)
	if rows, ok := aclList["data"].([]any); !ok || len(rows) != 1 {
		t.Fatalf("acl list %#v", aclList["data"])
	}
	requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+channelID+"/acl/"+uuid.New().String(), `{"can_publish":false,"can_subscribe":true}`), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+channelID+"/acl/bad", `{"can_publish":true}`), http.StatusBadRequest)
	changed := requireStatus(t, do(t, app, http.MethodPut, "/registry/channels/"+channelID+"/acl/"+aclID, `{"can_publish":false,"can_subscribe":true}`), http.StatusOK)
	changedACL := dataObject(t, changed)
	if changedACL["can_publish"] != false || changedACL["can_subscribe"] != true {
		t.Fatalf("updated acl %#v", changedACL)
	}
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/channels/"+channelID+"/acl/"+uuid.New().String(), ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/channels/"+channelID+"/acl/"+aclID, ""), http.StatusOK)
	empty := requireStatus(t, do(t, app, http.MethodGet, "/registry/channels/"+channelID+"/acl", ""), http.StatusOK)
	if rows, ok := empty["data"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("acl after delete %#v", empty["data"])
	}

	requireStatus(t, do(t, app, http.MethodPost, "/registry/channels/"+channelID+"/acl", fmt.Sprintf(
		`{"thing_id":"%s","can_publish":true,"can_subscribe":true}`, thingID)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/channels/"+uuid.New().String(), ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/channels/"+channelID, ""), http.StatusOK)
	var left int64
	if err := db.Raw(`SELECT COUNT(*) FROM channel_acl WHERE channel_id = ?`, channelID).Scan(&left).Error; err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("acl rows left = %d", left)
	}
}

func TestGroupRoutes(t *testing.T) {
	truncate(t)
	pub := &recordingPublisher{}
	app := newApp(pub)
	dept := uuid.New().String()

	requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", `{"name":"root"}`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", `{`), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(`{"name":"root","department_id":"%s","parent_group_id":"%s"}`, dept, uuid.New())), http.StatusBadRequest)

	root := requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(`{"name":"root","department_id":"%s"}`, dept)), http.StatusCreated)
	rootObj := dataObject(t, root)
	rootID := dataID(t, root)
	for _, key := range []string{"id", "name", "parent_group_id", "department_id", "created_at", "updated_at"} {
		if _, ok := rootObj[key]; !ok {
			t.Fatalf("missing %s in %#v", key, rootObj)
		}
	}
	if rootObj["parent_group_id"] != nil || rootObj["department_id"] != dept {
		t.Fatalf("root %#v", rootObj)
	}
	child := requireStatus(t, do(t, app, http.MethodPost, "/registry/groups", fmt.Sprintf(
		`{"name":"child","department_id":"%s","parent_group_id":"%s"}`, dept, rootID)), http.StatusCreated)
	childID := dataID(t, child)

	all := requireStatus(t, do(t, app, http.MethodGet, "/registry/groups", ""), http.StatusOK)
	if rows, ok := all["data"].([]any); !ok || len(rows) != 2 {
		t.Fatalf("all groups %#v", all["data"])
	}
	roots := requireStatus(t, do(t, app, http.MethodGet, "/registry/groups?parent_group_id=null", ""), http.StatusOK)
	rootRows, ok := roots["data"].([]any)
	if !ok || len(rootRows) != 1 || rootRows[0].(map[string]any)["id"] != rootID {
		t.Fatalf("roots %#v", roots["data"])
	}
	children := requireStatus(t, do(t, app, http.MethodGet, "/registry/groups?parent_group_id="+rootID, ""), http.StatusOK)
	childRows, ok := children["data"].([]any)
	if !ok || len(childRows) != 1 || childRows[0].(map[string]any)["id"] != childID {
		t.Fatalf("children %#v", children["data"])
	}
	requireStatus(t, do(t, app, http.MethodGet, "/registry/groups?parent_group_id=bad", ""), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/groups/"+rootID, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/groups/"+uuid.New().String(), ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/groups/bad-id", ""), http.StatusBadRequest)

	requireStatus(t, do(t, app, http.MethodPut, "/registry/groups/"+rootID, fmt.Sprintf(
		`{"name":"root","department_id":"%s","parent_group_id":"%s"}`, dept, rootID)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/groups/"+rootID, fmt.Sprintf(
		`{"name":"root","department_id":"%s","parent_group_id":"%s"}`, dept, childID)), http.StatusBadRequest)
	requireStatus(t, do(t, app, http.MethodPut, "/registry/groups/"+uuid.New().String(), fmt.Sprintf(`{"name":"x","department_id":"%s"}`, dept)), http.StatusNotFound)
	renamed := requireStatus(t, do(t, app, http.MethodPut, "/registry/groups/"+childID, fmt.Sprintf(
		`{"name":"child-2","department_id":"%s","parent_group_id":"%s"}`, dept, rootID)), http.StatusOK)
	if dataObject(t, renamed)["name"] != "child-2" {
		t.Fatalf("renamed %#v", renamed)
	}

	conflict := requireStatus(t, do(t, app, http.MethodDelete, "/registry/groups/"+rootID, ""), http.StatusConflict)
	if conflict["success"] != false || !strings.Contains(fmt.Sprint(conflict["error"]), "child groups") {
		t.Fatalf("conflict %#v", conflict)
	}
	requireStatus(t, do(t, app, http.MethodGet, "/registry/groups/"+rootID, ""), http.StatusOK)

	requireStatus(t, do(t, app, http.MethodDelete, "/registry/groups/"+childID, ""), http.StatusOK)
	thing := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"member","type":"sensor","department_id":"%s","group_id":"%s"}`, dept, rootID)), http.StatusCreated)
	memberConflict := requireStatus(t, do(t, app, http.MethodDelete, "/registry/groups/"+rootID, ""), http.StatusConflict)
	if !strings.Contains(fmt.Sprint(memberConflict["error"]), "things belong") {
		t.Fatalf("member conflict %#v", memberConflict)
	}
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+dataID(t, thing), ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/groups/"+rootID, ""), http.StatusOK)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/groups/"+rootID, ""), http.StatusNotFound)
	if len(pub.subjects()) != 2 {
		t.Fatalf("group routes should only publish thing events, got %#v", pub.subjects())
	}
}

func TestAPISecretHashIsStoredAndNeverReturned(t *testing.T) {
	truncate(t)
	const hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(&recordingPublisher{})
	dept := uuid.New()
	created := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"tracker","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	thingID := uuid.MustParse(dataID(t, created))

	cred := &models.DeviceCredential{
		ThingID:       thingID,
		APIKey:        "device-key-1",
		APISecretHash: models.SecretHash(hash),
		Status:        "active",
	}
	if err := db.Create(cred).Error; err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.Raw(`SELECT api_secret_hash FROM device_credentials WHERE id = ?`, cred.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != hash {
		t.Fatalf("stored hash = %q", stored)
	}

	ref := &models.ThingExternalRef{ThingID: thingID, ExternalSystem: "traccar-go", ExternalID: "dev-1"}
	if err := db.Create(ref).Error; err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"/registry/things",
		"/registry/things/" + thingID.String(),
		"/registry/things/" + thingID.String() + "/credentials",
		"/registry/channels",
		"/registry/groups",
		"/registry/device-credentials",
	}
	for _, path := range paths {
		resp := do(t, app, http.MethodGet, path, "")
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), hash) || strings.Contains(string(raw), "api_secret_hash") {
			t.Fatalf("%s leaked secret material: %s", path, raw)
		}
	}
	if strings.Contains(logs.String(), hash) {
		t.Fatalf("logs leaked the hash: %s", logs.String())
	}

	blocked := requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID.String(), ""), http.StatusConflict)
	if !strings.Contains(fmt.Sprint(blocked["error"]), "credentials") {
		t.Fatalf("delete blocked body %#v", blocked)
	}
}

func TestCredentialIssueListAndRevoke(t *testing.T) {
	truncate(t)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(osStderr) })

	app := newApp(&recordingPublisher{})
	dept := uuid.New()
	created := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"tracker","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	thingID := dataID(t, created)
	missing := uuid.New().String()

	requireStatus(t, do(t, app, http.MethodPost, "/registry/things/"+missing+"/credentials", ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+missing+"/credentials", ""), http.StatusNotFound)
	requireStatus(t, do(t, app, http.MethodPost, "/registry/things/not-a-uuid/credentials", ""), http.StatusBadRequest)

	issuedBody := requireStatus(t, do(t, app, http.MethodPost, "/registry/things/"+thingID+"/credentials", ""), http.StatusCreated)
	issued := dataObject(t, issuedBody)
	if len(issued) != 2 {
		t.Fatalf("issue data keys = %#v", issued)
	}
	apiKey, _ := issued["api_key"].(string)
	apiSecret, _ := issued["api_secret"].(string)
	if len(apiKey) != 64 || len(apiSecret) != 64 || apiKey == apiSecret {
		t.Fatalf("issued key material %#v", issued)
	}

	var stored struct {
		ID     uuid.UUID
		Hash   string
		Status string
	}
	if err := db.Raw(`SELECT id, api_secret_hash AS hash, status FROM device_credentials WHERE api_key = ?`, apiKey).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "active" || stored.Hash == apiSecret || bcrypt.CompareHashAndPassword([]byte(stored.Hash), []byte(apiSecret)) != nil {
		t.Fatalf("stored credential id=%s status=%s", stored.ID, stored.Status)
	}

	listed := requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+thingID+"/credentials", ""), http.StatusOK)
	rawList, _ := json.Marshal(listed)
	if strings.Contains(string(rawList), apiSecret) || strings.Contains(string(rawList), stored.Hash) || strings.Contains(string(rawList), "api_secret") {
		t.Fatalf("list leaked secret material: %s", rawList)
	}
	items, ok := listed["data"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("list data %#v", listed["data"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("list item %#v", items[0])
	}
	for _, key := range []string{"id", "api_key", "status", "created_at", "revoked_at"} {
		if _, present := item[key]; !present {
			t.Fatalf("list item missing %s: %#v", key, item)
		}
	}
	if len(item) != 5 || item["api_key"] != apiKey || item["status"] != "active" || item["revoked_at"] != nil {
		t.Fatalf("list item %#v", item)
	}

	thingBody := requireStatus(t, do(t, app, http.MethodGet, "/registry/things/"+thingID, ""), http.StatusOK)
	thingRaw, _ := json.Marshal(thingBody)
	if strings.Contains(string(thingRaw), apiSecret) || strings.Contains(string(thingRaw), stored.Hash) {
		t.Fatalf("thing response leaked secret material: %s", thingRaw)
	}

	revoked := requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID+"/credentials/"+stored.ID.String(), ""), http.StatusOK)
	revokedItem := dataObject(t, revoked)
	if revokedItem["status"] != "revoked" || revokedItem["revoked_at"] == nil || revokedItem["api_key"] != apiKey {
		t.Fatalf("revoked item %#v", revokedItem)
	}
	revokedRaw, _ := json.Marshal(revoked)
	if strings.Contains(string(revokedRaw), apiSecret) || strings.Contains(string(revokedRaw), stored.Hash) {
		t.Fatalf("revoke response leaked secret material: %s", revokedRaw)
	}

	var after struct {
		Status    string
		RevokedAt *time.Time
		Count     int64
	}
	if err := db.Raw(`SELECT status, revoked_at, count(*) OVER () AS count FROM device_credentials WHERE id = ?`, stored.ID).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Status != "revoked" || after.RevokedAt == nil || after.Count != 1 {
		t.Fatalf("row after revoke %#v", after)
	}

	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID+"/credentials/"+stored.ID.String(), ""), http.StatusConflict)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+thingID+"/credentials/"+uuid.New().String(), ""), http.StatusNotFound)

	other := requireStatus(t, do(t, app, http.MethodPost, "/registry/things", fmt.Sprintf(
		`{"name":"other","type":"vehicle","department_id":"%s"}`, dept)), http.StatusCreated)
	requireStatus(t, do(t, app, http.MethodDelete, "/registry/things/"+dataID(t, other)+"/credentials/"+stored.ID.String(), ""), http.StatusNotFound)

	logged := logs.String()
	if strings.Contains(logged, apiSecret) || strings.Contains(logged, stored.Hash) {
		t.Fatal("logs contained the secret or its hash")
	}
	for _, line := range []string{
		"credential issued thing_id=" + thingID,
		"credential list thing_id=" + thingID + " count=1",
		"credential revoked thing_id=" + thingID,
		"credential revoke rejected thing_id=" + thingID + " credential_id=" + stored.ID.String() + " reason=already revoked",
		"credential issue rejected reason=invalid thing id",
		"credential issue rejected thing_id=" + missing + " reason=thing not found",
	} {
		if !strings.Contains(logged, line) {
			t.Fatalf("log missing %q\n%s", line, logged)
		}
	}
}

func assertEvent(t *testing.T, msg published, subject, thingID string, trackable bool) {
	t.Helper()
	if msg.subject != subject {
		t.Fatalf("subject %s, want %s", msg.subject, subject)
	}
	var ev models.ThingEvent
	if err := json.Unmarshal(msg.data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Event != subject || ev.ThingID.String() != thingID || ev.IsTrackable != trackable || ev.OccurredAt.IsZero() {
		t.Fatalf("event %#v", ev)
	}
}

var osStderr = log.Default().Writer()
