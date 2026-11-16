package services

import (
	"context"
	"log"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

var dispatchRoles = []string{models.DispatchMaster, models.DispatchSupporting}

var dispatchStatuses = []string{
	models.DispatchOffered,
	models.DispatchAccepted,
	models.DispatchRejected,
	models.DispatchExpired,
	models.DispatchCompleted,
	models.DispatchCancelled,
}

type DispatchAssignmentService struct {
	repo      repository.DispatchAssignmentRepository
	resources repository.ResourceRepository
	state     ResourceStateCache
}

func NewDispatchAssignmentService(repo repository.DispatchAssignmentRepository, resources repository.ResourceRepository) *DispatchAssignmentService {
	return &DispatchAssignmentService{repo: repo, resources: resources}
}

func (s *DispatchAssignmentService) UseStateCache(cache ResourceStateCache) {
	if s == nil {
		return
	}
	s.state = cache
}

func (s *DispatchAssignmentService) Create(ctx context.Context, req models.DispatchAssignmentWriteRequest) (*models.DispatchAssignmentResponse, error) {
	row, err := s.prepare(ctx, uuid.Nil, nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
		}
		return nil, err
	}
	log.Printf("registry: dispatch assignment created id=%s incident_id=%s resource_id=%s role=%s status=%s", row.ID, row.IncidentID, row.ResourceID, row.Role, row.Status)
	if err := s.syncResource(ctx, row.ResourceID); err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *DispatchAssignmentService) Update(ctx context.Context, id uuid.UUID, req models.DispatchAssignmentWriteRequest) (*models.DispatchAssignmentResponse, error) {
	current, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("dispatch assignment not found")
	}
	if err != nil {
		return nil, err
	}
	row, err := s.prepare(ctx, id, current, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("dispatch assignment not found")
		}
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
		}
		return nil, err
	}
	log.Printf("registry: dispatch assignment updated id=%s incident_id=%s resource_id=%s role=%s status=%s", row.ID, row.IncidentID, row.ResourceID, row.Role, row.Status)
	if err := s.syncResource(ctx, row.ResourceID); err != nil {
		return nil, err
	}
	if current.ResourceID != row.ResourceID {
		if err := s.syncResource(ctx, current.ResourceID); err != nil {
			return nil, err
		}
	}
	resp := row.Response()
	return &resp, nil
}

func (s *DispatchAssignmentService) Delete(ctx context.Context, id uuid.UUID) error {
	current, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("dispatch assignment not found")
	}
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		if repository.IsNotFound(err) {
			return NotFound("dispatch assignment not found")
		}
		return err
	}
	log.Printf("registry: dispatch assignment deleted id=%s", id)
	return s.syncResource(ctx, current.ResourceID)
}

func (s *DispatchAssignmentService) Get(ctx context.Context, id uuid.UUID) (*models.DispatchAssignmentResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("dispatch assignment not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *DispatchAssignmentService) List(ctx context.Context, filter models.DispatchAssignmentFilter) ([]models.DispatchAssignmentResponse, error) {
	if filter.Status != "" {
		fields := map[string]string{}
		filter.Status = choice(fields, "status", filter.Status, "", dispatchStatuses...)
		if len(fields) > 0 {
			return nil, Invalid("validation failed", fields)
		}
	}
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]models.DispatchAssignmentResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func (s *DispatchAssignmentService) prepare(ctx context.Context, id uuid.UUID, current *models.DispatchAssignment, req models.DispatchAssignmentWriteRequest) (*models.DispatchAssignment, error) {
	fields := map[string]string{}
	if req.IncidentID == uuid.Nil {
		fields["incident_id"] = "incident_id is required"
	}
	if req.ResourceID == uuid.Nil {
		fields["resource_id"] = "resource_id is required"
	}
	role := choice(fields, "role", req.Role, models.DispatchMaster, dispatchRoles...)
	status := choice(fields, "status", req.Status, models.DispatchOffered, dispatchStatuses...)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}

	resource, err := s.resources.FindByID(ctx, req.ResourceID)
	if repository.IsNotFound(err) {
		return nil, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
	}
	if err != nil {
		return nil, err
	}
	if dispatchOpen(status) && resource.Status == models.ResourceMaintenance {
		return nil, Invalid("validation failed", map[string]string{"resource_id": "resource is in maintenance"})
	}
	if dispatchOpen(status) {
		busy, err := s.repo.HasOpen(ctx, req.ResourceID, id)
		if err != nil {
			return nil, err
		}
		if busy {
			return nil, Conflict("resource is already assigned to an incident")
		}
		if role == models.DispatchMaster {
			taken, err := s.repo.HasOpenMaster(ctx, req.IncidentID, id)
			if err != nil {
				return nil, err
			}
			if taken {
				return nil, Conflict("incident already has a master unit")
			}
		}
	}

	now := time.Now().UTC()
	row := &models.DispatchAssignment{
		ID:         id,
		IncidentID: req.IncidentID,
		ResourceID: req.ResourceID,
		Role:       role,
		Status:     status,
		OfferedAt:  now,
	}
	if current != nil {
		row.OfferedAt = current.OfferedAt
		row.RespondedAt = current.RespondedAt
		row.CompletedAt = current.CompletedAt
	}
	if status == models.DispatchAccepted || status == models.DispatchRejected {
		if row.RespondedAt == nil {
			row.RespondedAt = &now
		}
	}
	if status == models.DispatchCompleted || status == models.DispatchExpired || status == models.DispatchCancelled {
		if row.CompletedAt == nil {
			row.CompletedAt = &now
		}
	}
	return row, nil
}

func (s *DispatchAssignmentService) syncResource(ctx context.Context, resourceID uuid.UUID) error {
	resource, err := s.resources.FindByID(ctx, resourceID)
	if repository.IsNotFound(err) {
		return NotFound("resource not found")
	}
	if err != nil {
		return err
	}
	if resource.Status == models.ResourceMaintenance {
		return nil
	}
	open, err := s.repo.OpenState(ctx, resourceID)
	if err != nil {
		return err
	}
	next := models.ResourceAvailable
	switch open {
	case models.DispatchAccepted:
		next = models.ResourceDispatched
	case models.DispatchOffered:
		next = models.ResourceOffered
	default:
		if resource.Status != models.ResourceOffered && resource.Status != models.ResourceDispatched {
			return nil
		}
	}
	if resource.Status == next {
		s.remember(ctx, resource)
		return nil
	}
	if err := s.resources.SetStatus(ctx, resourceID, next); err != nil {
		if repository.IsNotFound(err) {
			return NotFound("resource not found")
		}
		return err
	}
	resource.Status = next
	log.Printf("registry: resource status followed dispatch assignment resource_id=%s status=%s", resourceID, next)
	s.remember(ctx, resource)
	return nil
}

func (s *DispatchAssignmentService) remember(ctx context.Context, row *models.Resource) {
	if s == nil || s.state == nil || row == nil {
		return
	}
	if err := s.state.RememberResource(ctx, row.ID, row.Status, row.ResourceTypeID); err != nil {
		log.Printf("registry: resource state cache refresh failed resource_id=%s err=%v", row.ID, err)
	}
}

func dispatchOpen(status string) bool {
	return status == models.DispatchOffered || status == models.DispatchAccepted
}
