package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResourceLocation is the dispatch department, type, and DSP-01 status of one resource.
type ResourceLocation struct {
	DepartmentID   uuid.UUID
	ResourceTypeID uuid.UUID
	Status         string
}

// ResourceLookup reads the rows FindNearestAvailable needs from Postgres.
type ResourceLookup interface {
	Location(ctx context.Context, id uuid.UUID) (*ResourceLocation, error)
	ActiveOperator(ctx context.Context, resourceID uuid.UUID) (*uuid.UUID, error)
	FixTargetByThing(ctx context.Context, thingID uuid.UUID) (*ResourceFixTarget, error)
}

// ResourceFixTarget is the resource and department that receive a live device fix.
type ResourceFixTarget struct {
	ResourceID   uuid.UUID
	DepartmentID uuid.UUID
}

type resourceLookup struct{ db *gorm.DB }

func NewResourceLookup(db *gorm.DB) ResourceLookup {
	return &resourceLookup{db: db}
}

type resourceLocationRow struct {
	ID             uuid.UUID `gorm:"column:id"`
	DepartmentID   uuid.UUID `gorm:"column:department_id"`
	ResourceTypeID uuid.UUID `gorm:"column:resource_type_id"`
	Status         string    `gorm:"column:status"`
}

func (r *resourceLookup) Location(ctx context.Context, id uuid.UUID) (*ResourceLocation, error) {
	var row resourceLocationRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.id, rt.department_id, r.resource_type_id, r.status
		FROM resource r
		JOIN resource_type rt ON rt.id = r.resource_type_id
		WHERE r.id = ?`, id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == uuid.Nil {
		return nil, nil
	}
	return &ResourceLocation{
		DepartmentID:   row.DepartmentID,
		ResourceTypeID: row.ResourceTypeID,
		Status:         row.Status,
	}, nil
}

type operatorRow struct {
	UserID uuid.UUID `gorm:"column:user_id"`
}

// ActiveOperator returns the user on an active resource_shift_assignment, or nil when the resource is off duty.
func (r *resourceLookup) ActiveOperator(ctx context.Context, resourceID uuid.UUID) (*uuid.UUID, error) {
	var rows []operatorRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT user_id
		FROM resource_shift_assignment
		WHERE resource_id = ? AND status = 'active'
		ORDER BY id
		LIMIT 1`, resourceID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || rows[0].UserID == uuid.Nil {
		return nil, nil
	}
	id := rows[0].UserID
	return &id, nil
}

type fixTargetRow struct {
	ResourceID   uuid.UUID `gorm:"column:id"`
	DepartmentID uuid.UUID `gorm:"column:department_id"`
}

func (r *resourceLookup) FixTargetByThing(ctx context.Context, thingID uuid.UUID) (*ResourceFixTarget, error) {
	var row fixTargetRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT r.id, rt.department_id
		FROM resource r
		JOIN resource_type rt ON rt.id = r.resource_type_id
		WHERE r.thing_id = ?`, thingID).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ResourceID == uuid.Nil {
		return nil, nil
	}
	return &ResourceFixTarget{ResourceID: row.ResourceID, DepartmentID: row.DepartmentID}, nil
}
