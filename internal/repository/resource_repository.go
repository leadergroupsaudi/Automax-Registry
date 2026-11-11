package repository

import (
	"context"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ResourceRepository interface {
	Create(ctx context.Context, row *models.Resource) error
	Update(ctx context.Context, row *models.Resource, previousThing uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Resource, error)
	List(ctx context.Context, filter models.ResourceListFilter) ([]models.Resource, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

type resourceRepository struct{ db *gorm.DB }

func NewResourceRepository(db *gorm.DB) ResourceRepository {
	return &resourceRepository{db: db}
}

const resourceSelect = `
	SELECT r.id, r.thing_id, t.name AS name, r.resource_type_id, r.service_point_id,
	       r.status, t.latitude, t.longitude, r.created_at, r.updated_at
	FROM resource r
	JOIN things t ON t.id = r.thing_id
`

func (r *resourceRepository) Create(ctx context.Context, row *models.Resource) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return applyThingResource(tx, row.ThingID, row.ResourceTypeID, row.ServicePointID, row.Status)
	})
	if err != nil {
		return err
	}
	return r.reload(ctx, row)
}

func (r *resourceRepository) Update(ctx context.Context, row *models.Resource, previousThing uuid.UUID) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Resource{}).Where("id = ?", row.ID).Updates(map[string]any{
			"thing_id":         row.ThingID,
			"resource_type_id": row.ResourceTypeID,
			"service_point_id": row.ServicePointID,
			"status":           row.Status,
			"updated_at":       gorm.Expr("now()"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if previousThing != row.ThingID {
			if err := clearThingResource(tx, previousThing); err != nil {
				return err
			}
		}
		return applyThingResource(tx, row.ThingID, row.ResourceTypeID, row.ServicePointID, row.Status)
	})
	if err != nil {
		return err
	}
	return r.reload(ctx, row)
}

func (r *resourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.Resource
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		var shifts, dispatches int64
		if err := tx.Model(&models.ResourceShiftAssignment{}).Where("resource_id = ?", id).Count(&shifts).Error; err != nil {
			return err
		}
		if err := tx.Table("dispatch_assignment").Where("resource_id = ?", id).Count(&dispatches).Error; err != nil {
			return err
		}
		if shifts > 0 || dispatches > 0 {
			return &ResourceInUseError{Assignments: shifts, Dispatches: dispatches}
		}
		if err := tx.Delete(&models.Resource{}, "id = ?", id).Error; err != nil {
			return err
		}
		return clearThingResource(tx, row.ThingID)
	})
}

func (r *resourceRepository) reload(ctx context.Context, row *models.Resource) error {
	stored, err := r.FindByID(ctx, row.ID)
	if err != nil {
		return err
	}
	*row = *stored
	return nil
}

func (r *resourceRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Resource, error) {
	rows, err := r.query(ctx, resourceSelect+` WHERE r.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &rows[0], nil
}

func (r *resourceRepository) List(ctx context.Context, filter models.ResourceListFilter) ([]models.Resource, error) {
	q := resourceSelect + ` WHERE 1 = 1`
	args := make([]any, 0, 3)
	if filter.ResourceTypeID != nil {
		q += ` AND r.resource_type_id = ?`
		args = append(args, *filter.ResourceTypeID)
	}
	if filter.ServicePointID != nil {
		q += ` AND r.service_point_id = ?`
		args = append(args, *filter.ServicePointID)
	}
	if filter.Status != "" {
		q += ` AND r.status = ?`
		args = append(args, filter.Status)
	}
	q += ` ORDER BY r.created_at ASC, r.id ASC`
	return r.query(ctx, q, args...)
}

type resourceScan struct {
	ID             uuid.UUID
	ThingID        uuid.UUID
	Name           string
	ResourceTypeID uuid.UUID
	ServicePointID *uuid.UUID
	Status         string
	Latitude       *float64
	Longitude      *float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (r *resourceRepository) query(ctx context.Context, sql string, args ...any) ([]models.Resource, error) {
	var scanned []resourceScan
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&scanned).Error; err != nil {
		return nil, err
	}
	rows := make([]models.Resource, 0, len(scanned))
	for _, row := range scanned {
		rows = append(rows, models.Resource{
			ID:             row.ID,
			ThingID:        row.ThingID,
			Name:           row.Name,
			ResourceTypeID: row.ResourceTypeID,
			ServicePointID: row.ServicePointID,
			Status:         row.Status,
			Latitude:       row.Latitude,
			Longitude:      row.Longitude,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
	}
	return rows, nil
}

func (r *resourceRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Resource{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func applyThingResource(tx *gorm.DB, thingID, typeID uuid.UUID, pointID *uuid.UUID, status string) error {
	res := tx.Exec(`
		UPDATE things
		SET resource_type_id = ?, service_point_id = ?, resource_status = ?, updated_at = now()
		WHERE id = ?`, typeID, pointID, status, thingID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func clearThingResource(tx *gorm.DB, thingID uuid.UUID) error {
	return tx.Exec(`
		UPDATE things
		SET resource_type_id = NULL, service_point_id = NULL, resource_status = NULL, updated_at = now()
		WHERE id = ?`, thingID).Error
}

// ResourceInUseError reports why a resource row cannot be deleted.
type ResourceInUseError struct {
	Assignments int64
	Dispatches  int64
}

func (e *ResourceInUseError) Error() string { return "resource is in use" }
