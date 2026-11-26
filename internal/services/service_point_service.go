package services

import (
	"context"
	"errors"
	"log"
	"math"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ServicePointService struct {
	repo      repository.ServicePointRepository
	resources repository.ResourceRepository
	logs      *DispatchLogService
	pub       Publisher
}

func NewServicePointService(repo repository.ServicePointRepository) *ServicePointService {
	return &ServicePointService{repo: repo}
}

func (s *ServicePointService) UsePublisher(pub Publisher) {
	if s == nil {
		return
	}
	s.pub = pub
}

func (s *ServicePointService) UseResources(resources repository.ResourceRepository) {
	if s == nil {
		return
	}
	s.resources = resources
}

func (s *ServicePointService) UseDispatchLog(logs *DispatchLogService) {
	if s == nil {
		return
	}
	s.logs = logs
}

func (s *ServicePointService) Create(ctx context.Context, req models.ServicePointWriteRequest) (*models.ServicePointResponse, error) {
	row, err := mergeServicePoint(nil, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, servicePointConflict(err)
	}
	if err := s.assign(ctx, row, req); err != nil {
		_ = s.repo.Delete(ctx, row.ID)
		return nil, err
	}
	log.Printf("registry: service point created id=%s department_id=%s name=%s lat=%v lng=%v", row.ID, row.DepartmentID, row.Name, row.Latitude, row.Longitude)
	resp := row.Response()
	publishEvent(s.pub, models.SubjectServicePointCreated, row.ID, resp)
	return &resp, nil
}

func (s *ServicePointService) Update(ctx context.Context, id uuid.UUID, req models.ServicePointWriteRequest) (*models.ServicePointResponse, error) {
	current, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("service point not found")
	} else if err != nil {
		return nil, err
	}
	row, err := mergeServicePoint(current, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if repository.IsNotFound(err) {
			return nil, NotFound("service point not found")
		}
		return nil, servicePointConflict(err)
	}
	if err := s.assign(ctx, row, req); err != nil {
		return nil, err
	}
	log.Printf("registry: service point updated id=%s department_id=%s name=%s lat=%v lng=%v", row.ID, row.DepartmentID, row.Name, row.Latitude, row.Longitude)
	resp := row.Response()
	publishEvent(s.pub, models.SubjectServicePointUpdated, row.ID, resp)
	return &resp, nil
}

func (s *ServicePointService) assign(ctx context.Context, row *models.ServicePoint, req models.ServicePointWriteRequest) error {
	if req.ResourceIDs == nil {
		return nil
	}
	previous := uniqueUUIDs([]uuid.UUID(row.ResourceIDs))
	ids := uniqueUUIDs(*req.ResourceIDs)
	moved, err := s.previousStations(ctx, row.ID, previous, ids)
	if err != nil {
		return err
	}
	if err := s.repo.AssignResources(ctx, row.ID, ids); err != nil {
		if errors.Is(err, repository.ErrUnknownResource) {
			return Invalid("validation failed", map[string]string{"resource_ids": "one or more resources were not found"})
		}
		return err
	}
	if err := s.recordLeftStations(ctx, row.ID, moved); err != nil {
		return err
	}
	if err := s.recordStationChanges(ctx, row, previous, ids); err != nil {
		return err
	}
	stored, err := s.repo.FindByID(ctx, row.ID)
	if err != nil {
		return err
	}
	*row = *stored
	return nil
}

// previousStations is the home station of each unit that is about to be dropped here from somewhere else.
func (s *ServicePointService) previousStations(ctx context.Context, pointID uuid.UUID, previous, next []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := map[uuid.UUID]uuid.UUID{}
	if s == nil || s.resources == nil {
		return out, nil
	}
	staying := map[uuid.UUID]struct{}{}
	for _, id := range previous {
		staying[id] = struct{}{}
	}
	for _, id := range next {
		if _, ok := staying[id]; ok {
			continue
		}
		resource, err := s.resources.FindByID(ctx, id)
		if err != nil {
			if repository.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if resource.ServicePointID == nil || *resource.ServicePointID == uuid.Nil || *resource.ServicePointID == pointID {
			continue
		}
		out[id] = *resource.ServicePointID
	}
	return out, nil
}

func (s *ServicePointService) recordStationChanges(ctx context.Context, point *models.ServicePoint, previous, next []uuid.UUID) error {
	if s == nil || s.logs == nil || point == nil {
		return nil
	}
	before := map[uuid.UUID]struct{}{}
	for _, id := range previous {
		before[id] = struct{}{}
	}
	after := map[uuid.UUID]struct{}{}
	for _, id := range next {
		after[id] = struct{}{}
	}
	for _, id := range next {
		if _, ok := before[id]; ok {
			continue
		}
		if err := s.logs.RecordUnitStation(ctx, id, point.ID, point.Name, true); err != nil {
			return err
		}
	}
	for _, id := range previous {
		if _, ok := after[id]; ok {
			continue
		}
		if err := s.logs.RecordUnitStation(ctx, id, point.ID, point.Name, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *ServicePointService) recordLeftStations(ctx context.Context, pointID uuid.UUID, moved map[uuid.UUID]uuid.UUID) error {
	if s == nil || s.logs == nil || len(moved) == 0 {
		return nil
	}
	names := map[uuid.UUID]string{}
	for resourceID, stationID := range moved {
		if stationID == pointID {
			continue
		}
		name, ok := names[stationID]
		if !ok {
			station, err := s.repo.FindByID(ctx, stationID)
			if err != nil {
				return err
			}
			name = station.Name
			names[stationID] = name
		}
		if err := s.logs.RecordUnitStation(ctx, resourceID, stationID, name, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *ServicePointService) Delete(ctx context.Context, id uuid.UUID, force bool) error {
	if force {
		current, findErr := s.repo.FindByID(ctx, id)
		if err := s.repo.AssignResources(ctx, id, nil); err != nil {
			return err
		}
		if findErr == nil && current != nil {
			if err := s.recordStationChanges(ctx, current, []uuid.UUID(current.ResourceIDs), nil); err != nil {
				return err
			}
		}
	}
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
	publishEvent(s.pub, models.SubjectServicePointDeleted, id, nil)
	return nil
}

func servicePointConflict(err error) error {
	switch repository.UniqueConstraint(err) {
	case "uq_service_point_department_radio":
		return Conflict("a service point with this radio id already exists in the department")
	default:
		return asDuplicateName(err, "a service point with this name already exists")
	}
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

func (s *ServicePointService) List(ctx context.Context, filter models.ServicePointListFilter) ([]models.ServicePointResponse, int64, error) {
	rows, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]models.ServicePointResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, total, nil
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
