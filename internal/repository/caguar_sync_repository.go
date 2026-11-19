package repository

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CaguarSyncRepository interface {
	Enqueue(ctx context.Context, job *models.CaguarSyncJob) error
	List(ctx context.Context) ([]models.CaguarSyncJob, error)
	Delete(ctx context.Context, id uuid.UUID) error
	MarkError(ctx context.Context, id uuid.UUID, message string) error
}

type caguarSyncRepository struct{ db *gorm.DB }

func NewCaguarSyncRepository(db *gorm.DB) CaguarSyncRepository {
	return &caguarSyncRepository{db: db}
}

func (r *caguarSyncRepository) Enqueue(ctx context.Context, job *models.CaguarSyncJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if len(job.Payload) == 0 {
		job.Payload = models.JSON(`{}`)
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "thing_id"}, {Name: "action"}},
		DoUpdates: clause.Assignments(map[string]any{
			"unique_id":  job.UniqueID,
			"payload":    job.Payload,
			"attempts":   0,
			"last_error": "",
			"updated_at": gorm.Expr("now()"),
		}),
	}).Create(job).Error
}

func (r *caguarSyncRepository) List(ctx context.Context) ([]models.CaguarSyncJob, error) {
	var rows []models.CaguarSyncJob
	err := r.db.WithContext(ctx).Order("created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *caguarSyncRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.CaguarSyncJob{}, "id = ?", id).Error
}

func (r *caguarSyncRepository) MarkError(ctx context.Context, id uuid.UUID, message string) error {
	return r.db.WithContext(ctx).Model(&models.CaguarSyncJob{}).Where("id = ?", id).Updates(map[string]any{
		"attempts":   gorm.Expr("attempts + 1"),
		"last_error": message,
		"updated_at": gorm.Expr("now()"),
	}).Error
}
