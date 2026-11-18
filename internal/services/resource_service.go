package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

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
	state     ResourceStateCache
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
	return &models.ResourceDriversResponse{DriverUserIDs: ids}, nil
}

// IncidentIDs returns the incident ids recorded for a resource at a service point.
func (s *ResourceService) IncidentIDs(ctx context.Context, req models.ResourceIncidentsRequest) (*models.ResourceIncidentsResponse, error) {
	fields := map[string]string{}
	if req.ResourceID == uuid.Nil {
		fields["resource_id"] = "resource_id is required"
	}
	if req.ServiceID == uuid.Nil {
		fields["service_id"] = "service_id is required"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	ids, err := s.resources.IncidentIDsByResourceAndService(ctx, req.ResourceID, req.ServiceID)
	if err != nil {
		return nil, err
	}
	return &models.ResourceIncidentsResponse{IncidentIDs: ids}, nil
}

func (s *ResourceService) Create(ctx context.Context, req models.ResourceWriteRequest) (*models.ResourceResponse, error) {
	fromCaguar, err := s.registerCaguar(ctx, &req)
	if err != nil {
		return nil, err
	}
	createdThing, err := s.ensureThing(ctx, &req)
	if err != nil {
		return nil, err
	}
	row, err := s.prepare(ctx, uuid.Nil, req)
	if err != nil {
		s.rollbackThing(ctx, createdThing)
		return nil, err
	}
	if err := applyResourceDetails(row, nil, req); err != nil {
		s.rollbackThing(ctx, createdThing)
		return nil, err
	}
	if err := s.resources.Create(ctx, row); err != nil {
		if fromCaguar {
			s.rollbackThing(ctx, req.ThingID)
		} else {
			s.rollbackThing(ctx, createdThing)
		}
		return nil, resourceWriteErr(err, false)
	}
	log.Printf("registry: resource created id=%s thing_id=%s resource_type_id=%s status=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status)
	s.remember(ctx, row)
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
	if _, err := s.ensureThing(ctx, &req); err != nil {
		return nil, err
	}
	if err := s.updateIdentifier(ctx, current, &req); err != nil {
		return nil, err
	}
	row, err := s.prepare(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if err := applyResourceDetails(row, current, req); err != nil {
		return nil, err
	}
	if err := s.resources.Update(ctx, row, current.ThingID); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("resource not found")
		}
		return nil, resourceWriteErr(err, true)
	}
	log.Printf("registry: resource updated id=%s thing_id=%s resource_type_id=%s status=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status)
	s.remember(ctx, row)
	resp := row.Response()
	publishEvent(s.pub, models.SubjectResourceUpdated, row.ID, resp)
	return &resp, nil
}

func (s *ResourceService) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.resources.Delete(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("resource not found")
	}
	var inUse *repository.ResourceInUseError
	if errors.As(err, &inUse) {
		return Conflict(resourceDeleteMessage(inUse))
	}
	if repository.IsForeignKeyViolation(err) {
		return Conflict("Cannot delete this resource: it has shift assignments; it has dispatch assignments")
	}
	if err != nil {
		return err
	}
	log.Printf("registry: resource deleted id=%s", id)
	s.forget(ctx, id)
	publishEvent(s.pub, models.SubjectResourceDeleted, id, nil)
	return nil
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
	}
	if req.ResourceTypeID == uuid.Nil {
		fields["resource_type_id"] = "resource_type_id is required"
	}
	if len(fields) > 0 {
		return uuid.Nil, Invalid("validation failed", fields)
	}
	if utf8.RuneCountInString(name) > 255 {
		return uuid.Nil, Invalid("validation failed", map[string]string{"name": "name must be at most 255 characters"})
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
	if name == "" {
		return Invalid("validation failed", map[string]string{"name": "name is required"})
	}
	if utf8.RuneCountInString(name) > 255 {
		return Invalid("validation failed", map[string]string{"name": "name must be at most 255 characters"})
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
