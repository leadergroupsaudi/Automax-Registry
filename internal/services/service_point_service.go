package services

import (
	"context"
	"log"
	"math"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ServicePointService struct {
	repo repository.ServicePointRepository
}

func NewServicePointService(repo repository.ServicePointRepository) *ServicePointService {
	return &ServicePointService{repo: repo}
}

func (s *ServicePointService) Create(ctx context.Context, req models.ServicePointWriteRequest) (*models.ServicePointResponse, error) {
	row, err := servicePointFromRequest(uuid.Nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, asDuplicateName(err, "a service point with this name already exists")
	}
	log.Printf("registry: service point created id=%s department_id=%s name=%s lat=%v lng=%v", row.ID, row.DepartmentID, row.Name, row.Latitude, row.Longitude)
	resp := row.Response()
	return &resp, nil
}

func (s *ServicePointService) Update(ctx context.Context, id uuid.UUID, req models.ServicePointWriteRequest) (*models.ServicePointResponse, error) {
	if _, err := s.repo.FindByID(ctx, id); repository.IsNotFound(err) {
		return nil, NotFound("service point not found")
	} else if err != nil {
		return nil, err
	}
	row, err := servicePointFromRequest(id, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("service point not found")
		}
		return nil, asDuplicateName(err, "a service point with this name already exists")
	}
	log.Printf("registry: service point updated id=%s department_id=%s name=%s lat=%v lng=%v", row.ID, row.DepartmentID, row.Name, row.Latitude, row.Longitude)
	resp := row.Response()
	return &resp, nil
}

func (s *ServicePointService) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.repo.ResourceCount(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return Conflict("service point is the home station of one or more resources")
	}
	err = s.repo.Delete(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("service point not found")
	}
	if repository.IsForeignKeyViolation(err) {
		return Conflict("service point is the home station of one or more resources")
	}
	if err != nil {
		return err
	}
	log.Printf("registry: service point deleted id=%s", id)
	return nil
}

func (s *ServicePointService) Get(ctx context.Context, id uuid.UUID) (*models.ServicePointResponse, error) {
	row, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("service point not found")
	}
	if err != nil {
		return nil, err
	}
	resp := row.Response()
	return &resp, nil
}

// FindNearest returns stations ordered by PostGIS ST_Distance.
// A resourceTypeID keeps only stations that have an available resource of that type.
func (s *ServicePointService) FindNearest(ctx context.Context, lat, lng float64, resourceTypeID *uuid.UUID, limit int) ([]models.NearestServicePoint, error) {
	fields := map[string]string{}
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		fields["lat"] = "lat must be between -90 and 90"
	}
	if math.IsNaN(lng) || math.IsInf(lng, 0) || lng < -180 || lng > 180 {
		fields["lng"] = "lng must be between -180 and 180"
	}
	if limit < 1 {
		fields["limit"] = "limit must be at least 1"
	}
	if limit > 100 {
		fields["limit"] = "limit must be at most 100"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	rows, err := s.repo.Nearest(ctx, lat, lng, resourceTypeID, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []models.NearestServicePoint{}
	}
	log.Printf("registry: nearest service points lat=%v lng=%v resource_type_id=%s limit=%d count=%d", lat, lng, typeLabel(resourceTypeID), limit, len(rows))
	return rows, nil
}

func typeLabel(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (s *ServicePointService) List(ctx context.Context) ([]models.ServicePointResponse, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.ServicePointResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func servicePointFromRequest(id uuid.UUID, req models.ServicePointWriteRequest) (*models.ServicePoint, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	lat, lng := validateLocation(fields, req.Location)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	return &models.ServicePoint{
		ID:           id,
		Name:         name,
		DepartmentID: req.DepartmentID,
		Latitude:     lat,
		Longitude:    lng,
	}, nil
}

func validateLocation(fields map[string]string, loc *models.LocationInput) (float64, float64) {
	if loc == nil {
		fields["location"] = "location is required"
		return 0, 0
	}
	var lat, lng float64
	if loc.Lat == nil {
		fields["lat"] = "lat is required"
	} else if *loc.Lat < -90 || *loc.Lat > 90 {
		fields["lat"] = "lat must be from -90 to 90"
	} else {
		lat = *loc.Lat
	}
	if loc.Lng == nil {
		fields["lng"] = "lng is required"
	} else if *loc.Lng < -180 || *loc.Lng > 180 {
		fields["lng"] = "lng must be from -180 to 180"
	} else {
		lng = *loc.Lng
	}
	return lat, lng
}
