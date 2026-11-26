package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"gorm.io/gorm"
)

type DispatchLogRepository interface {
	Create(ctx context.Context, row *models.DispatchLog) error
	List(ctx context.Context, filter models.DispatchLogFilter) ([]models.DispatchLog, error)
}

type dispatchLogRepository struct{ db *gorm.DB }

func NewDispatchLogRepository(db *gorm.DB) DispatchLogRepository {
	return &dispatchLogRepository{db: db}
}

func (r *dispatchLogRepository) Create(ctx context.Context, row *models.DispatchLog) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *dispatchLogRepository) List(ctx context.Context, filter models.DispatchLogFilter) ([]models.DispatchLog, error) {
	q := r.db.WithContext(ctx).Model(&models.DispatchLog{})
	if filter.IncidentID != nil {
		q = q.Where("incident_id = ?", *filter.IncidentID)
	}
	if filter.ResourceID != nil {
		q = q.Where("resource_id = ?", *filter.ResourceID)
	}
	if filter.AssignmentID != nil {
		q = q.Where("assignment_id = ?", *filter.AssignmentID)
	}
	if filter.FireStationID != nil {
		q = q.Where("fire_station_id = ?", *filter.FireStationID)
	}
	var rows []models.DispatchLog
	err := q.Order("created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}
