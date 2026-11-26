package services

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type DispatchLogService struct {
	repo      repository.DispatchLogRepository
	alerts    repository.FireStationAlertRepository
	resources repository.ResourceRepository
}

func NewDispatchLogService(repo repository.DispatchLogRepository) *DispatchLogService {
	return &DispatchLogService{repo: repo}
}

func (s *DispatchLogService) UseFireStations(alerts repository.FireStationAlertRepository, resources repository.ResourceRepository) {
	if s == nil {
		return
	}
	s.alerts = alerts
	s.resources = resources
}

func (s *DispatchLogService) List(ctx context.Context, filter models.DispatchLogFilter) ([]models.DispatchLogResponse, error) {
	rows, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	stations := s.stationsByIncident(ctx, rows)
	homes := s.homeStationByResource(ctx, rows)
	out := make([]models.DispatchLogResponse, 0, len(rows))
	for _, row := range rows {
		item := row.Response()
		if item.FireStationID == nil {
			item.FireStationID, item.FireStationName = pickFireStation(stations[logID(row.IncidentID)], homes[logID(row.ResourceID)])
		}
		out = append(out, item)
	}
	return out, nil
}

// RecordUnitStation writes the drag of a unit onto or off a fire station.
func (s *DispatchLogService) RecordUnitStation(ctx context.Context, resourceID, stationID uuid.UUID, stationName string, assigned bool) error {
	if s == nil || s.repo == nil || resourceID == uuid.Nil || stationID == uuid.Nil {
		return nil
	}
	action := models.DispatchLogStationUnassigned
	if assigned {
		action = models.DispatchLogStationAssigned
	}
	resource := resourceID
	station := stationID
	return s.repo.Create(ctx, &models.DispatchLog{
		ResourceID:      &resource,
		FireStationID:   &station,
		FireStationName: textPtr(stationName),
		Action:          action,
	})
}

// RecordIncidentStation writes that an incident was assigned to a fire station.
func (s *DispatchLogService) RecordIncidentStation(ctx context.Context, incidentID, stationID uuid.UUID, stationName string) error {
	if s == nil || s.repo == nil || incidentID == uuid.Nil || stationID == uuid.Nil {
		return nil
	}
	incident := incidentID
	station := stationID
	return s.repo.Create(ctx, &models.DispatchLog{
		IncidentID:      &incident,
		FireStationID:   &station,
		FireStationName: textPtr(stationName),
		Action:          models.DispatchLogIncidentAssigned,
	})
}

func (s *DispatchLogService) stationsByIncident(ctx context.Context, rows []models.DispatchLog) map[uuid.UUID][]models.DispatchFireStation {
	out := map[uuid.UUID][]models.DispatchFireStation{}
	if s == nil || s.alerts == nil {
		return out
	}
	seen := map[uuid.UUID]struct{}{}
	for _, row := range rows {
		incidentID := logID(row.IncidentID)
		if incidentID == uuid.Nil {
			continue
		}
		if _, ok := seen[incidentID]; ok {
			continue
		}
		seen[incidentID] = struct{}{}
		alerts, err := s.alerts.ListByIncident(ctx, incidentID)
		if err != nil {
			continue
		}
		stations := make([]models.DispatchFireStation, 0, len(alerts))
		for _, alert := range alerts {
			stations = append(stations, models.DispatchFireStation{
				ID:   alert.ServicePointID,
				Name: alert.Name,
			})
		}
		out[incidentID] = stations
	}
	return out
}

func (s *DispatchLogService) homeStationByResource(ctx context.Context, rows []models.DispatchLog) map[uuid.UUID]*uuid.UUID {
	out := map[uuid.UUID]*uuid.UUID{}
	if s == nil || s.resources == nil {
		return out
	}
	for _, row := range rows {
		resourceID := logID(row.ResourceID)
		if resourceID == uuid.Nil {
			continue
		}
		if _, ok := out[resourceID]; ok {
			continue
		}
		resource, err := s.resources.FindByID(ctx, resourceID)
		if err != nil || resource.ServicePointID == nil || *resource.ServicePointID == uuid.Nil {
			out[resourceID] = nil
			continue
		}
		out[resourceID] = resource.ServicePointID
	}
	return out
}

func logID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

func pickFireStation(alerted []models.DispatchFireStation, home *uuid.UUID) (*uuid.UUID, *string) {
	if home != nil {
		for _, station := range alerted {
			if station.ID == *home {
				name := station.Name
				id := station.ID
				return &id, &name
			}
		}
	}
	if len(alerted) == 1 {
		name := alerted[0].Name
		id := alerted[0].ID
		return &id, &name
	}
	return nil, nil
}
