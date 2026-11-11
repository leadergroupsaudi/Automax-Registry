package services

import (
	"context"
	"log"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ShiftService struct {
	repo repository.ShiftRepository
}

func NewShiftService(repo repository.ShiftRepository) *ShiftService {
	return &ShiftService{repo: repo}
}

func (s *ShiftService) Create(ctx context.Context, req models.ShiftWriteRequest) (*models.ShiftResponse, error) {
	row, err := shiftFromRequest(uuid.Nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, asDuplicateName(err, "a shift with this name already exists")
	}
	log.Printf("registry: shift created id=%s name=%s days=%s start=%s end=%s active=%t", row.ID, row.Name, row.DaysOfWeek, row.StartTime, row.EndTime, row.IsActive)
	resp := row.Response()
	return &resp, nil
}

func (s *ShiftService) Update(ctx context.Context, id uuid.UUID, req models.ShiftWriteRequest) (*models.ShiftResponse, error) {
	if _, err := s.repo.FindByID(ctx, id); repository.IsNotFound(err) {
		return nil, NotFound("shift not found")
	} else if err != nil {
		return nil, err
	}
	row, err := shiftFromRequest(id, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("shift not found")
		}
		return nil, asDuplicateName(err, "a shift with this name already exists")
	}
	log.Printf("registry: shift updated id=%s name=%s days=%s start=%s end=%s active=%t", row.ID, row.Name, row.DaysOfWeek, row.StartTime, row.EndTime, row.IsActive)
	resp := row.Response()
	return &resp, nil
}

func (s *ShiftService) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.AssignmentCount(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return Conflict("shift is used by one or more assignments")
	}
	err = s.repo.Delete(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("shift not found")
	}
	if repository.IsForeignKeyViolation(err) {
		return Conflict("shift is used by one or more assignments")
	}
	if err != nil {
		return err
	}
	log.Printf("registry: shift deleted id=%s", id)
	return nil
}

func (s *ShiftService) Get(ctx context.Context, id uuid.UUID) (*models.ShiftResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("shift not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

func (s *ShiftService) List(ctx context.Context) ([]models.ShiftResponse, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.ShiftResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func shiftFromRequest(id uuid.UUID, req models.ShiftWriteRequest) (*models.Shift, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 100)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	days, _, daysMsg := normalizeDays(req.DaysOfWeek)
	if daysMsg != "" {
		fields["days_of_week"] = daysMsg
	}
	start, startOK := parseClock(req.StartTime)
	end, endOK := parseClock(req.EndTime)
	if !startOK {
		fields["start_time"] = "start_time must be HH:MM or HH:MM:SS"
	}
	if !endOK {
		fields["end_time"] = "end_time must be HH:MM or HH:MM:SS"
	}
	if startOK && endOK && start == end {
		fields["end_time"] = "end_time must be different from start_time"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	return &models.Shift{
		ID:           id,
		DepartmentID: req.DepartmentID,
		Name:         name,
		StartTime:    formatClock(start),
		EndTime:      formatClock(end),
		DaysOfWeek:   days,
		IsActive:     activeOrDefault(req.IsActive),
	}, nil
}
