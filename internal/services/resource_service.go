package services

import (
	"context"
	"errors"
	"log"

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
}

func NewResourceService(resources repository.ResourceRepository, things repository.ThingRepository, types repository.ResourceTypeRepository, points repository.ServicePointRepository) *ResourceService {
	return &ResourceService{resources: resources, things: things, types: types, points: points}
}

// UseStateCache keeps the DSP-01 status cache aligned with resource create, update, and delete.
func (s *ResourceService) UseStateCache(cache ResourceStateCache) {
	if s == nil {
		return
	}
	s.state = cache
}

func (s *ResourceService) Create(ctx context.Context, req models.ResourceWriteRequest) (*models.ResourceResponse, error) {
	row, err := s.prepare(ctx, uuid.Nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.resources.Create(ctx, row); err != nil {
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a resource already exists for this thing")
		}
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_type_id": "resource type not found"})
		}
		if repository.IsNotFound(err) {
			return nil, Invalid("validation failed", map[string]string{"thing_id": "thing_id does not match a registry thing"})
		}
		return nil, err
	}
	log.Printf("registry: resource created id=%s thing_id=%s resource_type_id=%s status=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status)
	s.remember(ctx, row)
	resp := row.Response()
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
	row, err := s.prepare(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if err := s.resources.Update(ctx, row, current.ThingID); err != nil {
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a resource already exists for this thing")
		}
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_type_id": "resource type not found"})
		}
		if repository.IsNotFound(err) {
			return nil, NotFound("resource not found")
		}
		return nil, err
	}
	log.Printf("registry: resource updated id=%s thing_id=%s resource_type_id=%s status=%s", row.ID, row.ThingID, row.ResourceTypeID, row.Status)
	s.remember(ctx, row)
	resp := row.Response()
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

func (s *ResourceService) List(ctx context.Context, filter models.ResourceListFilter) ([]models.ResourceResponse, error) {
	if filter.Status != "" {
		fields := map[string]string{}
		filter.Status = choice(fields, "status", filter.Status, "", resourceStatuses...)
		if len(fields) > 0 {
			return nil, Invalid("validation failed", fields)
		}
	}
	rows, err := s.resources.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]models.ResourceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
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
	}, nil
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
