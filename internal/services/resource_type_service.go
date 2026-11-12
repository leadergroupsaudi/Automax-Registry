package services

import (
	"context"
	"log"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ResourceTypeService struct {
	repo repository.ResourceTypeRepository
	pub  Publisher
}

func NewResourceTypeService(repo repository.ResourceTypeRepository) *ResourceTypeService {
	return &ResourceTypeService{repo: repo}
}

func (s *ResourceTypeService) UsePublisher(pub Publisher) {
	if s == nil {
		return
	}
	s.pub = pub
}

func (s *ResourceTypeService) Create(ctx context.Context, req models.ResourceTypeWriteRequest) (*models.ResourceTypeResponse, error) {
	row, err := mergeResourceType(nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, resourceTypeWriteErr(err)
	}
	log.Printf("registry: resource type created id=%s department_id=%s name=%s timeout=%d", row.ID, row.DepartmentID, row.Name, row.AcceptanceTimeoutSeconds)
	resp := row.Response()
	publishEvent(s.pub, models.SubjectResourceTypeCreated, row.ID, resp)
	return &resp, nil
}

func (s *ResourceTypeService) Update(ctx context.Context, id uuid.UUID, req models.ResourceTypeWriteRequest) (*models.ResourceTypeResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource type not found")
	}
	if err != nil {
		return nil, err
	}
	next, err := mergeResourceType(row, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, next); err != nil {
		return nil, resourceTypeWriteErr(err)
	}
	log.Printf("registry: resource type updated id=%s department_id=%s name=%s timeout=%d", next.ID, next.DepartmentID, next.Name, next.AcceptanceTimeoutSeconds)
	resp := next.Response()
	publishEvent(s.pub, models.SubjectResourceTypeUpdated, next.ID, resp)
	return &resp, nil
}

func (s *ResourceTypeService) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.ResourceCount(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return Conflict("resource type is assigned to one or more resources")
	}
	err = s.repo.Delete(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("resource type not found")
	}
	if repository.IsForeignKeyViolation(err) {
		return Conflict("resource type is assigned to one or more resources")
	}
	if err != nil {
		return err
	}
	log.Printf("registry: resource type deleted id=%s", id)
	publishEvent(s.pub, models.SubjectResourceTypeDeleted, id, nil)
	return nil
}

func (s *ResourceTypeService) Get(ctx context.Context, id uuid.UUID) (*models.ResourceTypeResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("resource type not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *ResourceTypeService) List(ctx context.Context, filter models.ResourceTypeListFilter) ([]models.ResourceTypeResponse, int64, error) {
	rows, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.ResourceTypeResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, total, nil
}

func resourceTypeWriteErr(err error) error {
	switch repository.UniqueConstraint(err) {
	case "":
		return err
	case "uq_resource_type_department_code":
		return Conflict("a resource type with this code already exists")
	default:
		return Conflict("a resource type with this name already exists")
	}
}
