package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServicePointRepository interface {
	Create(ctx context.Context, row *models.ServicePoint) error
	Update(ctx context.Context, row *models.ServicePoint) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.ServicePoint, error)
	List(ctx context.Context) ([]models.ServicePoint, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
	ResourceCount(ctx context.Context, id uuid.UUID) (int64, error)
	Nearest(ctx context.Context, latitude, longitude float64, resourceTypeID *uuid.UUID, limit int) ([]models.NearestServicePoint, error)
}

type servicePointRepository struct{ db *gorm.DB }

func NewServicePointRepository(db *gorm.DB) ServicePointRepository {
	return &servicePointRepository{db: db}
}

const servicePointSelect = `
	SELECT id, name, department_id,
	       ST_Y(location::geometry) AS latitude,
	       ST_X(location::geometry) AS longitude,
	       created_at, updated_at
	FROM service_point
`

func (r *servicePointRepository) Create(ctx context.Context, row *models.ServicePoint) error {
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO service_point (id, name, department_id, location)
		VALUES (?, ?, ?, ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography)`,
		row.ID, row.Name, row.DepartmentID, row.Longitude, row.Latitude,
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
		SET name = ?, department_id = ?,
		    location = ST_SetSRID(ST_MakePoint(?, ?), 4326)::geography,
		    updated_at = now()
		WHERE id = ?`,
		row.Name, row.DepartmentID, row.Longitude, row.Latitude, row.ID,
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

func (r *servicePointRepository) List(ctx context.Context) ([]models.ServicePoint, error) {
	var rows []models.ServicePoint
	err := r.db.WithContext(ctx).Raw(servicePointSelect + ` ORDER BY name ASC, id ASC`).Scan(&rows).Error
	return rows, err
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
		WHERE (
			? = false
			OR EXISTS (
				SELECT 1 FROM resource r
				WHERE r.service_point_id = sp.id
				  AND r.resource_type_id = ?
				  AND r.status = 'available'
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
