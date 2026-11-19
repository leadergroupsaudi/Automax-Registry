package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FireStationAlertRepository interface {
	Save(ctx context.Context, rows []models.FireStationAlert) error
	ListByIncident(ctx context.Context, incidentID uuid.UUID) ([]models.FireStationAlert, error)
	ListSelected(ctx context.Context, incidentID *uuid.UUID, stationIDs []uuid.UUID) ([]models.FireStationAlert, error)
}

type fireStationAlertRepository struct{ db *gorm.DB }

func NewFireStationAlertRepository(db *gorm.DB) FireStationAlertRepository {
	return &fireStationAlertRepository{db: db}
}

func (r *fireStationAlertRepository) Save(ctx context.Context, rows []models.FireStationAlert) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			q := tx.Model(&models.FireStationAlert{}).Where("service_point_id = ?", rows[i].ServicePointID)
			if rows[i].IncidentID == nil {
				q = q.Where("incident_id IS NULL")
			} else {
				q = q.Where("incident_id = ?", *rows[i].IncidentID)
			}
			var existing int64
			if err := q.Count(&existing).Error; err != nil {
				return err
			}
			if existing > 0 {
				continue
			}
			if rows[i].ID == uuid.Nil {
				rows[i].ID = uuid.New()
			}
			if err := tx.Omit("Name", "RadioID").Create(&rows[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *fireStationAlertRepository) ListByIncident(ctx context.Context, incidentID uuid.UUID) ([]models.FireStationAlert, error) {
	var rows []models.FireStationAlert
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id, a.incident_id, a.service_point_id, a.department_id, a.created_at,
		       s.name AS name, s.radio_id AS radio_id
		FROM fire_station_alert a
		JOIN service_point s ON s.id = a.service_point_id
		WHERE a.incident_id = ?
		ORDER BY s.name ASC, a.service_point_id ASC`, incidentID).Scan(&rows).Error
	return rows, err
}

func (r *fireStationAlertRepository) ListSelected(ctx context.Context, incidentID *uuid.UUID, stationIDs []uuid.UUID) ([]models.FireStationAlert, error) {
	if len(stationIDs) == 0 {
		return []models.FireStationAlert{}, nil
	}
	var rows []models.FireStationAlert
	err := r.db.WithContext(ctx).Raw(`
		SELECT a.id, a.incident_id, a.service_point_id, a.department_id, a.created_at,
		       s.name AS name, s.radio_id AS radio_id
		FROM fire_station_alert a
		JOIN service_point s ON s.id = a.service_point_id
		WHERE a.service_point_id IN ?
		  AND ((?::uuid IS NULL AND a.incident_id IS NULL) OR a.incident_id = ?::uuid)
		ORDER BY s.name ASC, a.service_point_id ASC`, stationIDs, incidentID, incidentID).Scan(&rows).Error
	return rows, err
}
