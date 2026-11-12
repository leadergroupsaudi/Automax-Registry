package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ResourceTypeRepository interface {
	Create(ctx context.Context, row *models.ResourceType) error
	Update(ctx context.Context, row *models.ResourceType) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.ResourceType, error)
	List(ctx context.Context, filter models.ResourceTypeListFilter) ([]models.ResourceType, int64, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	ResourceCount(ctx context.Context, id uuid.UUID) (int64, error)
}

type resourceTypeRepository struct{ db *gorm.DB }

func NewResourceTypeRepository(db *gorm.DB) ResourceTypeRepository {
	return &resourceTypeRepository{db: db}
}

func (r *resourceTypeRepository) Create(ctx context.Context, row *models.ResourceType) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *resourceTypeRepository) Update(ctx context.Context, row *models.ResourceType) error {
	return r.db.WithContext(ctx).Save(row).Error
}

func (r *resourceTypeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.ResourceType{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *resourceTypeRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.ResourceType, error) {
	var row models.ResourceType
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *resourceTypeRepository) List(ctx context.Context, filter models.ResourceTypeListFilter) ([]models.ResourceType, int64, error) {
	q := r.db.WithContext(ctx).Model(&models.ResourceType{})
	if filter.DepartmentID != nil {
		q = q.Where("department_id = ?", *filter.DepartmentID)
	}
	if filter.Active != nil {
		q = q.Where("is_active = ?", *filter.Active)
	}
	if pattern := likeContains(filter.Search); pattern != "" {
		q = q.Where(
			"(name ILIKE ? ESCAPE '\\' OR code ILIKE ? ESCAPE '\\' OR alt_name ILIKE ? ESCAPE '\\')",
			pattern, pattern, pattern,
		)
	}
	var total int64
	if filter.Page > 0 {
		if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, 0, err
		}
		limit := pageLimit(filter.Limit)
		q = q.Offset((filter.Page - 1) * limit).Limit(limit)
	}
	var rows []models.ResourceType
	if err := q.Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if filter.Page == 0 {
		total = int64(len(rows))
	}
	return rows, total, nil
}

func pageLimit(limit int) int {
	if limit < 1 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (r *resourceTypeRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.ResourceType{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func (r *resourceTypeRepository) ResourceCount(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Resource{}).Where("resource_type_id = ?", id).Count(&n).Error
	return n, err
}
