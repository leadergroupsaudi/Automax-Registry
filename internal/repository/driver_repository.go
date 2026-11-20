package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DriverRepository interface {
	Create(ctx context.Context, row *models.Driver) error
	Update(ctx context.Context, row *models.Driver) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Driver, error)
	FindByUniqueID(ctx context.Context, uniqueID string) (*models.Driver, error)
	List(ctx context.Context) ([]models.Driver, error)
}

type driverRepository struct{ db *gorm.DB }

func NewDriverRepository(db *gorm.DB) DriverRepository {
	return &driverRepository{db: db}
}

func (r *driverRepository) Create(ctx context.Context, row *models.Driver) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *driverRepository) Update(ctx context.Context, row *models.Driver) error {
	return r.db.WithContext(ctx).Save(row).Error
}

func (r *driverRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.Driver{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *driverRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Driver, error) {
	var row models.Driver
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *driverRepository) FindByUniqueID(ctx context.Context, uniqueID string) (*models.Driver, error) {
	var row models.Driver
	err := r.db.WithContext(ctx).First(&row, "unique_id = ?", uniqueID).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *driverRepository) List(ctx context.Context) ([]models.Driver, error) {
	var rows []models.Driver
	err := r.db.WithContext(ctx).Order("name ASC, unique_id ASC").Find(&rows).Error
	return rows, err
}
