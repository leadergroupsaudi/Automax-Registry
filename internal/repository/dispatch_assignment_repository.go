package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DispatchAssignmentRepository interface {
	Create(ctx context.Context, row *models.DispatchAssignment) error
	Update(ctx context.Context, row *models.DispatchAssignment) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.DispatchAssignment, error)
	List(ctx context.Context, filter models.DispatchAssignmentFilter) ([]models.DispatchAssignment, error)
	HasOpen(ctx context.Context, resourceID, except uuid.UUID) (bool, error)
	HasOpenMaster(ctx context.Context, incidentID, except uuid.UUID) (bool, error)
	OpenState(ctx context.Context, resourceID uuid.UUID) (string, error)
}

type dispatchAssignmentRepository struct{ db *gorm.DB }

func NewDispatchAssignmentRepository(db *gorm.DB) DispatchAssignmentRepository {
	return &dispatchAssignmentRepository{db: db}
}

func (r *dispatchAssignmentRepository) Create(ctx context.Context, row *models.DispatchAssignment) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *dispatchAssignmentRepository) Update(ctx context.Context, row *models.DispatchAssignment) error {
	res := r.db.WithContext(ctx).Model(&models.DispatchAssignment{}).Where("id = ?", row.ID).Updates(map[string]any{
		"incident_id":  row.IncidentID,
		"resource_id":  row.ResourceID,
		"role":         row.Role,
		"status":       row.Status,
		"responded_at": row.RespondedAt,
		"completed_at": row.CompletedAt,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *dispatchAssignmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.DispatchAssignment{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *dispatchAssignmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.DispatchAssignment, error) {
	var row models.DispatchAssignment
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *dispatchAssignmentRepository) List(ctx context.Context, filter models.DispatchAssignmentFilter) ([]models.DispatchAssignment, error) {
	q := r.db.WithContext(ctx).Model(&models.DispatchAssignment{})
	if filter.IncidentID != nil {
		q = q.Where("incident_id = ?", *filter.IncidentID)
	}
	if filter.ResourceID != nil {
		q = q.Where("resource_id = ?", *filter.ResourceID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	var rows []models.DispatchAssignment
	err := q.Order("offered_at DESC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *dispatchAssignmentRepository) HasOpen(ctx context.Context, resourceID, except uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.DispatchAssignment{}).
		Where("resource_id = ? AND status IN ? AND id <> ?", resourceID, openDispatchStatuses, except).
		Count(&n).Error
	return n > 0, err
}

func (r *dispatchAssignmentRepository) HasOpenMaster(ctx context.Context, incidentID, except uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.DispatchAssignment{}).
		Where("incident_id = ? AND role = ? AND status IN ? AND id <> ?", incidentID, models.DispatchMaster, openDispatchStatuses, except).
		Count(&n).Error
	return n > 0, err
}

func (r *dispatchAssignmentRepository) OpenState(ctx context.Context, resourceID uuid.UUID) (string, error) {
	var statuses []string
	err := r.db.WithContext(ctx).Model(&models.DispatchAssignment{}).
		Where("resource_id = ? AND status IN ?", resourceID, openDispatchStatuses).
		Pluck("status", &statuses).Error
	if err != nil {
		return "", err
	}
	for _, status := range statuses {
		if status == models.DispatchAccepted {
			return models.DispatchAccepted, nil
		}
	}
	if len(statuses) > 0 {
		return models.DispatchOffered, nil
	}
	return "", nil
}

var openDispatchStatuses = []string{models.DispatchOffered, models.DispatchAccepted}
