package services

import (
	"context"
	"errors"
	"testing"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

func TestDeviceRegisterValidatesAndSendsToCaguar(t *testing.T) {
	dept := uuid.New()
	portal := &fakePortal{}
	things := &fakeCreator{id: uuid.New()}
	svc := NewDevicePortal(things, fakeLinker{}, fakeOwners{}, portal)

	_, err := svc.Register(context.Background(), DeviceRegistration{DepartmentID: dept})
	if fields := errFields(t, err); fields["name"] == "" || fields["unique_id"] == "" || fields["tracker_type"] == "" || fields["plate_number"] == "" || fields["chassis_number"] == "" {
		t.Fatalf("fields %#v", fields)
	}

	_, err = svc.Register(context.Background(), DeviceRegistration{
		Name: "Truck", UniqueID: "TESTGO123", DepartmentID: dept,
		TrackerType: "Vehicle", PlateNumber: "bad", ChassisNumber: "CHS-0001",
	})
	if errFields(t, err)["plate_number"] == "" {
		t.Fatal(err)
	}

	got, err := svc.Register(context.Background(), DeviceRegistration{
		Name: " Test Device Go ", UniqueIDCamel: "TESTGO123", DepartmentID: dept,
		TrackerTypeCamel: "Vehicle", PlateCamel: "abc-1234", ChassisCamel: "CHS-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ThingID != things.id || got.UniqueID != "TESTGO123" || got.PlateNumber != "ABC-1234" || got.Name != "Test Device Go" {
		t.Fatalf("result %#v", got)
	}
	if portal.device.Name != "Test Device Go" || portal.device.UniqueID != "TESTGO123" || portal.device.PlateNumber != "ABC-1234" || portal.device.TrackerType != "Vehicle" || portal.device.Status != "" || portal.device.Category != "car" || portal.device.ChassisNumber != "CHS-0001" {
		t.Fatalf("caguar %#v", portal.device)
	}

	portal.err = &caguar.RemoteError{Status: 400, Body: `{"error":"A device with this name already exists","status":"DUPLICATE_DEVICE"}`}
	_, err = svc.Register(context.Background(), DeviceRegistration{
		Name: "Other", UniqueID: "OTHER1", DepartmentID: dept,
		TrackerType: "Vehicle", PlateNumber: "ABC-1234", ChassisNumber: "CHS-0001",
	})
	var se *Error
	if !errors.As(err, &se) || se.Kind != KindConflict || se.Message != "A device with this name already exists" {
		t.Fatalf("conflict %#v", err)
	}

	portal.err = &caguar.RemoteError{Status: 500, Body: `{"error":"INTERNAL_ERROR"}`}
	_, err = svc.Register(context.Background(), DeviceRegistration{
		Name: "Other", UniqueID: "OTHER1", DepartmentID: dept,
		TrackerType: "Vehicle", PlateNumber: "ABC-1234", ChassisNumber: "CHS-0001",
	})
	if !errors.As(err, &se) || se.Kind != KindUpstream || se.Message != "INTERNAL_ERROR" {
		t.Fatalf("upstream %#v", err)
	}
}

func TestDriverRegisterRequiresNameAndUniqueID(t *testing.T) {
	svc := NewDriverPortal(&fakePortal{})
	_, err := svc.Register(context.Background(), DriverRegistration{})
	if fields := errFields(t, err); fields["name"] == "" || fields["unique_id"] == "" {
		t.Fatalf("fields %#v", fields)
	}
	if _, err := NewDriverPortal(nil).Register(context.Background(), DriverRegistration{Name: "A", UniqueID: "B"}); err == nil {
		t.Fatal("expected unconfigured caguar")
	}
	portal := &fakePortal{}
	got, err := NewDriverPortal(portal).Register(context.Background(), DriverRegistration{Name: " Test Driver Go ", UniqueIDCamel: "DRV001"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Test Driver Go" || got.UniqueID != "DRV001" || portal.driver.UniqueID != "DRV001" {
		t.Fatalf("driver %#v portal %#v", got, portal.driver)
	}
}

func TestFindNearestServicePoints(t *testing.T) {
	near := uuid.New()
	far := uuid.New()
	repo := &stubPoints{rows: []models.NearestServicePoint{
		{ID: near, Name: "Near", DistanceMeters: 10},
		{ID: far, Name: "Far", DistanceMeters: 40},
	}}
	svc := NewServicePointService(repo)
	rows, err := svc.FindNearest(context.Background(), 12.97, 77.59, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != near || rows[0].DistanceMeters > rows[1].DistanceMeters {
		t.Fatalf("rows %#v", rows)
	}
	typeID := uuid.New()
	if _, err := svc.FindNearest(context.Background(), 12.97, 77.59, &typeID, 5); err != nil {
		t.Fatal(err)
	}
	if repo.typ == nil || *repo.typ != typeID || repo.limit != 5 {
		t.Fatalf("filter %#v limit %d", repo.typ, repo.limit)
	}
	if _, err := svc.FindNearest(context.Background(), 91, 0, nil, 1); errFields(t, err)["lat"] == "" {
		t.Fatal(err)
	}
}

type fakePortal struct {
	device caguar.Device
	driver caguar.Driver
	err    error
}

func (f *fakePortal) RegisterDevice(_ context.Context, device caguar.Device) error {
	f.device = device
	return f.err
}

func (f *fakePortal) RegisterDriver(_ context.Context, driver caguar.Driver) error {
	f.driver = driver
	return f.err
}

type fakeCreator struct{ id uuid.UUID }

func (f fakeCreator) Create(_ context.Context, req models.ThingCreateRequest) (*models.ThingResponse, error) {
	return &models.ThingResponse{ID: f.id, Name: req.Name, DepartmentID: req.DepartmentID}, nil
}

type fakeLinker struct{}

func (fakeLinker) Upsert(context.Context, uuid.UUID, models.ExternalRefRequest) (*models.ThingExternalRef, bool, error) {
	return &models.ThingExternalRef{}, true, nil
}

type fakeOwners struct{}

func (fakeOwners) CountOtherOwners(context.Context, string, string, uuid.UUID) (int64, error) {
	return 0, nil
}

type stubPoints struct {
	rows  []models.NearestServicePoint
	typ   *uuid.UUID
	limit int
}

func (s *stubPoints) Nearest(_ context.Context, _, _ float64, resourceTypeID *uuid.UUID, limit int) ([]models.NearestServicePoint, error) {
	s.typ = resourceTypeID
	s.limit = limit
	return s.rows, nil
}

func (s *stubPoints) Create(context.Context, *models.ServicePoint) error { return nil }
func (s *stubPoints) Update(context.Context, *models.ServicePoint) error { return nil }
func (s *stubPoints) Delete(context.Context, uuid.UUID) error            { return nil }
func (s *stubPoints) FindByID(context.Context, uuid.UUID) (*models.ServicePoint, error) {
	return nil, nil
}
func (s *stubPoints) List(context.Context, models.ServicePointListFilter) ([]models.ServicePoint, int64, error) {
	return nil, 0, nil
}
func (s *stubPoints) AssignResources(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
func (s *stubPoints) Exists(context.Context, uuid.UUID) (bool, error)               { return false, nil }
func (s *stubPoints) ResourceCount(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

func errFields(t *testing.T, err error) map[string]string {
	t.Helper()
	var se *Error
	if !errors.As(err, &se) {
		t.Fatalf("err %#v", err)
	}
	return se.Fields
}
