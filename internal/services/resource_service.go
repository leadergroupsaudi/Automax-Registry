package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

var resourceStatuses = []string{
	models.ResourceAvailable,
	models.ResourceOffered,
	models.ResourceDispatched,
	models.ResourceMaintenance,
}

type ResourceService struct {
	resources repository.ResourceRepository
	things    repository.ThingRepository
	types     repository.ResourceTypeRepository
	points    repository.ServicePointRepository
	devices   *DevicePortal
	locate    *CaguarSync
	outbox    repository.CaguarSyncRepository
	state     ResourceStateCache
	logs      *DispatchLogService
	pub       Publisher
}

func NewResourceService(resources repository.ResourceRepository, things repository.ThingRepository, types repository.ResourceTypeRepository, points repository.ServicePointRepository) *ResourceService {
	return &ResourceService{resources: resources, things: things, types: types, points: points}
}

func resourceWriteErr(err error, missingIsNotFound bool) error {
	switch repository.UniqueConstraint(err) {
	case "uq_resource_unit_code":
		return Conflict("a unit with this unit id already exists")
	case "":
	default:
		return Conflict("a resource already exists for this thing")
	}
	if repository.IsForeignKeyViolation(err) {
		if repository.ConstraintName(err) == "fk_resource_shift" {
			return Invalid("validation failed", map[string]string{"shift_id": "shift not found"})
		}
		return Invalid("validation failed", map[string]string{"resource_type_id": "resource type not found"})
	}
	if repository.IsNotFound(err) {
		if missingIsNotFound {
			return NotFound("resource not found")
		}
		return Invalid("validation failed", map[string]string{"thing_id": "thing_id does not match a registry thing"})
	}
	return err
}

func (s *ResourceService) UseDispatchLog(logs *DispatchLogService) {
	if s == nil {
		return
	}
	s.logs = logs
}

func (s *ResourceService) UsePublisher(pub Publisher) {
	if s == nil {
		return
	}
	s.pub = pub
}

// UseDevicePortal registers a new unit in Caguar when the form does not already name a thing.
func (s *ResourceService) UseDevicePortal(portal *DevicePortal) {
	if s == nil {
		return
	}
	s.devices = portal
}

// UseCaguarSync reads the latest Caguar fix before a unit is returned.
func (s *ResourceService) UseCaguarSync(sync *CaguarSync) {
	if s == nil {
		return
	}
	s.locate = sync
}

// UseCaguarOutbox keeps device creates, edits, and deletes that Caguar has not confirmed.
func (s *ResourceService) UseCaguarOutbox(outbox repository.CaguarSyncRepository) {
	if s == nil {
		return
	}
	s.outbox = outbox
}

// refreshLocations gives Caguar a short chance to update coordinates.
// A slow portal must not hold the unit list open until the client gives up.
func (s *ResourceService) refreshLocations(ctx context.Context) {
	if s == nil || s.locate == nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s.locate.Refresh(syncCtx)
}

// UseStateCache keeps the DSP-01 status cache aligned with resource create, update, and delete.
func (s *ResourceService) UseStateCache(cache ResourceStateCache) {
	if s == nil {
		return
	}
	s.state = cache
}

// DriverUserIDs returns the driver user ids of the resources at the given service point.
func (s *ResourceService) DriverUserIDs(ctx context.Context, req models.ResourceDriversRequest) (*models.ResourceDriversResponse, error) {
	fields := map[string]string{}
	if req.ServiceID == uuid.Nil {
		fields["service_id"] = "service_id is required"
	}
	if req.IncidentID == uuid.Nil {
		fields["incident_id"] = "incident_id is required"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	drivers, err := s.resources.DriversByServicePoint(ctx, req.ServiceID)
	if err != nil {
		return nil, err
	}
	resourceIDs := make([]uuid.UUID, 0, len(drivers))
	ids := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, d := range drivers {
		resourceIDs = append(resourceIDs, d.ResourceID)
		if !seen[d.DriverUserID] {
			seen[d.DriverUserID] = true
			ids = append(ids, d.DriverUserID)
		}
	}
	if err := s.resources.SaveIncidentMapping(ctx, req.ServiceID, req.IncidentID, resourceIDs); err != nil {
		return nil, err
	}
	if err := s.logIncidentStation(ctx, req.IncidentID, req.ServiceID); err != nil {
		log.Printf("registry: incident station log failed incident_id=%s service_id=%s err=%v", req.IncidentID, req.ServiceID, err)
		return nil, err
	}
	return &models.ResourceDriversResponse{DriverUserIDs: ids}, nil
}

func (s *ResourceService) logIncidentStation(ctx context.Context, incidentID, stationID uuid.UUID) error {
	if s == nil || s.logs == nil || incidentID == uuid.Nil || stationID == uuid.Nil {
		return nil
	}
	rows, err := s.logs.List(ctx, models.DispatchLogFilter{IncidentID: &incidentID, FireStationID: &stationID})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Action == models.DispatchLogIncidentAssigned {
			return nil
		}
	}
	name, err := s.stationName(ctx, stationID)
	if repository.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.logs.RecordIncidentStation(ctx, incidentID, stationID, name)
}

// IncidentIDs returns the incident ids recorded for a resource at a service point.
func (s *ResourceService) IncidentIDs(ctx context.Context, req models.ResourceIncidentsRequest) (*models.ResourceIncidentsResponse, error) {
	fields := map[string]string{}
	if req.ResourceID == uuid.Nil {
		fields["resource_id"] = "resource_id is required"
	}
	// if req.ServiceID == uuid.Nil {
	// 	fields["service_id"] = "service_id is required"
	// }
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	ids, err := s.resources.IncidentIDsByResourceAndService(ctx, req.ResourceID)
	if err != nil {
		return nil, err
	}
	return &models.ResourceIncidentsResponse{IncidentIDs: ids}, nil
}

func (s *ResourceService) Create(ctx context.Context, req models.ResourceWriteRequest) (*models.ResourceResponse, error) {
	if err := validateUnit(&req); err != nil {
		log.Printf("registry: unit create rejected before save reason=%v", err)
		return nil, err
	}
	createdThing, err := s.ensureThing(ctx, &req)
	if err != nil {
		log.Printf("registry: unit create rejected before save reason=%v", err)
		return nil, err
	}
	row, err := s.prepare(ctx, uuid.Nil, req)
	if err != nil {
		s.rollbackThing(ctx, createdThing)
		log.Printf("registry: unit create rejected thing_id=%s reason=%v", req.ThingID, err)
		return nil, err
	}
	if err := applyResourceDetails(row, nil, req); err != nil {
		s.rollbackThing(ctx, createdThing)
		log.Printf("registry: unit create rejected thing_id=%s reason=%v", req.ThingID, err)
		return nil, err
	}
	if err := s.resources.Create(ctx, row); err != nil {
		s.rollbackThing(ctx, createdThing)
		log.Printf("registry: unit create failed thing_id=%s err=%v", req.ThingID, err)
		return nil, resourceWriteErr(err, false)
	}
	if err := s.storeDeviceMeta(ctx, row.ThingID, &req); err != nil {
		log.Printf("registry: unit device details not stored thing_id=%s err=%v", row.ThingID, err)
	}
	pending, err := s.pushCreate(ctx, row, &req)
	if err != nil {
		if delErr := s.resources.Delete(ctx, row.ID); delErr != nil {
			log.Printf("registry: unit create rollback failed id=%s err=%v", row.ID, delErr)
		}
		s.rollbackThing(ctx, createdThing)
		log.Printf("registry: unit create rejected by caguar id=%s thing_id=%s unique_id=%s reason=%v", row.ID, row.ThingID, strings.TrimSpace(req.UniqueID), err)
		return nil, err
	}
	if strings.TrimSpace(req.UniqueID) != "" {
		row.UniqueID = strings.TrimSpace(req.UniqueID)
	}
	if pending {
		log.Printf("registry: unit created locally; caguar sync queued id=%s thing_id=%s unique_id=%s", row.ID, row.ThingID, row.UniqueID)
	} else {
		log.Printf("registry: resource created id=%s thing_id=%s resource_type_id=%s status=%s unique_id=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status, row.UniqueID)
	}
	s.remember(ctx, row)
	if err := s.logHomeStation(ctx, row.ID, nil, row.ServicePointID); err != nil {
		log.Printf("registry: unit station log failed id=%s err=%v", row.ID, err)
		return nil, err
	}
	resp := row.Response()
	publishEvent(s.pub, models.SubjectResourceCreated, row.ID, resp)
	return &resp, nil
}

func (s *ResourceService) Update(ctx context.Context, id uuid.UUID, req models.ResourceWriteRequest) (*models.ResourceResponse, error) {
	current, err := s.resources.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource not found")
	}
	if err != nil {
		return nil, err
	}
	if err := validateUnit(&req); err != nil {
		log.Printf("registry: unit update rejected id=%s reason=%v", id, err)
		return nil, err
	}
	previousName := ""
	if req.ThingID != uuid.Nil && req.Name != nil {
		if thing, findErr := s.things.FindByID(ctx, req.ThingID); findErr == nil {
			previousName = thing.Name
		}
	}
	if _, err := s.ensureThing(ctx, &req); err != nil {
		log.Printf("registry: unit update rejected id=%s reason=%v", id, err)
		return nil, err
	}
	row, err := s.prepare(ctx, id, req)
	if err != nil {
		log.Printf("registry: unit update rejected id=%s reason=%v", id, err)
		return nil, err
	}
	if err := applyResourceDetails(row, current, req); err != nil {
		log.Printf("registry: unit update rejected id=%s reason=%v", id, err)
		return nil, err
	}
	pending, err := s.pushUpdate(ctx, current, row, &req)
	if err != nil {
		if previousName != "" && req.ThingID != uuid.Nil {
			if undoErr := s.renameThing(ctx, req.ThingID, previousName); undoErr != nil {
				log.Printf("registry: unit name restore failed id=%s thing_id=%s err=%v", id, req.ThingID, undoErr)
			}
		}
		log.Printf("registry: unit update rejected by caguar id=%s thing_id=%s unique_id=%s reason=%v", id, row.ThingID, strings.TrimSpace(req.UniqueID), err)
		return nil, err
	}
	if err := s.resources.Update(ctx, row, current.ThingID); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("resource not found")
		}
		log.Printf("registry: unit update failed id=%s err=%v", id, err)
		return nil, resourceWriteErr(err, true)
	}
	if err := s.storeDeviceMeta(ctx, row.ThingID, &req); err != nil {
		log.Printf("registry: unit device details not stored thing_id=%s err=%v", row.ThingID, err)
	}
	if strings.TrimSpace(req.UniqueID) != "" {
		row.UniqueID = strings.TrimSpace(req.UniqueID)
	} else {
		row.UniqueID = current.UniqueID
	}
	if pending {
		log.Printf("registry: unit updated locally; caguar sync queued id=%s thing_id=%s unique_id=%s", row.ID, row.ThingID, row.UniqueID)
	} else {
		log.Printf("registry: resource updated id=%s thing_id=%s resource_type_id=%s status=%s unique_id=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status, row.UniqueID)
	}
	s.remember(ctx, row)
	if err := s.logHomeStation(ctx, row.ID, current.ServicePointID, row.ServicePointID); err != nil {
		log.Printf("registry: unit station log failed id=%s err=%v", row.ID, err)
		return nil, err
	}
	resp := row.Response()
	publishEvent(s.pub, models.SubjectResourceUpdated, row.ID, resp)
	return &resp, nil
}

func (s *ResourceService) Delete(ctx context.Context, id uuid.UUID) error {
	current, err := s.resources.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("resource not found")
	}
	if err != nil {
		return err
	}
	err = s.resources.Delete(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("resource not found")
	}
	var inUse *repository.ResourceInUseError
	if errors.As(err, &inUse) {
		log.Printf("registry: unit delete rejected id=%s reason=%s", id, resourceDeleteMessage(inUse))
		return Conflict(resourceDeleteMessage(inUse))
	}
	if repository.IsForeignKeyViolation(err) {
		log.Printf("registry: unit delete rejected id=%s reason=resource is in use", id)
		return Conflict("Cannot delete this resource: it has shift assignments; it has dispatch assignments")
	}
	if err != nil {
		log.Printf("registry: unit delete failed id=%s err=%v", id, err)
		return err
	}
	if pending, pushErr := s.pushDelete(ctx, current); pushErr != nil {
		log.Printf("registry: unit deleted locally; caguar delete failed id=%s thing_id=%s unique_id=%s err=%v", id, current.ThingID, current.UniqueID, pushErr)
	} else if pending {
		log.Printf("registry: unit deleted locally; caguar delete queued id=%s thing_id=%s unique_id=%s", id, current.ThingID, current.UniqueID)
	} else {
		log.Printf("registry: resource deleted id=%s thing_id=%s unique_id=%s", id, current.ThingID, current.UniqueID)
	}
	s.forget(ctx, id)
	publishEvent(s.pub, models.SubjectResourceDeleted, id, nil)
	return nil
}

func (s *ResourceService) logHomeStation(ctx context.Context, resourceID uuid.UUID, from, to *uuid.UUID) error {
	if s == nil || s.logs == nil || sameStation(from, to) {
		return nil
	}
	if from != nil && *from != uuid.Nil {
		name, err := s.stationName(ctx, *from)
		if err != nil {
			return err
		}
		if err := s.logs.RecordUnitStation(ctx, resourceID, *from, name, false); err != nil {
			return err
		}
	}
	if to != nil && *to != uuid.Nil {
		name, err := s.stationName(ctx, *to)
		if err != nil {
			return err
		}
		if err := s.logs.RecordUnitStation(ctx, resourceID, *to, name, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *ResourceService) stationName(ctx context.Context, id uuid.UUID) (string, error) {
	row, err := s.points.FindByID(ctx, id)
	if err != nil {
		return "", err
	}
	return row.Name, nil
}

func sameStation(a, b *uuid.UUID) bool {
	aNil := a == nil || *a == uuid.Nil
	bNil := b == nil || *b == uuid.Nil
	if aNil || bNil {
		return aNil && bNil
	}
	return *a == *b
}

func (s *ResourceService) remember(ctx context.Context, row *models.Resource) {
	if s == nil || s.state == nil || row == nil {
		return
	}
	if err := s.state.RememberResource(ctx, row.ID, row.Status, row.ResourceTypeID); err != nil {
		log.Printf("registry: resource state cache refresh failed resource_id=%s err=%v", row.ID, err)
	}
}

func (s *ResourceService) forget(ctx context.Context, id uuid.UUID) {
	if s == nil || s.state == nil || id == uuid.Nil {
		return
	}
	if err := s.state.ForgetResource(ctx, id); err != nil {
		log.Printf("registry: resource geo forget failed resource_id=%s err=%v", id, err)
	}
}

// SetIncident attaches the unit to an incident, or detaches it when incidentID is nil.
func (s *ResourceService) SetIncident(ctx context.Context, id uuid.UUID, incidentID *uuid.UUID) (*models.ResourceResponse, error) {
	if incidentID != nil && *incidentID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"incident_id": "incident_id must be a UUID"})
	}
	err := s.resources.SetIncident(ctx, id, incidentID)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource not found")
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *ResourceService) Get(ctx context.Context, id uuid.UUID) (*models.ResourceResponse, error) {
	s.refreshLocations(ctx)
	row, err := s.resources.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *ResourceService) GetByUniqueID(ctx context.Context, uniqueID string) (*models.ResourceResponse, error) {
	uniqueID = strings.TrimSpace(uniqueID)
	if uniqueID == "" {
		return nil, Invalid("validation failed", map[string]string{"unique_id": "unique_id is required"})
	}
	s.refreshLocations(ctx)
	row, err := s.resources.FindByUniqueID(ctx, uniqueID)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *ResourceService) List(ctx context.Context, filter models.ResourceListFilter) ([]models.ResourceResponse, int64, error) {
	s.refreshLocations(ctx)
	if filter.Status != "" {
		fields := map[string]string{}
		filter.Status = choice(fields, "status", filter.Status, "", resourceStatuses...)
		if len(fields) > 0 {
			return nil, 0, Invalid("validation failed", fields)
		}
	}
	rows, total, err := s.resources.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.ResourceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, total, nil
}

func (s *ResourceService) prepare(ctx context.Context, id uuid.UUID, req models.ResourceWriteRequest) (*models.Resource, error) {
	fields := map[string]string{}
	if req.ThingID == uuid.Nil {
		fields["thing_id"] = "thing_id is required"
	}
	if req.ResourceTypeID == uuid.Nil {
		fields["resource_type_id"] = "resource_type_id is required"
	}
	status := choice(fields, "status", req.Status, models.ResourceAvailable, resourceStatuses...)
	if req.ServicePointID != nil && *req.ServicePointID == uuid.Nil {
		fields["service_point_id"] = "service_point_id must be a UUID"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	ok, err := s.things.Exists(ctx, req.ThingID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, Invalid("validation failed", map[string]string{"thing_id": "thing_id does not match a registry thing"})
	}
	ok, err = s.types.Exists(ctx, req.ResourceTypeID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, Invalid("validation failed", map[string]string{"resource_type_id": "resource type not found"})
	}
	if req.ServicePointID != nil {
		ok, err = s.points.Exists(ctx, *req.ServicePointID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, Invalid("validation failed", map[string]string{"service_point_id": "service point not found"})
		}
	}
	return &models.Resource{
		ID:             id,
		ThingID:        req.ThingID,
		ResourceTypeID: req.ResourceTypeID,
		ServicePointID: req.ServicePointID,
		Status:         status,
		Classification: models.JSON(`[]`),
		IsActive:       true,
	}, nil
}

// updateIdentifier changes the tracker identifier on an existing unit.
// An empty value leaves the stored identifier unchanged.
func (s *ResourceService) updateIdentifier(ctx context.Context, current *models.Resource, req *models.ResourceWriteRequest) error {
	if s == nil || s.devices == nil || !s.devices.Enabled() || current == nil || req == nil {
		return nil
	}
	next := strings.TrimSpace(req.UniqueID)
	if next == "" || next == strings.TrimSpace(current.UniqueID) {
		return nil
	}
	thing, err := s.things.FindByID(ctx, current.ThingID)
	if err != nil {
		return err
	}
	meta := map[string]string{}
	if len(thing.Metadata) > 0 {
		_ = json.Unmarshal(thing.Metadata, &meta)
	}
	tracker := strings.TrimSpace(req.TrackerType)
	if tracker == "" {
		tracker = strings.TrimSpace(meta["tracker_type"])
	}
	if tracker == "" {
		tracker = "Vehicle"
	}
	chassis := strings.TrimSpace(req.ChassisNumber)
	if chassis == "" {
		chassis = strings.TrimSpace(meta["chassis_number"])
	}
	plate := current.PlateNumber
	if req.PlateNumber != nil && strings.TrimSpace(*req.PlateNumber) != "" {
		plate = strings.TrimSpace(*req.PlateNumber)
	}
	name := current.Name
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		name = strings.TrimSpace(*req.Name)
	}
	dept := req.DepartmentID
	if dept == uuid.Nil {
		dept = thing.DepartmentID
	}
	registration := DeviceRegistration{
		Name:          name,
		UniqueID:      next,
		DepartmentID:  dept,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: chassis,
	}
	if strings.TrimSpace(current.UniqueID) == "" {
		_, err = s.devices.Attach(ctx, current.ThingID, registration)
	} else {
		_, err = s.devices.Update(ctx, current.ThingID, registration)
	}
	if err != nil {
		return err
	}
	current.UniqueID = next
	log.Printf("registry: unit identifier updated thing_id=%s unique_id=%s", current.ThingID, next)
	return nil
}

func validateUnit(req *models.ResourceWriteRequest) error {
	fields := map[string]string{}
	if req == nil || req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		fields["name"] = "name is required"
	}
	unique := ""
	plate := ""
	chassis := ""
	if req != nil {
		unique = strings.TrimSpace(req.UniqueID)
		if req.PlateNumber != nil {
			plate = strings.ToUpper(strings.TrimSpace(*req.PlateNumber))
		}
		chassis = strings.TrimSpace(req.ChassisNumber)
	}
	if unique == "" {
		fields["unique_id"] = "unique_id is required"
	} else if len(unique) > 255 {
		fields["unique_id"] = "unique_id must be at most 255 characters"
	}
	if plate == "" {
		fields["plate_number"] = "plate_number is required"
	}
	if chassis == "" {
		fields["chassis_number"] = "chassis_number is required"
	} else if len(chassis) > 64 {
		fields["chassis_number"] = "chassis_number must be at most 64 characters"
	}
	if len(fields) > 0 {
		return Invalid("validation failed", fields)
	}
	return nil
}

func caguarDeferred(err error) bool {
	var se *Error
	return errors.As(err, &se) && (se.Kind == KindUnavailable || se.Kind == KindUpstream)
}

// caguarRestricted means Caguar will not change a device it marked as created by Automax.
func caguarDeviceUnchanged(current *models.Resource, reg DeviceRegistration) bool {
	if current == nil {
		return false
	}
	same := func(a, b string) bool {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	if !same(current.Name, reg.Name) || !same(current.PlateNumber, reg.PlateNumber) || !same(current.UniqueID, reg.UniqueID) {
		return false
	}
	if strings.TrimSpace(current.ChassisNumber) != "" && !same(current.ChassisNumber, reg.ChassisNumber) {
		return false
	}
	if strings.TrimSpace(current.TrackerType) != "" && !same(current.TrackerType, reg.TrackerType) {
		return false
	}
	return true
}

func caguarRestricted(err error) bool {
	text := ""
	var se *Error
	if errors.As(err, &se) {
		text = se.Message
	}
	var remote *caguar.RemoteError
	if errors.As(err, &remote) {
		text += " " + remote.Body
	}
	text = strings.ToLower(text)
	return strings.Contains(text, "restricted_product") || strings.Contains(text, "automax device")
}

func (s *ResourceService) registration(ctx context.Context, thingID uuid.UUID, req *models.ResourceWriteRequest) DeviceRegistration {
	name := ""
	if req != nil && req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	plate := ""
	if req != nil && req.PlateNumber != nil {
		plate = *req.PlateNumber
	}
	dept := uuid.Nil
	if req != nil {
		dept = req.DepartmentID
	}
	tracker := ""
	chassis := ""
	unique := ""
	if req != nil {
		tracker = strings.TrimSpace(req.TrackerType)
		chassis = strings.TrimSpace(req.ChassisNumber)
		unique = strings.TrimSpace(req.UniqueID)
	}
	if thing, err := s.things.FindByID(ctx, thingID); err == nil {
		if dept == uuid.Nil {
			dept = thing.DepartmentID
		}
		if name == "" {
			name = thing.Name
		}
		meta := map[string]string{}
		if len(thing.Metadata) > 0 {
			_ = json.Unmarshal(thing.Metadata, &meta)
		}
		if chassis == "" {
			chassis = strings.TrimSpace(meta["chassis_number"])
		}
		if tracker == "" {
			tracker = strings.TrimSpace(meta["tracker_type"])
		}
	}
	if tracker == "" {
		tracker = "Vehicle"
	}
	return DeviceRegistration{
		Name:            name,
		UniqueID:        unique,
		DepartmentID:    dept,
		TrackerType:     tracker,
		PlateNumber:     plate,
		ChassisNumber:   chassis,
		SkipPlateFormat: true,
	}
}

func (s *ResourceService) storeDeviceMeta(ctx context.Context, thingID uuid.UUID, req *models.ResourceWriteRequest) error {
	if s == nil || thingID == uuid.Nil || req == nil || strings.TrimSpace(req.UniqueID) == "" {
		return nil
	}
	thing, err := s.things.FindByID(ctx, thingID)
	if err != nil {
		return err
	}
	meta := map[string]string{}
	if len(thing.Metadata) > 0 {
		_ = json.Unmarshal(thing.Metadata, &meta)
	}
	meta["unique_id"] = strings.TrimSpace(req.UniqueID)
	tracker := strings.TrimSpace(req.TrackerType)
	if tracker == "" {
		tracker = "Vehicle"
	}
	meta["tracker_type"] = tracker
	if req.PlateNumber != nil {
		meta["plate_number"] = strings.ToUpper(strings.TrimSpace(*req.PlateNumber))
	}
	meta["chassis_number"] = strings.TrimSpace(req.ChassisNumber)
	meta["product"] = caguar.ProductAutomax
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	thing.Metadata = models.JSON(raw)
	return s.things.Update(ctx, thing)
}

func (s *ResourceService) enqueueDevice(ctx context.Context, action string, thingID uuid.UUID, reg DeviceRegistration) {
	if s == nil || s.outbox == nil || thingID == uuid.Nil {
		log.Printf("registry: caguar sync not queued action=%s thing_id=%s unique_id=%s", action, thingID, reg.UniqueID)
		return
	}
	raw, err := json.Marshal(reg)
	if err != nil {
		log.Printf("registry: caguar sync payload failed action=%s thing_id=%s err=%v", action, thingID, err)
		return
	}
	if err := s.outbox.Enqueue(ctx, &models.CaguarSyncJob{
		ThingID:  thingID,
		Action:   action,
		UniqueID: reg.UniqueID,
		Payload:  models.JSON(raw),
	}); err != nil {
		log.Printf("registry: caguar sync queue failed action=%s thing_id=%s unique_id=%s err=%v", action, thingID, reg.UniqueID, err)
	}
}

func (s *ResourceService) pushCreate(ctx context.Context, row *models.Resource, req *models.ResourceWriteRequest) (bool, error) {
	if s == nil || s.devices == nil || !s.devices.Enabled() || req == nil || strings.TrimSpace(req.UniqueID) == "" {
		return false, nil
	}
	reg := s.registration(ctx, row.ThingID, req)
	if _, err := s.devices.Attach(ctx, row.ThingID, reg); err != nil {
		if caguarDeferred(err) {
			s.enqueueDevice(ctx, models.CaguarSyncCreate, row.ThingID, reg)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (s *ResourceService) pushUpdate(ctx context.Context, current, row *models.Resource, req *models.ResourceWriteRequest) (bool, error) {
	if s == nil || s.devices == nil || !s.devices.Enabled() || current == nil || row == nil {
		return false, nil
	}
	reg := s.registration(ctx, row.ThingID, req)
	if reg.UniqueID == "" {
		reg.UniqueID = strings.TrimSpace(current.UniqueID)
	}
	if reg.Name == "" {
		reg.Name = row.Name
	}
	if strings.TrimSpace(reg.PlateNumber) == "" {
		reg.PlateNumber = row.PlateNumber
	}
	reg.ExistingPlate = current.PlateNumber
	if reg.UniqueID == "" {
		return false, nil
	}
	var err error
	if strings.TrimSpace(current.UniqueID) == "" {
		_, err = s.devices.Attach(ctx, row.ThingID, reg)
	} else {
		_, err = s.devices.Update(ctx, row.ThingID, reg)
		var se *Error
		if errors.As(err, &se) && se.Kind == KindNotFound {
			_, err = s.devices.Attach(ctx, row.ThingID, reg)
		}
	}
	if err == nil {
		return false, nil
	}
	if caguarRestricted(err) && caguarDeviceUnchanged(current, reg) {
		log.Printf("registry: caguar left the automax device unchanged; other unit fields saved thing_id=%s unique_id=%s", row.ThingID, reg.UniqueID)
		return false, nil
	}
	if caguarDeferred(err) {
		action := models.CaguarSyncUpdate
		if strings.TrimSpace(current.UniqueID) == "" {
			action = models.CaguarSyncCreate
		}
		s.enqueueDevice(ctx, action, row.ThingID, reg)
		return true, nil
	}
	return false, err
}

func (s *ResourceService) pushDelete(ctx context.Context, current *models.Resource) (bool, error) {
	if s == nil || current == nil {
		return false, nil
	}
	pendingCreate := s.dropPendingEdits(ctx, current.ThingID)
	if s.devices == nil || !s.devices.Enabled() {
		return false, nil
	}
	if pendingCreate {
		log.Printf("registry: unit deleted before caguar create thing_id=%s unique_id=%s", current.ThingID, current.UniqueID)
		return false, nil
	}
	if err := s.devices.Remove(ctx, current.ThingID); err != nil {
		if caguarRestricted(err) {
			log.Printf("registry: caguar kept the automax device thing_id=%s unique_id=%s", current.ThingID, current.UniqueID)
			return false, nil
		}
		reg := DeviceRegistration{Name: current.Name, UniqueID: current.UniqueID, PlateNumber: current.PlateNumber, TrackerType: "Vehicle"}
		s.enqueueDevice(ctx, models.CaguarSyncDelete, current.ThingID, reg)
		return true, nil
	}
	return false, nil
}

func (s *ResourceService) dropPendingEdits(ctx context.Context, thingID uuid.UUID) bool {
	if s == nil || s.outbox == nil || thingID == uuid.Nil {
		return false
	}
	jobs, err := s.outbox.List(ctx)
	if err != nil {
		log.Printf("registry: caguar sync list failed thing_id=%s err=%v", thingID, err)
		return false
	}
	pendingCreate := false
	for _, job := range jobs {
		if job.ThingID != thingID || job.Action == models.CaguarSyncDelete {
			continue
		}
		if job.Action == models.CaguarSyncCreate {
			pendingCreate = true
		}
		if err := s.outbox.Delete(ctx, job.ID); err != nil {
			log.Printf("registry: caguar sync cancel failed id=%s err=%v", job.ID, err)
		}
	}
	return pendingCreate
}

// registerCaguar sends a new unit to Caguar and keeps the returned thing id.
// A unit that already has a thing id is left unchanged.
func (s *ResourceService) registerCaguar(ctx context.Context, req *models.ResourceWriteRequest) (bool, error) {
	if s == nil || s.devices == nil || !s.devices.Enabled() || req.ThingID != uuid.Nil {
		return false, nil
	}
	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	plate := ""
	if req.PlateNumber != nil {
		plate = *req.PlateNumber
	}
	dept := req.DepartmentID
	if dept == uuid.Nil && req.ResourceTypeID != uuid.Nil {
		typ, err := s.types.FindByID(ctx, req.ResourceTypeID)
		if err == nil {
			dept = typ.DepartmentID
		}
	}
	tracker := strings.TrimSpace(req.TrackerType)
	if tracker == "" {
		tracker = "Vehicle"
	}
	result, err := s.devices.Register(ctx, DeviceRegistration{
		Name:          name,
		UniqueID:      strings.TrimSpace(req.UniqueID),
		DepartmentID:  dept,
		TrackerType:   tracker,
		PlateNumber:   plate,
		ChassisNumber: strings.TrimSpace(req.ChassisNumber),
	})
	if err != nil {
		return false, err
	}
	req.ThingID = result.ThingID
	storedPlate := result.PlateNumber
	req.PlateNumber = &storedPlate
	log.Printf("registry: unit registered in caguar thing_id=%s unique_id=%s", result.ThingID, result.UniqueID)
	return true, nil
}

// ensureThing links an existing thing, renames it, or creates a vehicle thing
// when the unit form sends a name and no thing_id. created is set only when
// this call inserted the thing, so the caller can remove it if the resource write fails.
func (s *ResourceService) ensureThing(ctx context.Context, req *models.ResourceWriteRequest) (created uuid.UUID, err error) {
	if req.ThingID != uuid.Nil {
		if req.Name != nil {
			if err := s.renameThing(ctx, req.ThingID, *req.Name); err != nil {
				return uuid.Nil, err
			}
		}
		return uuid.Nil, nil
	}
	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	fields := map[string]string{}
	if name == "" {
		fields["thing_id"] = "thing_id is required"
		fields["name"] = "name is required when thing_id is empty"
	} else if utf8.RuneCountInString(name) > 255 {
		fields["name"] = "name must be at most 255 characters"
	} else {
		rejectSpecialName(fields, "name", name)
	}
	if req.ResourceTypeID == uuid.Nil {
		fields["resource_type_id"] = "resource_type_id is required"
	}
	if len(fields) > 0 {
		return uuid.Nil, Invalid("validation failed", fields)
	}
	typ, err := s.types.FindByID(ctx, req.ResourceTypeID)
	if repository.IsNotFound(err) {
		return uuid.Nil, Invalid("validation failed", map[string]string{"resource_type_id": "resource type not found"})
	}
	if err != nil {
		return uuid.Nil, err
	}
	thing := &models.Thing{
		Name:         name,
		Type:         "vehicle",
		IsTrackable:  true,
		DepartmentID: typ.DepartmentID,
		Metadata:     models.JSON(`{}`),
	}
	if err := s.things.Create(ctx, thing); err != nil {
		if repository.IsUniqueViolation(err) {
			return uuid.Nil, Conflict("a thing with this name already exists")
		}
		return uuid.Nil, err
	}
	s.publishThing(*thing, models.SubjectThingCreated)
	req.ThingID = thing.ID
	return thing.ID, nil
}

func (s *ResourceService) renameThing(ctx context.Context, id uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	fields := map[string]string{}
	if name == "" {
		fields["name"] = "name is required"
	} else if utf8.RuneCountInString(name) > 255 {
		fields["name"] = "name must be at most 255 characters"
	} else {
		rejectSpecialName(fields, "name", name)
	}
	if len(fields) > 0 {
		return Invalid("validation failed", fields)
	}
	thing, err := s.things.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return Invalid("validation failed", map[string]string{"thing_id": "thing_id does not match a registry thing"})
	}
	if err != nil {
		return err
	}
	if thing.Name == name {
		return nil
	}
	thing.Name = name
	if err := s.things.Update(ctx, thing); err != nil {
		if repository.IsUniqueViolation(err) {
			return Conflict("a thing with this name already exists")
		}
		return err
	}
	s.publishThing(*thing, models.SubjectThingUpdated)
	return nil
}

func (s *ResourceService) rollbackThing(ctx context.Context, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	if err := s.things.Delete(ctx, id); err != nil {
		log.Printf("registry: failed to roll back thing %s: %v", id, err)
		return
	}
	s.publishThing(models.Thing{ID: id}, models.SubjectThingDeleted)
}

func (s *ResourceService) publishThing(thing models.Thing, subject string) {
	if s == nil || s.pub == nil {
		return
	}
	body, err := json.Marshal(thing.Event(subject, time.Now().UTC()))
	if err != nil {
		log.Printf("registry: failed to encode %s for thing %s: %v", subject, thing.ID, err)
		return
	}
	if err := s.pub.Publish(subject, body); err != nil {
		log.Printf("registry: failed to publish %s for thing %s: %v", subject, thing.ID, err)
	}
}

func resourceDeleteMessage(e *repository.ResourceInUseError) string {
	var parts []string
	if e.Assignments > 0 {
		parts = append(parts, "it has shift assignments")
	}
	if e.Dispatches > 0 {
		parts = append(parts, "it has dispatch assignments")
	}
	if len(parts) == 0 {
		return "Cannot delete this resource: it is in use"
	}
	return "Cannot delete this resource: " + joinParts(parts)
}

func joinParts(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + "; " + parts[1]
}
