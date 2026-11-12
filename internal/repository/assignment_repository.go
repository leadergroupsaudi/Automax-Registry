package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AssignmentRepository interface {
	Create(ctx context.Context, row *models.ResourceShiftAssignment) error
	Update(ctx context.Context, row *models.ResourceShiftAssignment) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.ResourceShiftAssignment, error)
	List(ctx context.Context) ([]models.ResourceShiftAssignment, error)
	Occupying(ctx context.Context, resourceID, except uuid.UUID) ([]models.Occupancy, error)
}

type assignmentRepository struct{ db *gorm.DB }

func NewAssignmentRepository(db *gorm.DB) AssignmentRepository {
	return &assignmentRepository{db: db}
}

func (r *assignmentRepository) Create(ctx context.Context, row *models.ResourceShiftAssignment) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *assignmentRepository) Update(ctx context.Context, row *models.ResourceShiftAssignment) error {
	res := r.db.WithContext(ctx).Model(&models.ResourceShiftAssignment{}).Where("id = ?", row.ID).Updates(map[string]any{
		"resource_id":     row.ResourceID,
		"user_id":         row.UserID,
		"partner_user_id": row.PartnerUserID,
		"shift_id":        row.ShiftID,
		"start_at":        row.StartAt,
		"end_at":          row.EndAt,
		"status":          row.Status,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *assignmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.ResourceShiftAssignment{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *assignmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.ResourceShiftAssignment, error) {
	var row models.ResourceShiftAssignment
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *assignmentRepository) List(ctx context.Context) ([]models.ResourceShiftAssignment, error) {
	var rows []models.ResourceShiftAssignment
	err := r.db.WithContext(ctx).Order("start_at ASC NULLS LAST, id ASC").Find(&rows).Error
	return rows, err
}

func (r *assignmentRepository) Occupying(ctx context.Context, resourceID, except uuid.UUID) ([]models.Occupancy, error) {
	var rows []models.Occupancy
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id, a.shift_id, a.start_at, a.end_at,
		       COALESCE(s.days_of_week, '') AS days_of_week,
		       COALESCE(to_char(s.start_time, 'HH24:MI:SS'), '') AS start_time,
		       COALESCE(to_char(s.end_time, 'HH24:MI:SS'), '') AS end_time
		FROM resource_shift_assignment a
		LEFT JOIN shift s ON s.id = a.shift_id
		WHERE a.resource_id = ?
		  AND a.status IN ('scheduled', 'active')
		  AND a.id <> ?`, resourceID, except).Scan(&rows).Error
	return rows, err
}
