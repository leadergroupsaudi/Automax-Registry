package services

import (
	"context"
	"log"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type FireStationAlertService struct {
	points repository.ServicePointRepository
	alerts repository.FireStationAlertRepository
}

func NewFireStationAlertService(points repository.ServicePointRepository, alerts repository.FireStationAlertRepository) *FireStationAlertService {
	return &FireStationAlertService{points: points, alerts: alerts}
}

func (s *FireStationAlertService) Alert(ctx context.Context, incidentID uuid.UUID, req models.FireStationAlertRequest) (*models.FireStationAlertResult, error) {
	if incidentID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"incident_id": "incident_id is required"})
	}
	if req.DepartmentID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"department_id": "department_id is required"})
	}
	return s.record(ctx, incidentID, req)
}

// AlertSelection records an alert for the stations the dispatcher checked.
// The create-incident screen alerts before the incident exists, so incident_id may be empty.
func (s *FireStationAlertService) AlertSelection(ctx context.Context, req models.FireStationAlertRequest) (*models.FireStationAlertResult, error) {
	incidentID := uuid.Nil
	if req.IncidentID != nil {
		incidentID = *req.IncidentID
	}
	if req.All && req.DepartmentID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"department_id": "department_id is required"})
	}
	return s.record(ctx, incidentID, req)
}

func (s *FireStationAlertService) List(ctx context.Context, incidentID uuid.UUID) ([]models.FireStationAlertResponse, error) {
	if incidentID == uuid.Nil {
		return nil, Invalid("validation failed", map[string]string{"incident_id": "incident_id is required"})
	}
	rows, err := s.alerts.ListByIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	out := make([]models.FireStationAlertResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Response())
	}
	return out, nil
}

func (s *FireStationAlertService) record(ctx context.Context, incidentID uuid.UUID, req models.FireStationAlertRequest) (*models.FireStationAlertResult, error) {
	stations, err := s.stations(ctx, req)
	if err != nil {
		return nil, err
	}
	var incident *uuid.UUID
	if incidentID != uuid.Nil {
		incident = &incidentID
	}
	rows := make([]models.FireStationAlert, 0, len(stations))
	ids := make([]uuid.UUID, 0, len(stations))
	for _, station := range stations {
		rows = append(rows, models.FireStationAlert{
			IncidentID:     incident,
			ServicePointID: station.ID,
			DepartmentID:   station.DepartmentID,
		})
		ids = append(ids, station.ID)
	}
	if incidentID != uuid.Nil {
		existing, err := s.alerts.ListSelected(ctx, incident, ids)
		if err != nil {
			return nil, err
		}
		already := map[uuid.UUID]struct{}{}
		for _, row := range existing {
			already[row.ServicePointID] = struct{}{}
		}
		fresh := make([]models.FireStationAlert, 0, len(rows))
		freshIDs := make([]uuid.UUID, 0, len(ids))
		for i, row := range rows {
			if _, ok := already[row.ServicePointID]; ok {
				continue
			}
			fresh = append(fresh, row)
			freshIDs = append(freshIDs, ids[i])
		}
		if len(fresh) == 0 {
			return nil, Conflict("incident is already assigned to this fire station")
		}
		rows = fresh
		ids = freshIDs
	}
	if err := s.alerts.Save(ctx, rows); err != nil {
		log.Printf("registry: fire station alert failed incident_id=%s err=%v", incidentID, err)
		return nil, err
	}
	stored, err := s.alerts.ListSelected(ctx, incident, ids)
	if err != nil {
		return nil, err
	}
	alerted := responsesFor(stored, stations)
	log.Printf("registry: fire stations alerted incident_id=%s department_id=%s count=%d", incidentID, req.DepartmentID, len(alerted))
	return &models.FireStationAlertResult{IncidentID: incident, Alerted: alerted}, nil
}

func (s *FireStationAlertService) stations(ctx context.Context, req models.FireStationAlertRequest) ([]models.ServicePoint, error) {
	if req.All {
		active := true
		rows, _, err := s.points.List(ctx, models.ServicePointListFilter{
			DepartmentID: &req.DepartmentID,
			Active:       &active,
		})
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, Invalid("validation failed", map[string]string{"department_id": "no active fire stations"})
		}
		return rows, nil
	}
	ids := make([]uuid.UUID, 0, len(req.ServicePointIDs))
	for _, id := range uniqueUUIDs(req.ServicePointIDs) {
		if id != uuid.Nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, Invalid("validation failed", map[string]string{"service_point_ids": "service_point_ids is required"})
	}
	fields := map[string]string{}
	stations := make([]models.ServicePoint, 0, len(ids))
	for _, id := range ids {
		row, err := s.points.FindByID(ctx, id)
		if repository.IsNotFound(err) {
			fields[id.String()] = "fire station not found"
			continue
		}
		if err != nil {
			return nil, err
		}
		if !row.IsActive {
			fields[id.String()] = "fire station is not active"
			continue
		}
		if req.DepartmentID != uuid.Nil && row.DepartmentID != req.DepartmentID {
			fields[id.String()] = "fire station is not in this department"
			continue
		}
		stations = append(stations, *row)
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	return stations, nil
}

func responsesFor(stored []models.FireStationAlert, requested []models.ServicePoint) []models.FireStationAlertResponse {
	want := map[uuid.UUID]struct{}{}
	for _, station := range requested {
		want[station.ID] = struct{}{}
	}
	out := make([]models.FireStationAlertResponse, 0, len(requested))
	for _, row := range stored {
		if _, ok := want[row.ServicePointID]; !ok {
			continue
		}
		out = append(out, row.Response())
	}
	return out
}
