package services

import (
	"context"
	"log"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

var assignmentStatuses = []string{
	models.AssignmentScheduled,
	models.AssignmentActive,
	models.AssignmentCompleted,
	models.AssignmentCancelled,
}

type AssignmentService struct {
	repo      repository.AssignmentRepository
	resources repository.ResourceRepository
	shifts    repository.ShiftRepository
	duty      DutyCache
}

func NewAssignmentService(repo repository.AssignmentRepository, resources repository.ResourceRepository, shifts repository.ShiftRepository) *AssignmentService {
	return &AssignmentService{repo: repo, resources: resources, shifts: shifts}
}

// UseDutyCache refreshes resource:operator when a shift assignment starts or ends.
func (s *AssignmentService) UseDutyCache(cache DutyCache) {
	if s == nil {
		return
	}
	s.duty = cache
}

func (s *AssignmentService) Create(ctx context.Context, req models.AssignmentWriteRequest) (*models.AssignmentResponse, error) {
	row, window, err := s.prepare(ctx, uuid.Nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoOverlap(ctx, row, window); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
		}
		return nil, err
	}
	log.Printf("registry: assignment created id=%s resource_id=%s user_id=%s status=%s", row.ID, row.ResourceID, row.UserID, row.Status)
	s.refreshDuty(ctx, row.ResourceID)
	resp := row.Response()
	return &resp, nil
}

func (s *AssignmentService) Update(ctx context.Context, id uuid.UUID, req models.AssignmentWriteRequest) (*models.AssignmentResponse, error) {
	current, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("assignment not found")
	} else if err != nil {
		return nil, err
	}
	row, window, err := s.prepare(ctx, id, req)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoOverlap(ctx, row, window); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("assignment not found")
		}
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
		}
		return nil, err
	}
	stored, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	log.Printf("registry: assignment updated id=%s resource_id=%s user_id=%s status=%s", stored.ID, stored.ResourceID, stored.UserID, stored.Status)
	s.refreshDuty(ctx, current.ResourceID)
	if stored.ResourceID != current.ResourceID {
		s.refreshDuty(ctx, stored.ResourceID)
	}
	resp := stored.Response()
	return &resp, nil
}

func (s *AssignmentService) Delete(ctx context.Context, id uuid.UUID) error {
	current, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("assignment not found")
	}
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		if repository.IsNotFound(err) {
			return NotFound("assignment not found")
		}
		return err
	}
	log.Printf("registry: assignment deleted id=%s", id)
	s.refreshDuty(ctx, current.ResourceID)
	return nil
}

func (s *AssignmentService) refreshDuty(ctx context.Context, resourceID uuid.UUID) {
	if s == nil || s.duty == nil || resourceID == uuid.Nil {
		return
	}
	if err := s.duty.RefreshOperator(ctx, resourceID); err != nil {
		log.Printf("registry: operator cache refresh failed resource_id=%s err=%v", resourceID, err)
	}
}

func (s *AssignmentService) Get(ctx context.Context, id uuid.UUID) (*models.AssignmentResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("assignment not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *AssignmentService) List(ctx context.Context) ([]models.AssignmentResponse, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.AssignmentResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func (s *AssignmentService) prepare(ctx context.Context, id uuid.UUID, req models.AssignmentWriteRequest) (*models.ResourceShiftAssignment, occupancy, error) {
	fields := map[string]string{}
	if req.ResourceID == uuid.Nil {
		fields["resource_id"] = "resource_id is required"
	}
	if req.UserID == uuid.Nil {
		fields["user_id"] = "user_id is required"
	}
	if req.ShiftID != nil && *req.ShiftID == uuid.Nil {
		fields["shift_id"] = "shift_id must be a UUID"
	}
	status := choice(fields, "status", req.Status, models.AssignmentScheduled, assignmentStatuses...)
	if req.StartAt != nil && req.EndAt != nil && !req.EndAt.After(*req.StartAt) {
		fields["end_at"] = "end_at must be after start_at"
	}
	if req.ShiftID == nil && req.StartAt == nil {
		fields["start_at"] = "start_at is required when shift_id is empty"
	}
	if len(fields) > 0 {
		return nil, occupancy{}, Invalid("validation failed", fields)
	}

	ok, err := s.resources.Exists(ctx, req.ResourceID)
	if err != nil {
		return nil, occupancy{}, err
	}
	if !ok {
		return nil, occupancy{}, Invalid("validation failed", map[string]string{"resource_id": "resource not found"})
	}

	var window occupancy
	if req.ShiftID != nil {
		shift, err := s.shifts.FindByID(ctx, *req.ShiftID)
		if repository.IsNotFound(err) {
			return nil, occupancy{}, Invalid("validation failed", map[string]string{"shift_id": "shift not found"})
		}
		if err != nil {
			return nil, occupancy{}, err
		}
		parsed, ok := occupancyFromShift(shift)
		if !ok {
			return nil, occupancy{}, Invalid("validation failed", map[string]string{"shift_id": "shift schedule is invalid"})
		}
		window = parsed
	} else {
		window = occupancy{start: *req.StartAt, end: req.EndAt}
	}

	return &models.ResourceShiftAssignment{
		ID:         id,
		ResourceID: req.ResourceID,
		UserID:     req.UserID,
		ShiftID:    req.ShiftID,
		StartAt:    req.StartAt,
		EndAt:      req.EndAt,
		Status:     status,
	}, window, nil
}

func (s *AssignmentService) ensureNoOverlap(ctx context.Context, row *models.ResourceShiftAssignment, window occupancy) error {
	if row.Status != models.AssignmentScheduled && row.Status != models.AssignmentActive {
		return nil
	}
	existing, err := s.repo.Occupying(ctx, row.ResourceID, row.ID)
	if err != nil {
		return err
	}
	for _, other := range existing {
		parsed, ok := occupancyFromRow(other)
		if !ok || occupanciesOverlap(window, parsed) {
			log.Printf("registry: assignment rejected resource_id=%s reason=operator overlap existing=%s", row.ResourceID, other.ID)
			return Conflict("resource already has an operator during this time")
		}
	}
	return nil
}

func occupancyFromShift(shift *models.Shift) (occupancy, bool) {
	_, days, msg := normalizeDays(shift.DaysOfWeek)
	start, okStart := parseClock(shift.StartTime)
	end, okEnd := parseClock(shift.EndTime)
	if msg != "" || !okStart || !okEnd {
		return occupancy{}, false
	}
	return occupancy{weekly: &clockWindow{days: days, start: start, end: end}}, true
}

func occupancyFromRow(row models.Occupancy) (occupancy, bool) {
	if row.ShiftID != nil {
		_, days, msg := normalizeDays(row.DaysOfWeek)
		start, okStart := parseClock(row.StartTime)
		end, okEnd := parseClock(row.EndTime)
		if msg != "" || !okStart || !okEnd {
			return occupancy{}, false
		}
		return occupancy{weekly: &clockWindow{days: days, start: start, end: end}}, true
	}
	if row.StartAt == nil {
		return occupancy{}, false
	}
	return occupancy{start: *row.StartAt, end: row.EndAt}, true
}
