package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

var platePattern = regexp.MustCompile(`^[A-Z]{3}-[0-9]{4}$`)

// Portal registers a device or a driver in the Caguar portal.
type Portal interface {
	RegisterDevice(ctx context.Context, device caguar.Device) error
	UpdateDevice(ctx context.Context, currentUniqueID string, device caguar.Device) error
	DeleteDevice(ctx context.Context, uniqueID string) error
	RegisterDriver(ctx context.Context, driver caguar.Driver) error
	UpdateDriver(ctx context.Context, currentUniqueID string, driver caguar.Driver) error
	DeleteDriver(ctx context.Context, uniqueID string) error
}

type thingCreator interface {
	Create(ctx context.Context, req models.ThingCreateRequest) (*models.ThingResponse, error)
	Get(ctx context.Context, id uuid.UUID) (*models.ThingResponse, error)
	Update(ctx context.Context, id uuid.UUID, req models.ThingUpdateRequest) (*models.ThingResponse, error)
}

type refLinker interface {
	Upsert(ctx context.Context, thingID uuid.UUID, req models.ExternalRefRequest) (*models.ThingExternalRef, bool, error)
}

// DevicePortal registers a trackable thing locally and the same device in Caguar.
type DevicePortal struct {
	things thingCreator
	refs   refLinker
	owners interface {
		CountOtherOwners(ctx context.Context, system, externalID string, thingID uuid.UUID) (int64, error)
		FindByThingSystem(ctx context.Context, thingID uuid.UUID, system string) (*models.ThingExternalRef, error)
		UpdateExternalID(ctx context.Context, id uuid.UUID, externalID string, syncedAt time.Time) error
		Delete(ctx context.Context, id uuid.UUID) error
	}
	portal Portal
}

func NewDevicePortal(things thingCreator, refs refLinker, owners interface {
	CountOtherOwners(ctx context.Context, system, externalID string, thingID uuid.UUID) (int64, error)
	FindByThingSystem(ctx context.Context, thingID uuid.UUID, system string) (*models.ThingExternalRef, error)
	UpdateExternalID(ctx context.Context, id uuid.UUID, externalID string, syncedAt time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
}, portal Portal) *DevicePortal {
	return &DevicePortal{things: things, refs: refs, owners: owners, portal: portal}
}

// Enabled reports whether device registration can reach Caguar.
func (s *DevicePortal) Enabled() bool {
	return s != nil && s.portal != nil
}

// DeviceRegistration is the registry body for one Caguar device.
type DeviceRegistration struct {
	Name             string    `json:"name"`
	UniqueID         string    `json:"unique_id"`
	UniqueIDCamel    string    `json:"uniqueId"`
	DepartmentID     uuid.UUID `json:"department_id"`
	TrackerType      string    `json:"tracker_type"`
	TrackerTypeCamel string    `json:"trackerType"`
	PlateNumber      string    `json:"plate_number"`
	PlateCamel       string    `json:"plateNumber"`
	ChassisNumber    string    `json:"chassis_number"`
	ChassisCamel     string    `json:"chassisNumber"`
	Phone            string    `json:"phone"`
	Model            string    `json:"model"`
	Contact          string    `json:"contact"`
	Status           string    `json:"status"`
	Category         string    `json:"category"`
	Disabled         *bool     `json:"disabled"`
}

// DeviceResult is stored locally and sent to Caguar.
type DeviceResult struct {
	ThingID       uuid.UUID `json:"thing_id"`
	Name          string    `json:"name"`
	UniqueID      string    `json:"unique_id"`
	DepartmentID  uuid.UUID `json:"department_id"`
	TrackerType   string    `json:"tracker_type"`
	PlateNumber   string    `json:"plate_number"`
	ChassisNumber string    `json:"chassis_number"`
}

func (s *DevicePortal) Register(ctx context.Context, req DeviceRegistration) (*DeviceResult, error) {
	if s == nil || s.portal == nil {
		return nil, Unavailable("caguar is not configured")
	}
	name, uniqueID, tracker, plate, chassis, status, category, disabled, fields := normalizeDevice(req)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	owners, err := s.owners.CountOtherOwners(ctx, models.ExternalSystemTraccar, uniqueID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	if owners > 0 {
		return nil, Conflict("external id is already linked to another thing")
	}

	if err := s.portal.RegisterDevice(ctx, caguar.Device{
		Name:          name,
		UniqueID:      uniqueID,
		Status:        status,
		Disabled:      disabled,
		Category:      category,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
		Phone:         strings.TrimSpace(req.Phone),
		Model:         strings.TrimSpace(req.Model),
		Contact:       strings.TrimSpace(req.Contact),
		ZoneIDs:       []int{},
		Attributes:    map[string]any{},
		Product:       caguar.ProductAutomax,
	}); err != nil {
		return nil, portalErr("device", uniqueID, err)
	}

	meta, err := json.Marshal(map[string]string{
		"unique_id":      uniqueID,
		"tracker_type":   tracker,
		"plate_number":   plate,
		"chassis_number": chassis,
	})
	if err != nil {
		return nil, err
	}
	thing, err := s.things.Create(ctx, models.ThingCreateRequest{
		Name:         name,
		Type:         "vehicle",
		IsTrackable:  true,
		DepartmentID: req.DepartmentID,
		Metadata:     models.JSON(meta),
	})
	if err != nil {
		log.Printf("registry: caguar device registered but local thing failed unique_id=%s err=%v", uniqueID, err)
		return nil, err
	}
	if _, _, err := s.refs.Upsert(ctx, thing.ID, models.ExternalRefRequest{
		ExternalSystem: models.ExternalSystemTraccar,
		ExternalID:     uniqueID,
	}); err != nil {
		log.Printf("registry: caguar device registered but external ref failed thing_id=%s unique_id=%s err=%v", thing.ID, uniqueID, err)
		return nil, err
	}
	log.Printf("registry: caguar device registered thing_id=%s unique_id=%s name=%s tracker_type=%s", thing.ID, uniqueID, name, tracker)
	return deviceResult(thing.ID, name, uniqueID, req.DepartmentID, tracker, plate, chassis), nil
}

// Update changes the device in Caguar and the local thing. thingID is the id returned by register.
func (s *DevicePortal) Update(ctx context.Context, thingID uuid.UUID, req DeviceRegistration) (*DeviceResult, error) {
	if s == nil || s.portal == nil {
		return nil, Unavailable("caguar is not configured")
	}
	if thingID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"id": "id is required"})
	}
	name, uniqueID, tracker, plate, chassis, status, category, disabled, fields := normalizeDevice(req)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	thing, err := s.things.Get(ctx, thingID)
	if err != nil {
		return nil, err
	}
	ref, err := s.owners.FindByThingSystem(ctx, thingID, models.ExternalSystemTraccar)
	if repository.IsNotFound(err) {
		return nil, NotFound("device is not linked to caguar")
	}
	if err != nil {
		return nil, err
	}
	if ref.ExternalID != uniqueID {
		owners, err := s.owners.CountOtherOwners(ctx, models.ExternalSystemTraccar, uniqueID, thingID)
		if err != nil {
			return nil, err
		}
		if owners > 0 {
			return nil, Conflict("external id is already linked to another thing")
		}
	}
	if err := s.portal.UpdateDevice(ctx, ref.ExternalID, caguar.Device{
		Name:          name,
		UniqueID:      uniqueID,
		Status:        status,
		Disabled:      disabled,
		Category:      category,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
		Phone:         strings.TrimSpace(req.Phone),
		Model:         strings.TrimSpace(req.Model),
		Contact:       strings.TrimSpace(req.Contact),
		Product:       caguar.ProductAutomax,
	}); err != nil {
		var remote *caguar.RemoteError
		if errors.As(err, &remote) && remote.Status == http.StatusNotFound {
			return nil, NotFound("device was not found in caguar")
		}
		return nil, portalErr("device", uniqueID, err)
	}

	meta, err := json.Marshal(map[string]string{
		"unique_id":      uniqueID,
		"tracker_type":   tracker,
		"plate_number":   plate,
		"chassis_number": chassis,
	})
	if err != nil {
		return nil, err
	}
	updated, err := s.things.Update(ctx, thingID, models.ThingUpdateRequest{
		Name:         name,
		Type:         thing.Type,
		IsTrackable:  thing.IsTrackable,
		DepartmentID: req.DepartmentID,
		GroupID:      thing.GroupID,
		Metadata:     models.JSON(meta),
	})
	if err != nil {
		log.Printf("registry: caguar device updated but local thing failed thing_id=%s unique_id=%s err=%v", thingID, uniqueID, err)
		return nil, err
	}
	if ref.ExternalID != uniqueID {
		if err := s.owners.UpdateExternalID(ctx, ref.ID, uniqueID, time.Now().UTC()); err != nil {
			log.Printf("registry: caguar device updated but external ref failed thing_id=%s unique_id=%s err=%v", thingID, uniqueID, err)
			return nil, err
		}
	}
	log.Printf("registry: caguar device updated thing_id=%s unique_id=%s name=%s tracker_type=%s", updated.ID, uniqueID, name, tracker)
	return deviceResult(updated.ID, name, uniqueID, req.DepartmentID, tracker, plate, chassis), nil
}

// Attach registers a Caguar device for a unit that does not have an identifier yet.
func (s *DevicePortal) Attach(ctx context.Context, thingID uuid.UUID, req DeviceRegistration) (*DeviceResult, error) {
	if s == nil || s.portal == nil {
		return nil, Unavailable("caguar is not configured")
	}
	if thingID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"id": "id is required"})
	}
	name, uniqueID, tracker, plate, chassis, status, category, disabled, fields := normalizeDevice(req)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	thing, err := s.things.Get(ctx, thingID)
	if err != nil {
		return nil, err
	}
	owners, err := s.owners.CountOtherOwners(ctx, models.ExternalSystemTraccar, uniqueID, thingID)
	if err != nil {
		return nil, err
	}
	if owners > 0 {
		return nil, Conflict("external id is already linked to another thing")
	}
	if err := s.portal.RegisterDevice(ctx, caguar.Device{
		Name:          name,
		UniqueID:      uniqueID,
		Status:        status,
		Disabled:      disabled,
		Category:      category,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
		Phone:         strings.TrimSpace(req.Phone),
		Model:         strings.TrimSpace(req.Model),
		Contact:       strings.TrimSpace(req.Contact),
		ZoneIDs:       []int{},
		Attributes:    map[string]any{},
		Product:       caguar.ProductAutomax,
	}); err != nil {
		return nil, portalErr("device", uniqueID, err)
	}
	meta, err := json.Marshal(map[string]string{
		"unique_id":      uniqueID,
		"tracker_type":   tracker,
		"plate_number":   plate,
		"chassis_number": chassis,
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.things.Update(ctx, thingID, models.ThingUpdateRequest{
		Name:         name,
		Type:         thing.Type,
		IsTrackable:  thing.IsTrackable,
		DepartmentID: req.DepartmentID,
		GroupID:      thing.GroupID,
		Metadata:     models.JSON(meta),
	}); err != nil {
		log.Printf("registry: caguar device registered but local thing failed thing_id=%s unique_id=%s err=%v", thingID, uniqueID, err)
		return nil, err
	}
	if _, _, err := s.refs.Upsert(ctx, thingID, models.ExternalRefRequest{
		ExternalSystem: models.ExternalSystemTraccar,
		ExternalID:     uniqueID,
	}); err != nil {
		log.Printf("registry: caguar device registered but external ref failed thing_id=%s unique_id=%s err=%v", thingID, uniqueID, err)
		return nil, err
	}
	log.Printf("registry: caguar device attached thing_id=%s unique_id=%s name=%s", thingID, uniqueID, name)
	return deviceResult(thingID, name, uniqueID, req.DepartmentID, tracker, plate, chassis), nil
}

// Remove deletes the linked Caguar device. A device Caguar has already dropped is treated as gone.
func (s *DevicePortal) Remove(ctx context.Context, thingID uuid.UUID) error {
	if s == nil || s.portal == nil || thingID == uuid.Nil {
		return nil
	}
	ref, err := s.owners.FindByThingSystem(ctx, thingID, models.ExternalSystemTraccar)
	if repository.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.portal.DeleteDevice(ctx, ref.ExternalID); err != nil {
		var remote *caguar.RemoteError
		if errors.As(err, &remote) && remote.Status == http.StatusNotFound {
			log.Printf("registry: caguar device already gone thing_id=%s unique_id=%s", thingID, ref.ExternalID)
		} else {
			return portalErr("device", ref.ExternalID, err)
		}
	} else {
		log.Printf("registry: caguar device deleted thing_id=%s unique_id=%s", thingID, ref.ExternalID)
	}
	if err := s.owners.Delete(ctx, ref.ID); err != nil && !repository.IsNotFound(err) {
		log.Printf("registry: caguar device deleted but external ref remained thing_id=%s ref_id=%s err=%v", thingID, ref.ID, err)
	}
	return nil
}

func deviceResult(thingID uuid.UUID, name, uniqueID string, departmentID uuid.UUID, tracker, plate, chassis string) *DeviceResult {
	return &DeviceResult{
		ThingID:       thingID,
		Name:          name,
		UniqueID:      uniqueID,
		DepartmentID:  departmentID,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
	}
}

// DriverRegistration is the registry body for one Caguar driver.
type DriverRegistration struct {
	Name          string `json:"name"`
	UniqueID      string `json:"unique_id"`
	UniqueIDCamel string `json:"uniqueId"`
}

// DriverResult is the driver stored in the registry and sent to Caguar.
type DriverResult struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	UniqueID string    `json:"unique_id"`
}

// DriverNameUpdate changes the driver name. The unique id stays as it was stored.
type DriverNameUpdate struct {
	Name string `json:"name"`
}

// DriverPortal stores a driver in the registry and sends the same driver to Caguar.
type DriverPortal struct {
	portal Portal
	store  repository.DriverRepository
}

func NewDriverPortal(portal Portal) *DriverPortal {
	return &DriverPortal{portal: portal}
}

func (s *DriverPortal) UseStore(store repository.DriverRepository) {
	if s == nil {
		return
	}
	s.store = store
}

func (s *DriverPortal) Register(ctx context.Context, req DriverRegistration) (*DriverResult, error) {
	if s == nil {
		return nil, Unavailable("caguar is not configured")
	}
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	uniqueID := strings.TrimSpace(req.UniqueID)
	if uniqueID == "" {
		uniqueID = strings.TrimSpace(req.UniqueIDCamel)
	}
	switch {
	case uniqueID == "":
		fields["unique_id"] = "unique_id is required"
	case len(uniqueID) > 255:
		fields["unique_id"] = "unique_id must be at most 255 characters"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if s.portal == nil && s.store == nil {
		return nil, Unavailable("caguar is not configured")
	}
	body := caguar.Driver{Name: name, UniqueID: uniqueID, Product: caguar.ProductAutomax}
	row := &models.Driver{ID: uuid.New(), Name: name, UniqueID: uniqueID}
	if s.store != nil {
		existing, findErr := s.store.FindByUniqueID(ctx, uniqueID)
		if findErr == nil && existing != nil {
			return nil, Conflict("a driver with this unique id already exists")
		}
		if findErr != nil && !repository.IsNotFound(findErr) {
			return nil, findErr
		}
		if err := s.store.Create(ctx, row); err != nil {
			if repository.IsUniqueViolation(err) {
				return nil, Conflict("a driver with this unique id already exists")
			}
			return nil, err
		}
	}
	if s.portal == nil {
		if s.store == nil {
			return nil, Unavailable("caguar is not configured")
		}
		row.PendingAction = "create"
		if err := s.store.Update(ctx, row); err != nil {
			return nil, err
		}
		log.Printf("registry: driver stored locally; caguar sync queued id=%s unique_id=%s", row.ID, uniqueID)
		return &DriverResult{ID: row.ID, Name: name, UniqueID: uniqueID}, nil
	}
	if err := s.portal.RegisterDriver(ctx, body); err != nil {
		mapped := portalErr("driver", uniqueID, err)
		if s.store != nil && caguarDeferred(mapped) {
			row.PendingAction = "create"
			if saveErr := s.store.Update(ctx, row); saveErr != nil {
				return nil, saveErr
			}
			log.Printf("registry: driver stored locally; caguar sync queued id=%s unique_id=%s", row.ID, uniqueID)
			return &DriverResult{ID: row.ID, Name: name, UniqueID: uniqueID}, nil
		}
		if s.store != nil {
			if delErr := s.store.Delete(ctx, row.ID); delErr != nil {
				log.Printf("registry: driver create rollback failed id=%s err=%v", row.ID, delErr)
			}
		}
		return nil, mapped
	}
	log.Printf("registry: driver registered id=%s unique_id=%s name=%s", row.ID, uniqueID, name)
	return &DriverResult{ID: row.ID, Name: name, UniqueID: uniqueID}, nil
}

// UpdateByUniqueID changes the name of the driver stored under this unique id.
// The unique id is the Automax user id and is not changed.
func (s *DriverPortal) UpdateByUniqueID(ctx context.Context, uniqueID string, req DriverNameUpdate) (*DriverResult, error) {
	if s == nil || s.store == nil {
		return nil, Unavailable("driver store is not configured")
	}
	uniqueID = strings.TrimSpace(uniqueID)
	row, err := s.store.FindByUniqueID(ctx, uniqueID)
	if repository.IsNotFound(err) {
		return nil, NotFound("driver not found")
	}
	if err != nil {
		return nil, err
	}
	return s.Update(ctx, row.ID, req)
}

// Update changes the stored name and the same name in Caguar. The unique id is not changed.
func (s *DriverPortal) Update(ctx context.Context, id uuid.UUID, req DriverNameUpdate) (*DriverResult, error) {
	if s == nil || s.store == nil {
		return nil, Unavailable("driver store is not configured")
	}
	row, err := s.store.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("driver not found")
	}
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if name == row.Name && row.PendingAction == "" {
		return &DriverResult{ID: row.ID, Name: row.Name, UniqueID: row.UniqueID}, nil
	}
	previous := row.Name
	row.Name = name
	body := caguar.Driver{Name: name, UniqueID: row.UniqueID, Product: caguar.ProductAutomax}
	if s.portal == nil {
		row.PendingAction = "update"
		if err := s.store.Update(ctx, row); err != nil {
			return nil, err
		}
		log.Printf("registry: driver name stored locally; caguar sync queued id=%s unique_id=%s", row.ID, row.UniqueID)
		return &DriverResult{ID: row.ID, Name: name, UniqueID: row.UniqueID}, nil
	}
	if err := s.portal.UpdateDriver(ctx, row.UniqueID, body); err != nil {
		mapped := portalErr("driver", row.UniqueID, err)
		if caguarDeferred(mapped) {
			row.PendingAction = "update"
			if saveErr := s.store.Update(ctx, row); saveErr != nil {
				return nil, saveErr
			}
			log.Printf("registry: driver name stored locally; caguar sync queued id=%s unique_id=%s", row.ID, row.UniqueID)
			return &DriverResult{ID: row.ID, Name: name, UniqueID: row.UniqueID}, nil
		}
		log.Printf("registry: driver update rejected by caguar id=%s unique_id=%s previous_name=%s reason=%v", row.ID, row.UniqueID, previous, mapped)
		return nil, mapped
	}
	row.PendingAction = ""
	if err := s.store.Update(ctx, row); err != nil {
		return nil, err
	}
	log.Printf("registry: driver updated id=%s unique_id=%s name=%s", row.ID, row.UniqueID, name)
	return &DriverResult{ID: row.ID, Name: name, UniqueID: row.UniqueID}, nil
}

// DeleteByUniqueID removes the registry driver and the same driver in Caguar.
// The unique id is the Automax user id.
func (s *DriverPortal) DeleteByUniqueID(ctx context.Context, uniqueID string) error {
	if s == nil || s.store == nil {
		return Unavailable("driver store is not configured")
	}
	uniqueID = strings.TrimSpace(uniqueID)
	row, err := s.store.FindByUniqueID(ctx, uniqueID)
	if repository.IsNotFound(err) {
		return NotFound("driver not found")
	}
	if err != nil {
		return err
	}
	if s.portal != nil {
		if err := s.portal.DeleteDriver(ctx, row.UniqueID); err != nil {
			mapped := portalErr("driver", row.UniqueID, err)
			log.Printf("registry: driver delete rejected by caguar id=%s unique_id=%s reason=%v", row.ID, row.UniqueID, mapped)
			return mapped
		}
	}
	if err := s.store.Delete(ctx, row.ID); err != nil {
		return err
	}
	log.Printf("registry: driver deleted id=%s unique_id=%s name=%s", row.ID, row.UniqueID, row.Name)
	return nil
}

// List returns the drivers stored in the registry.
func (s *DriverPortal) List(ctx context.Context) ([]DriverResult, error) {
	if s == nil || s.store == nil {
		return nil, Unavailable("driver store is not configured")
	}
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DriverResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, DriverResult{ID: row.ID, Name: row.Name, UniqueID: row.UniqueID})
	}
	return out, nil
}

func normalizeDevice(req DeviceRegistration) (name, uniqueID, tracker, plate, chassis, status, category string, disabled bool, fields map[string]string) {
	fields = map[string]string{}
	name = validateName(fields, "name", req.Name, 255)
	uniqueID = strings.TrimSpace(req.UniqueID)
	if uniqueID == "" {
		uniqueID = strings.TrimSpace(req.UniqueIDCamel)
	}
	if uniqueID == "" {
		fields["unique_id"] = "unique_id is required"
	} else if len(uniqueID) > 255 {
		fields["unique_id"] = "unique_id must be at most 255 characters"
	}
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	tracker = strings.TrimSpace(req.TrackerType)
	if tracker == "" {
		tracker = strings.TrimSpace(req.TrackerTypeCamel)
	}
	if tracker == "" {
		fields["tracker_type"] = "tracker_type is required"
	} else if len(tracker) > 100 {
		fields["tracker_type"] = "tracker_type must be at most 100 characters"
	}
	plate = strings.ToUpper(strings.TrimSpace(req.PlateNumber))
	if plate == "" {
		plate = strings.ToUpper(strings.TrimSpace(req.PlateCamel))
	}
	if !platePattern.MatchString(plate) {
		fields["plate_number"] = "plate_number must match ABC-1234"
	}
	chassis = strings.TrimSpace(req.ChassisNumber)
	if chassis == "" {
		chassis = strings.TrimSpace(req.ChassisCamel)
	}
	if chassis == "" {
		fields["chassis_number"] = "chassis_number is required"
	} else if len(chassis) > 64 {
		fields["chassis_number"] = "chassis_number must be at most 64 characters"
	}
	// Leave status empty unless the caller set it. Caguar starts a device
	// offline and marks it online only when the identifier is valid.
	status = strings.TrimSpace(req.Status)
	category = strings.TrimSpace(req.Category)
	if category == "" {
		category = "car"
	}
	if req.Disabled != nil {
		disabled = *req.Disabled
	}
	return name, uniqueID, tracker, plate, chassis, status, category, disabled, fields
}

func portalErr(kind, uniqueID string, err error) error {
	var remote *caguar.RemoteError
	if errors.As(err, &remote) {
		log.Printf("registry: caguar %s rejected status=%d unique_id=%s body=%s", kind, remote.Status, uniqueID, remote.Body)
		msg := caguarMessage(remote.Body)
		if msg == "" {
			msg = "caguar rejected the " + kind
		}
		if remote.Status >= 500 {
			return Upstream(msg)
		}
		if fields := caguarDuplicateFields(remote.Body, msg); len(fields) > 0 {
			return Invalid("validation failed", fields)
		}
		if caguarDuplicate(remote.Body, msg) {
			return Conflict(msg)
		}
		return Invalid(msg, nil)
	}
	log.Printf("registry: caguar %s unavailable unique_id=%s err=%v", kind, uniqueID, err)
	return Unavailable("caguar is unavailable")
}

func caguarMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var parsed struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &parsed) == nil {
		if text := strings.TrimSpace(parsed.Error); text != "" {
			return text
		}
		if text := strings.TrimSpace(parsed.Message); text != "" {
			return text
		}
	}
	if strings.HasPrefix(body, "{") {
		return ""
	}
	return body
}

func caguarDuplicateFields(body, msg string) map[string]string {
	text := strings.ToLower(body + " " + msg)
	fields := map[string]string{}
	if strings.Contains(text, "chassis") {
		fields["chassis_number"] = "chassis_number is already used in Caguar"
	}
	if strings.Contains(text, "plate") {
		fields["plate_number"] = "plate_number is already used in Caguar"
	}
	if strings.Contains(text, "unique") || strings.Contains(text, "identifier") {
		fields["unique_id"] = "unique_id is already used in Caguar"
	}
	return fields
}

func caguarDuplicate(body, msg string) bool {
	text := strings.ToLower(body + " " + msg)
	return strings.Contains(text, "duplicate") || strings.Contains(text, "already exists")
}
