package repository

import (
	"context"
	"errors"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrUnknownResource means a station assignment listed a resource id that does not exist.
var ErrUnknownResource = errors.New("unknown resource")

type ServicePointRepository interface {
	Create(ctx context.Context, row *models.ServicePoint) error
	Update(ctx context.Context, row *models.ServicePoint) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.ServicePoint, error)
	List(ctx context.Context, filter models.ServicePointListFilter) ([]models.ServicePoint, int64, error)
	AssignResources(ctx context.Context, pointID uuid.UUID, resourceIDs []uuid.UUID) error
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	ResourceCount(ctx context.Context, id uuid.UUID) (int64, error)
	Nearest(ctx context.Context, latitude, longitude float64, resourceTypeID *uuid.UUID, limit int) ([]models.NearestServicePoint, error)
}

type servicePointRepository struct{ db *gorm.DB }

func NewServicePointRepository(db *gorm.DB) ServicePointRepository {
	return &servicePointRepository{db: db}
}

const servicePointSelect = `
	SELECT id, name, department_id, alt_name, radio_id, location_name, location_id,
	       active_till_date, is_active, icon,
	       ST_Y(location::geometry) AS latitude,
	       ST_X(location::geometry) AS longitude,
	       created_at, updated_at,
	       COALESCE((
	           SELECT json_agg(r.id ORDER BY r.id)
	           FROM resource r
	           WHERE r.service_point_id = service_point.id
	       ), '[]'::json) AS resource_ids
	FROM service_point
`

func (r *servicePointRepository) Create(ctx context.Context, row *models.ServicePoint) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO service_point (
			id, name, department_id, location, alt_name, radio_id, location_name, location_id,
			active_till_date, is_active, icon
		)
		VALUES (?, ?, ?, ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.Name, row.DepartmentID, row.Longitude, row.Latitude,
		row.AltName, row.RadioID, row.LocationName, row.LocationID, row.ActiveTillDate, row.IsActive, row.Icon,
	).Error
	if err != nil {
		return err
	}
	stored, err := r.FindByID(ctx, row.ID)
	if err != nil {
		return err
	}
	*row = *stored
	return nil
}

func (r *servicePointRepository) Update(ctx context.Context, row *models.ServicePoint) error {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE service_point
		SET name = ?, department_id = ?, alt_name = ?, radio_id = ?, location_name = ?, location_id = ?,
		    location = ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography,
		    active_till_date = ?, is_active = ?, icon = ?,
		    updated_at = now()
		WHERE id = ?`,
		row.Name, row.DepartmentID, row.AltName, row.RadioID, row.LocationName, row.LocationID,
		row.Longitude, row.Latitude, row.ActiveTillDate, row.IsActive, row.Icon, row.ID,
	)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	stored, err := r.FindByID(ctx, row.ID)
	if err != nil {
		return err
	}
	*row = *stored
	return nil
}

func (r *servicePointRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Exec(`DELETE FROM service_point WHERE id = ?`, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *servicePointRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.ServicePoint, error) {
	var row models.ServicePoint
	err := r.db.WithContext(ctx).Raw(servicePointSelect+` WHERE id = ?`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	return &row, nil
}

func (r *servicePointRepository) List(ctx context.Context, filter models.ServicePointListFilter) ([]models.ServicePoint, int64, error) {
	where := ` WHERE 1 = 1`
	args := make([]any, 0, 6)
	if filter.DepartmentID != nil {
		where += ` AND department_id = ?`
		args = append(args, *filter.DepartmentID)
	}
	if filter.Active != nil {
		where += ` AND is_active = ?`
		args = append(args, *filter.Active)
	}
	if pattern := likeContains(filter.Search); pattern != "" {
		where += ` AND (name ILIKE ? ESCAPE '\' OR alt_name ILIKE ? ESCAPE '\' OR radio_id ILIKE ? ESCAPE '\')`
		args = append(args, pattern, pattern, pattern)
	}
	var total int64
	if filter.Page > 0 {
		if err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM service_point`+where, args...).Scan(&total).Error; err != nil {
			return nil, 0, err
		}
	}
	q := servicePointSelect + where + ` ORDER BY name ASC, id ASC`
	qargs := append([]any{}, args...)
	if filter.Page > 0 {
		limit := pageLimit(filter.Limit)
		q += ` LIMIT ? OFFSET ?`
		qargs = append(qargs, limit, (filter.Page-1)*limit)
	}
	var rows []models.ServicePoint
	if err := r.db.WithContext(ctx).Raw(q, qargs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	if filter.Page == 0 {
		total = int64(len(rows))
	}
	return rows, total, nil
}

func (r *servicePointRepository) AssignResources(ctx context.Context, pointID uuid.UUID, resourceIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(resourceIDs) > 0 {
			var n int64
			if err := tx.Model(&models.Resource{}).Where("id IN ?", resourceIDs).Count(&n).Error; err != nil {
				return err
			}
			if int(n) != len(resourceIDs) {
				return ErrUnknownResource
			}
		}
		detachThings := `
			UPDATE things SET service_point_id = NULL, updated_at = now()
			WHERE id IN (SELECT thing_id FROM resource WHERE service_point_id = ?`
		detachResources := `UPDATE resource SET service_point_id = NULL, updated_at = now() WHERE service_point_id = ?`
		args := []any{pointID}
		if len(resourceIDs) > 0 {
			detachThings += ` AND id NOT IN ?`
			detachResources += ` AND id NOT IN ?`
			args = append(args, resourceIDs)
		}
		detachThings += `)`
		if err := tx.Exec(detachThings, args...).Error; err != nil {
			return err
		}
		if err := tx.Exec(detachResources, args...).Error; err != nil {
			return err
		}
		if len(resourceIDs) == 0 {
			return nil
		}
		if err := tx.Exec(`
			UPDATE things SET service_point_id = ?, updated_at = now()
			WHERE id IN (SELECT thing_id FROM resource WHERE id IN ?)`, pointID, resourceIDs).Error; err != nil {
			return err
		}
		return tx.Exec(`
			UPDATE resource SET service_point_id = ?, updated_at = now()
			WHERE id IN ?`, pointID, resourceIDs).Error
	})
}

func (r *servicePointRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM service_point WHERE id = ?`, id).Scan(&n).Error
	return n > 0, err
}

func (r *servicePointRepository) Nearest(ctx context.Context, latitude, longitude float64, resourceTypeID *uuid.UUID, limit int) ([]models.NearestServicePoint, error) {
	filter := resourceTypeID != nil && *resourceTypeID != uuid.Nil
	typeID := uuid.Nil
	if filter {
		typeID = *resourceTypeID
	}
	var scanned []nearestStationRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT sp.id, sp.name, sp.department_id,
		       ST_Y(sp.location::geometry) AS latitude,
		       ST_X(sp.location::geometry) AS longitude,
		       ST_Distance(sp.location, ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography) AS distance_meters
		FROM service_point sp
		WHERE sp.is_active = true
		  AND (
			? = false
			OR EXISTS (
				SELECT 1 FROM resource r
				WHERE r.service_point_id = sp.id
				  AND r.resource_type_id = ?
				  AND r.status = 'available'
				  AND r.is_active = true
			)
		)
		ORDER BY distance_meters ASC, sp.id ASC
		LIMIT ?`, longitude, latitude, filter, typeID, limit).Scan(&scanned).Error
	if err != nil {
		return nil, err
	}
	out := make([]models.NearestServicePoint, 0, len(scanned))
	for _, row := range scanned {
		out = append(out, models.NearestServicePoint{
			ID:             row.ID,
			Name:           row.Name,
			DepartmentID:   row.DepartmentID,
			Location:       models.Location{Lat: row.Latitude, Lng: row.Longitude},
			DistanceMeters: row.DistanceMeters,
		})
	}
	return out, nil
}

type nearestStationRow struct {
	ID             uuid.UUID `gorm:"column:id"`
	Name           string    `gorm:"column:name"`
	DepartmentID   uuid.UUID `gorm:"column:department_id"`
	Latitude       float64   `gorm:"column:latitude"`
	Longitude      float64   `gorm:"column:longitude"`
	DistanceMeters float64   `gorm:"column:distance_meters"`
}

func (r *servicePointRepository) ResourceCount(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Resource{}).Where("service_point_id = ?", id).Count(&n).Error
	return n, err
}
