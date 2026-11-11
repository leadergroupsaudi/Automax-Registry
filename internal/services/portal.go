package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"regexp"
	"strings"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

var platePattern = regexp.MustCompile(`^[A-Z]{3}-[0-9]{4}$`)

// Portal registers a device or a driver in the Caguar portal.
type Portal interface {
	RegisterDevice(ctx context.Context, device caguar.Device) error
	RegisterDriver(ctx context.Context, driver caguar.Driver) error
}

type thingCreator interface {
	Create(ctx context.Context, req models.ThingCreateRequest) (*models.ThingResponse, error)
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
	}
	portal Portal
}

func NewDevicePortal(things thingCreator, refs refLinker, owners interface {
	CountOtherOwners(ctx context.Context, system, externalID string, thingID uuid.UUID) (int64, error)
}, portal Portal) *DevicePortal {
	return &DevicePortal{things: things, refs: refs, owners: owners, portal: portal}
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
		GroupIDs:      []int{},
		ZoneIDs:       []int{},
		Attributes:    map[string]any{},
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
	return &DeviceResult{
		ThingID:       thing.ID,
		Name:          name,
		UniqueID:      uniqueID,
		DepartmentID:  req.DepartmentID,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
	}, nil
}

// DriverRegistration is the registry body for one Caguar driver.
type DriverRegistration struct {
	Name          string `json:"name"`
	UniqueID      string `json:"unique_id"`
	UniqueIDCamel string `json:"uniqueId"`
}

// DriverResult is the driver accepted by Caguar.
type DriverResult struct {
	Name     string `json:"name"`
	UniqueID string `json:"unique_id"`
}

// DriverPortal sends a driver to Caguar. Both name and unique id are required.
type DriverPortal struct {
	portal Portal
}

func NewDriverPortal(portal Portal) *DriverPortal {
	return &DriverPortal{portal: portal}
}

func (s *DriverPortal) Register(ctx context.Context, req DriverRegistration) (*DriverResult, error) {
	if s == nil || s.portal == nil {
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
	if err := s.portal.RegisterDriver(ctx, caguar.Driver{Name: name, UniqueID: uniqueID}); err != nil {
		return nil, portalErr("driver", uniqueID, err)
	}
	log.Printf("registry: caguar driver registered unique_id=%s name=%s", uniqueID, name)
	return &DriverResult{Name: name, UniqueID: uniqueID}, nil
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

func caguarDuplicate(body, msg string) bool {
	text := strings.ToLower(body + " " + msg)
	return strings.Contains(text, "duplicate") || strings.Contains(text, "already exists")
}
