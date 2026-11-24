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
		item.FireStationID, item.FireStationName = pickFireStation(stations[row.IncidentID], homes[row.ResourceID])
		out = append(out, item)
	}
	return out, nil
}

func (s *DispatchLogService) stationsByIncident(ctx context.Context, rows []models.DispatchLog) map[uuid.UUID][]models.DispatchFireStation {
	out := map[uuid.UUID][]models.DispatchFireStation{}
	if s == nil || s.alerts == nil {
		return out
	}
	seen := map[uuid.UUID]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.IncidentID]; ok {
			continue
		}
		seen[row.IncidentID] = struct{}{}
		alerts, err := s.alerts.ListByIncident(ctx, row.IncidentID)
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
		out[row.IncidentID] = stations
	}
	return out
}

func (s *DispatchLogService) homeStationByResource(ctx context.Context, rows []models.DispatchLog) map[uuid.UUID]*uuid.UUID {
	out := map[uuid.UUID]*uuid.UUID{}
	if s == nil || s.resources == nil {
		return out
	}
	for _, row := range rows {
		if _, ok := out[row.ResourceID]; ok {
			continue
		}
		resource, err := s.resources.FindByID(ctx, row.ResourceID)
		if err != nil || resource.ServicePointID == nil || *resource.ServicePointID == uuid.Nil {
			out[row.ResourceID] = nil
			continue
		}
		out[row.ResourceID] = resource.ServicePointID
	}
	return out
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
