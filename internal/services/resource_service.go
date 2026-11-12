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

// UseStateCache keeps the DSP-01 status cache aligned with resource create, update, and delete.
func (s *ResourceService) UseStateCache(cache ResourceStateCache) {
	if s == nil {
		return
	}
	s.state = cache
}

func (s *ResourceService) Create(ctx context.Context, req models.ResourceWriteRequest) (*models.ResourceResponse, error) {
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
		s.rollbackThing(ctx, createdThing)
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
