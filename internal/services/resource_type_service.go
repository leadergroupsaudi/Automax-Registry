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
}

func NewResourceTypeService(repo repository.ResourceTypeRepository) *ResourceTypeService {
	return &ResourceTypeService{repo: repo}
}

func (s *ResourceTypeService) Create(ctx context.Context, req models.ResourceTypeWriteRequest) (*models.ResourceTypeResponse, error) {
	row, err := resourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, asDuplicateName(err, "a resource type with this name already exists")
	}
	log.Printf("registry: resource type created id=%s department_id=%s name=%s timeout=%d", row.ID, row.DepartmentID, row.Name, row.AcceptanceTimeoutSeconds)
	resp := row.Response()
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
	next, err := resourceTypeFromRequest(req)
	if err != nil {
		return nil, err
	}
	row.DepartmentID = next.DepartmentID
	row.Name = next.Name
	row.AcceptanceTimeoutSeconds = next.AcceptanceTimeoutSeconds
	if err := s.repo.Update(ctx, row); err != nil {
		return nil, asDuplicateName(err, "a resource type with this name already exists")
	}
	log.Printf("registry: resource type updated id=%s department_id=%s name=%s timeout=%d", row.ID, row.DepartmentID, row.Name, row.AcceptanceTimeoutSeconds)
	resp := row.Response()
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

func (s *ResourceTypeService) List(ctx context.Context, filter models.ResourceTypeListFilter) ([]models.ResourceTypeResponse, error) {
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]models.ResourceTypeResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func resourceTypeFromRequest(req models.ResourceTypeWriteRequest) (*models.ResourceType, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	timeout := timeoutSeconds(fields, req.AcceptanceTimeoutSeconds)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	return &models.ResourceType{
		DepartmentID:             req.DepartmentID,
		Name:                     name,
		AcceptanceTimeoutSeconds: timeout,
	}, nil
}
