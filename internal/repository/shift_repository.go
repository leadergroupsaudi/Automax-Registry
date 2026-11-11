package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShiftRepository interface {
	Create(ctx context.Context, row *models.Shift) error
	Update(ctx context.Context, row *models.Shift) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Shift, error)
	List(ctx context.Context) ([]models.Shift, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	AssignmentCount(ctx context.Context, id uuid.UUID) (int64, error)
}

type shiftRepository struct{ db *gorm.DB }

func NewShiftRepository(db *gorm.DB) ShiftRepository {
	return &shiftRepository{db: db}
}

func (r *shiftRepository) Create(ctx context.Context, row *models.Shift) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return err
	}
	return r.reload(ctx, row)
}

func (r *shiftRepository) Update(ctx context.Context, row *models.Shift) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE shift
		SET department_id = ?, name = ?, start_time = ?::time, end_time = ?::time,
		    days_of_week = ?, is_active = ?, updated_at = now()
		WHERE id = ?`,
		row.DepartmentID, row.Name, row.StartTime, row.EndTime, row.DaysOfWeek, row.IsActive, row.ID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return r.reload(ctx, row)
}

func (r *shiftRepository) reload(ctx context.Context, row *models.Shift) error {
	stored, err := r.FindByID(ctx, row.ID)
	if err != nil {
		return err
	}
	*row = *stored
	return nil
}

func (r *shiftRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.Shift{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *shiftRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Shift, error) {
	var row models.Shift
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, department_id, name,
		       to_char(start_time, 'HH24:MI:SS') AS start_time,
		       to_char(end_time, 'HH24:MI:SS') AS end_time,
		       days_of_week, is_active, created_at, updated_at
		FROM shift WHERE id = ?`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (r *shiftRepository) List(ctx context.Context) ([]models.Shift, error) {
	var rows []models.Shift
	err := r.db.WithContext(ctx).Raw(`
		SELECT id, department_id, name,
		       to_char(start_time, 'HH24:MI:SS') AS start_time,
		       to_char(end_time, 'HH24:MI:SS') AS end_time,
		       days_of_week, is_active, created_at, updated_at
		FROM shift
		ORDER BY name ASC, id ASC`).Scan(&rows).Error
	return rows, err
}

func (r *shiftRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Shift{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func (r *shiftRepository) AssignmentCount(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.ResourceShiftAssignment{}).Where("shift_id = ?", id).Count(&n).Error
	return n, err
}
