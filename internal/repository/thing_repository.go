package repository

import (
	"context"
	"errors"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ThingRepository interface {
	Create(ctx context.Context, thing *models.Thing) error
	Update(ctx context.Context, thing *models.Thing) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Thing, error)
	List(ctx context.Context, filter models.ThingListFilter) ([]models.Thing, error)
	BlockingRefCount(ctx context.Context, id uuid.UUID) (int64, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	UpdateCoordinates(ctx context.Context, id uuid.UUID, latitude, longitude float64) error
}

type thingRepository struct{ db *gorm.DB }

func NewThingRepository(db *gorm.DB) ThingRepository {
	return &thingRepository{db: db}
}

func (r *thingRepository) Create(ctx context.Context, thing *models.Thing) error {
	return r.db.WithContext(ctx).Create(thing).Error
}

func (r *thingRepository) Update(ctx context.Context, thing *models.Thing) error {
	return r.db.WithContext(ctx).Save(thing).Error
}

func (r *thingRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.Thing{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *thingRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Thing, error) {
	var thing models.Thing
	err := r.db.WithContext(ctx).First(&thing, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &thing, nil
}

func (r *thingRepository) List(ctx context.Context, filter models.ThingListFilter) ([]models.Thing, error) {
	q := r.db.WithContext(ctx).Model(&models.Thing{})
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	if filter.DepartmentID != nil {
		q = q.Where("department_id = ?", *filter.DepartmentID)
	}
	if filter.GroupID != nil {
		q = q.Where("group_id = ?", *filter.GroupID)
	}
	var things []models.Thing
	err := q.Order("created_at ASC, id ASC").Find(&things).Error
	return things, err
}

func (r *thingRepository) BlockingRefCount(ctx context.Context, id uuid.UUID) (int64, error) {
	var creds, refs int64
	if err := r.db.WithContext(ctx).Model(&models.DeviceCredential{}).Where("thing_id = ?", id).Count(&creds).Error; err != nil {
		return 0, err
	}
	if err := r.db.WithContext(ctx).Model(&models.ThingExternalRef{}).Where("thing_id = ?", id).Count(&refs).Error; err != nil {
		return 0, err
	}
	return creds + refs, nil
}

func (r *thingRepository) UpdateCoordinates(ctx context.Context, id uuid.UUID, latitude, longitude float64) error {
	res := r.db.WithContext(ctx).Model(&models.Thing{}).Where("id = ?", id).Updates(map[string]any{
		"latitude":   latitude,
		"longitude":  longitude,
		"updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *thingRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Thing{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
