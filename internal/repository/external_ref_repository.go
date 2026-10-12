package repository

import (
	"context"
	"errors"
	"time"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrAmbiguousExternalRef means one Caugar device id is linked to more than one thing.
var ErrAmbiguousExternalRef = errors.New("ambiguous external ref")

// LinkedThing is the registry thing behind a Caugar device id.
type LinkedThing struct {
	ThingID     uuid.UUID
	IsTrackable bool
}

type ExternalRefRepository interface {
	FindLinkedThing(ctx context.Context, system, externalID string) (LinkedThing, error)
	FindByThingSystem(ctx context.Context, thingID uuid.UUID, system string) (*models.ThingExternalRef, error)
	CountOtherOwners(ctx context.Context, system, externalID string, thingID uuid.UUID) (int64, error)
	Create(ctx context.Context, ref *models.ThingExternalRef) error
	UpdateExternalID(ctx context.Context, id uuid.UUID, externalID string, syncedAt time.Time) error
}

type externalRefRepository struct{ db *gorm.DB }

func NewExternalRefRepository(db *gorm.DB) ExternalRefRepository {
	return &externalRefRepository{db: db}
}

type linkedRow struct {
	ThingID     uuid.UUID `gorm:"column:thing_id"`
	IsTrackable bool      `gorm:"column:is_trackable"`
}

func (r *externalRefRepository) FindLinkedThing(ctx context.Context, system, externalID string) (LinkedThing, error) {
	var rows []linkedRow
	err := r.db.WithContext(ctx).
		Table("thing_external_refs AS r").
		Select("r.thing_id, t.is_trackable").
		Joins("JOIN things t ON t.id = r.thing_id").
		Where("r.external_system = ? AND r.external_id = ?", system, externalID).
		Scan(&rows).Error
	if err != nil {
		return LinkedThing{}, err
	}
	switch len(rows) {
	case 0:
		return LinkedThing{}, gorm.ErrRecordNotFound
	case 1:
		return LinkedThing{ThingID: rows[0].ThingID, IsTrackable: rows[0].IsTrackable}, nil
	default:
		return LinkedThing{}, ErrAmbiguousExternalRef
	}
}

func (r *externalRefRepository) FindByThingSystem(ctx context.Context, thingID uuid.UUID, system string) (*models.ThingExternalRef, error) {
	var ref models.ThingExternalRef
	err := r.db.WithContext(ctx).
		Where("thing_id = ? AND external_system = ?", thingID, system).
		First(&ref).Error
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

func (r *externalRefRepository) CountOtherOwners(ctx context.Context, system, externalID string, thingID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.ThingExternalRef{}).
		Where("external_system = ? AND external_id = ? AND thing_id <> ?", system, externalID, thingID).
		Count(&n).Error
	return n, err
}

func (r *externalRefRepository) Create(ctx context.Context, ref *models.ThingExternalRef) error {
	return r.db.WithContext(ctx).Create(ref).Error
}

func (r *externalRefRepository) UpdateExternalID(ctx context.Context, id uuid.UUID, externalID string, syncedAt time.Time) error {
	res := r.db.WithContext(ctx).Model(&models.ThingExternalRef{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"external_id": externalID,
			"synced_at":   syncedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
