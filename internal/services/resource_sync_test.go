package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestUnitCreateDoesNotCallCaguarWhenTypeIsMissing(t *testing.T) {
	h := newUnitHarness(t)
	_, err := h.svc.Create(context.Background(), unitRequest(h, ""))
	if errFields(t, err)["resource_type_id"] == "" {
		t.Fatal(err)
	}
	if h.portal.device.UniqueID != "" {
		t.Fatalf("caguar was called: %#v", h.portal.device)
	}
	if len(h.resources.rows) != 0 {
		t.Fatalf("resource stored %#v", h.resources.rows)
	}
}

func TestUnitCreateStaysLocalWhenCaguarIsDown(t *testing.T) {
	h := newUnitHarness(t)
	h.portal.err = fmt.Errorf("dial tcp: connection refused")
	req := unitRequest(h, h.typeID.String())
	got, err := h.svc.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.UniqueID != "UNIT1001" || len(h.resources.rows) != 1 || len(h.outbox.jobs) != 1 {
		t.Fatalf("response %#v resources %d jobs %#v", got, len(h.resources.rows), h.outbox.jobs)
	}
	if h.outbox.jobs[0].Action != models.CaguarSyncCreate || h.outbox.jobs[0].UniqueID != "UNIT1001" {
		t.Fatalf("job %#v", h.outbox.jobs[0])
	}
	if h.portal.device.Product != caguar.ProductAutomax || h.portal.device.PlateNumber != "ENG-1001" || h.portal.device.ChassisNumber != "CHS1001" {
		t.Fatalf("payload %#v", h.portal.device)
	}
}

func TestUnitUpdateSavesWhenCaguarLocksTheAutomaxDevice(t *testing.T) {
	h := newUnitHarness(t)
	thingID := uuid.New()
	resourceID := uuid.New()
	h.db.things[thingID] = &models.Thing{ID: thingID, Name: "Engine 1", Type: "vehicle", IsTrackable: true, DepartmentID: h.dept, Metadata: models.JSON(`{}`)}
	h.resources.rows[resourceID] = &models.Resource{ID: resourceID, ThingID: thingID, Name: "Engine 1", ResourceTypeID: h.typeID, Status: models.ResourceAvailable, UniqueID: "UNIT1322", PlateNumber: "ENG-1322", IsActive: true}
	h.owners.ref = &models.ThingExternalRef{ID: uuid.New(), ThingID: thingID, ExternalSystem: models.ExternalSystemTraccar, ExternalID: "UNIT1322"}
	h.portal.err = &caguar.RemoteError{Status: 400, Body: `{"http_code":400,"error":"This device is registered as an AutoMax device and cannot be accessed through this system. Please contact the administrator.","status":"RESTRICTED_PRODUCT"}`}

	name := "Engine 1355"
	plate := "ENG-1322"
	req := unitRequest(h, h.typeID.String())
	req.Name = &name
	req.PlateNumber = &plate
	req.ThingID = thingID
	req.UniqueID = "UNIT1322"
	_, err := h.svc.Update(context.Background(), resourceID, req)
	if err == nil || !strings.Contains(err.Error(), "AutoMax device") {
		t.Fatal(err)
	}
	if h.db.things[thingID].Name != "Engine 1" || h.resources.rows[resourceID].PlateNumber != "ENG-1322" {
		t.Fatalf("local copy changed thing %#v row %#v", h.db.things[thingID], h.resources.rows[resourceID])
	}
}

func TestDeletingAQueuedUnitDoesNotCreateItLater(t *testing.T) {
	h := newUnitHarness(t)
	h.portal.err = fmt.Errorf("dial tcp: connection refused")
	got, err := h.svc.Create(context.Background(), unitRequest(h, h.typeID.String()))
	if err != nil {
		t.Fatal(err)
	}
	h.portal.err = nil
	h.portal.device = caguar.Device{}
	if err := h.svc.Delete(context.Background(), got.ID); err != nil {
		t.Fatal(err)
	}
	if len(h.outbox.jobs) != 0 || h.portal.deleted != "" || h.portal.device.UniqueID != "" {
		t.Fatalf("jobs %#v deleted %q device %#v", h.outbox.jobs, h.portal.deleted, h.portal.device)
	}
}

func TestUnitCreateRollsBackWhenCaguarAlreadyHasTheChassis(t *testing.T) {
	h := newUnitHarness(t)
	h.portal.err = &caguar.RemoteError{Status: 400, Body: `{"error":"chassis number already exists"}`}
	_, err := h.svc.Create(context.Background(), unitRequest(h, h.typeID.String()))
	if errFields(t, err)["chassis_number"] != "chassis_number is already used in Caguar" {
		t.Fatal(err)
	}
	if len(h.resources.rows) != 0 || len(h.db.things) != 0 {
		t.Fatalf("left behind resources=%d things=%d", len(h.resources.rows), len(h.db.things))
	}
}

func TestUnitUpdateAndDeleteReachCaguar(t *testing.T) {
	h := newUnitHarness(t)
	thingID := uuid.New()
	resourceID := uuid.New()
	h.db.things[thingID] = &models.Thing{ID: thingID, Name: "Engine 1", Type: "vehicle", IsTrackable: true, DepartmentID: h.dept, Metadata: models.JSON(`{}`)}
	h.resources.rows[resourceID] = &models.Resource{ID: resourceID, ThingID: thingID, Name: "Engine 1", ResourceTypeID: h.typeID, Status: models.ResourceAvailable, UniqueID: "UNIT1001", PlateNumber: "ENG-1001", IsActive: true}
	h.owners.ref = &models.ThingExternalRef{ID: uuid.New(), ThingID: thingID, ExternalSystem: models.ExternalSystemTraccar, ExternalID: "UNIT1001"}

	name := "Engine 2"
	plate := "ENG-1002"
	req := unitRequest(h, h.typeID.String())
	req.Name = &name
	req.PlateNumber = &plate
	req.ThingID = thingID
	if _, err := h.svc.Update(context.Background(), resourceID, req); err != nil {
		t.Fatal(err)
	}
	if h.portal.device.Name != "Engine 2" || h.portal.device.PlateNumber != "ENG-1002" || h.portal.device.Product != caguar.ProductAutomax {
		t.Fatalf("update payload %#v", h.portal.device)
	}

	if err := h.svc.Delete(context.Background(), resourceID); err != nil {
		t.Fatal(err)
	}
	if h.portal.deleted != "UNIT1001" {
		t.Fatalf("deleted %q", h.portal.deleted)
	}
	if _, ok := h.resources.rows[resourceID]; ok {
		t.Fatal("resource still stored")
	}
}

func TestQueuedCreateIsSentWhenCaguarReturns(t *testing.T) {
	h := newUnitHarness(t)
	thingID := uuid.New()
	raw := []byte(`{"name":"Engine 9","unique_id":"UNIT9","department_id":"` + h.dept.String() + `","tracker_type":"Vehicle","plate_number":"ENG-1009","chassis_number":"CHS1009"}`)
	job := models.CaguarSyncJob{ID: uuid.New(), ThingID: thingID, Action: models.CaguarSyncCreate, UniqueID: "UNIT9", Payload: models.JSON(raw)}
	h.outbox.jobs = append(h.outbox.jobs, job)
	sync := NewCaguarSync(&catalogStub{}, h.refs, h.repoThings)
	sync.UseOutbox(h.outbox)
	sync.UseWriter(h.portal)
	sync.Refresh(context.Background())
	if h.portal.device.UniqueID != "UNIT9" || h.portal.device.Product != caguar.ProductAutomax {
		t.Fatalf("flushed %#v", h.portal.device)
	}
	if len(h.outbox.jobs) != 0 {
		t.Fatalf("job remained %#v", h.outbox.jobs)
	}
	if h.refs.created == nil || h.refs.created.ExternalID != "UNIT9" {
		t.Fatalf("ref %#v", h.refs.created)
	}
}

type unitHarness struct {
	svc        *ResourceService
	portal     *syncPortal
	db         *memDB
	resources  *memResources
	outbox     *memOutbox
	refs       *memRefs
	repoThings *repoThings
	owners     *recordingOwners
	dept       uuid.UUID
	typeID     uuid.UUID
}

func newUnitHarness(t *testing.T) *unitHarness {
	t.Helper()
	dept := uuid.New()
	typeID := uuid.New()
	db := &memDB{things: map[uuid.UUID]*models.Thing{}}
	resources := &memResources{rows: map[uuid.UUID]*models.Resource{}}
	portal := &syncPortal{}
	owners := &recordingOwners{}
	refs := &memRefs{}
	outbox := &memOutbox{}
	repo := &repoThings{db: db}
	portalThings := &portalThings{db: db}
	svc := NewResourceService(resources, repo, &memTypes{id: typeID, dept: dept}, &stubPoints{})
	svc.UseDevicePortal(NewDevicePortal(portalThings, fakeLinker{}, owners, portal))
	svc.UseCaguarOutbox(outbox)
	return &unitHarness{svc: svc, portal: portal, db: db, resources: resources, outbox: outbox, refs: refs, repoThings: repo, owners: owners, dept: dept, typeID: typeID}
}

func unitRequest(h *unitHarness, typeID string) models.ResourceWriteRequest {
	name := "Engine 1001"
	plate := "ENG-1001"
	parsed := uuid.Nil
	if typeID != "" {
		parsed = uuid.MustParse(typeID)
	}
	return models.ResourceWriteRequest{
		Name: &name, ResourceTypeID: parsed, UniqueID: "UNIT1001", TrackerType: "Vehicle",
		PlateNumber: &plate, ChassisNumber: "CHS1001", DepartmentID: h.dept,
	}
}

type memDB struct {
	things map[uuid.UUID]*models.Thing
}

type repoThings struct{ db *memDB }

func (r *repoThings) Create(_ context.Context, thing *models.Thing) error {
	if thing.ID == uuid.Nil {
		thing.ID = uuid.New()
	}
	copy := *thing
	r.db.things[thing.ID] = &copy
	return nil
}
func (r *repoThings) Update(_ context.Context, thing *models.Thing) error {
	if _, ok := r.db.things[thing.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	copy := *thing
	r.db.things[thing.ID] = &copy
	return nil
}
func (r *repoThings) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.db.things, id)
	return nil
}
func (r *repoThings) FindByID(_ context.Context, id uuid.UUID) (*models.Thing, error) {
	thing, ok := r.db.things[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *thing
	return &copy, nil
}
func (r *repoThings) List(context.Context, models.ThingListFilter) ([]models.Thing, error) {
	return nil, nil
}
func (r *repoThings) BlockingRefCount(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (r *repoThings) ResourceCount(context.Context, uuid.UUID) (int64, error)    { return 0, nil }
func (r *repoThings) Exists(_ context.Context, id uuid.UUID) (bool, error) {
	_, ok := r.db.things[id]
	return ok, nil
}
func (r *repoThings) UpdateCoordinates(context.Context, uuid.UUID, float64, float64) error {
	return nil
}

type portalThings struct{ db *memDB }

func (p *portalThings) Create(context.Context, models.ThingCreateRequest) (*models.ThingResponse, error) {
	return nil, errors.New("unused")
}
func (p *portalThings) Get(_ context.Context, id uuid.UUID) (*models.ThingResponse, error) {
	thing, ok := p.db.things[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &models.ThingResponse{ID: thing.ID, Name: thing.Name, Type: thing.Type, IsTrackable: thing.IsTrackable, DepartmentID: thing.DepartmentID, Metadata: thing.Metadata}, nil
}
func (p *portalThings) Update(_ context.Context, id uuid.UUID, req models.ThingUpdateRequest) (*models.ThingResponse, error) {
	thing, ok := p.db.things[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	thing.Name = req.Name
	thing.Metadata = req.Metadata
	thing.DepartmentID = req.DepartmentID
	return p.Get(context.Background(), id)
}

type memResources struct {
	rows map[uuid.UUID]*models.Resource
}

func (m *memResources) Create(_ context.Context, row *models.Resource) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	copy := *row
	m.rows[row.ID] = &copy
	return nil
}
func (m *memResources) Update(_ context.Context, row *models.Resource, _ uuid.UUID) error {
	copy := *row
	m.rows[row.ID] = &copy
	return nil
}
func (m *memResources) SetIncident(_ context.Context, id uuid.UUID, incidentID *uuid.UUID) error {
	row, ok := m.rows[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	row.IncidentID = incidentID
	return nil
}
func (m *memResources) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.rows[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.rows, id)
	return nil
}
func (m *memResources) FindByID(_ context.Context, id uuid.UUID) (*models.Resource, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *row
	return &copy, nil
}
func (m *memResources) FindByUniqueID(_ context.Context, uniqueID string) (*models.Resource, error) {
	for _, row := range m.rows {
		if row.UniqueID == uniqueID {
			copy := *row
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *memResources) List(context.Context, models.ResourceListFilter) ([]models.Resource, int64, error) {
	return nil, 0, nil
}
func (m *memResources) Exists(_ context.Context, id uuid.UUID) (bool, error) {
	_, ok := m.rows[id]
	return ok, nil
}
func (m *memResources) SetStatus(context.Context, uuid.UUID, string) error { return nil }
func (m *memResources) DriversByServicePoint(context.Context, uuid.UUID) ([]models.ResourceDriver, error) {
	return nil, nil
}
func (m *memResources) SaveIncidentMapping(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) error {
	return nil
}
func (m *memResources) IncidentIDsByResourceAndService(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *memResources) FindByThingID(context.Context, uuid.UUID) (*models.Resource, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *memResources) UpdatePlate(context.Context, uuid.UUID, string) error { return nil }

type memTypes struct {
	id   uuid.UUID
	dept uuid.UUID
}

func (m *memTypes) Create(context.Context, *models.ResourceType) error { return nil }
func (m *memTypes) Update(context.Context, *models.ResourceType) error { return nil }
func (m *memTypes) Delete(context.Context, uuid.UUID) error            { return nil }
func (m *memTypes) FindByID(_ context.Context, id uuid.UUID) (*models.ResourceType, error) {
	if id != m.id {
		return nil, gorm.ErrRecordNotFound
	}
	return &models.ResourceType{ID: m.id, DepartmentID: m.dept, Name: "Truck"}, nil
}
func (m *memTypes) List(context.Context, models.ResourceTypeListFilter) ([]models.ResourceType, int64, error) {
	return nil, 0, nil
}
func (m *memTypes) Exists(_ context.Context, id uuid.UUID) (bool, error) { return id == m.id, nil }
func (m *memTypes) ResourceCount(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

type memOutbox struct {
	jobs []models.CaguarSyncJob
}

func (m *memOutbox) Enqueue(_ context.Context, job *models.CaguarSyncJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	m.jobs = append(m.jobs, *job)
	return nil
}
func (m *memOutbox) List(context.Context) ([]models.CaguarSyncJob, error) {
	return append([]models.CaguarSyncJob(nil), m.jobs...), nil
}
func (m *memOutbox) Delete(_ context.Context, id uuid.UUID) error {
	next := m.jobs[:0]
	for _, job := range m.jobs {
		if job.ID != id {
			next = append(next, job)
		}
	}
	m.jobs = next
	return nil
}
func (m *memOutbox) MarkError(context.Context, uuid.UUID, string) error { return nil }

type memRefs struct {
	created *models.ThingExternalRef
}

func (m *memRefs) FindLinkedThing(context.Context, string, string) (repository.LinkedThing, error) {
	return repository.LinkedThing{}, gorm.ErrRecordNotFound
}
func (m *memRefs) FindByThingSystem(context.Context, uuid.UUID, string) (*models.ThingExternalRef, error) {
	return nil, gorm.ErrRecordNotFound
}
func (m *memRefs) CountOtherOwners(context.Context, string, string, uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *memRefs) ListNamed(context.Context, string) ([]repository.NamedExternalRef, error) {
	return nil, nil
}
func (m *memRefs) Create(_ context.Context, ref *models.ThingExternalRef) error {
	m.created = ref
	return nil
}
func (m *memRefs) UpdateExternalID(context.Context, uuid.UUID, string, time.Time) error {
	return nil
}
func (m *memRefs) Delete(context.Context, uuid.UUID) error { return nil }

type syncPortal struct {
	device  caguar.Device
	deleted string
	err     error
}

func (p *syncPortal) RegisterDevice(_ context.Context, device caguar.Device) error {
	p.device = device
	return p.err
}
func (p *syncPortal) UpdateDevice(_ context.Context, _ string, device caguar.Device) error {
	p.device = device
	return p.err
}
func (p *syncPortal) DeleteDevice(_ context.Context, uniqueID string) error {
	p.deleted = uniqueID
	return p.err
}
func (p *syncPortal) RegisterDriver(context.Context, caguar.Driver) error { return p.err }

type catalogStub struct{}

func (catalogStub) Devices(context.Context) ([]caguar.Device, error) { return nil, nil }
func (catalogStub) Positions(context.Context) ([]caguar.Position, error) {
	return nil, nil
}
