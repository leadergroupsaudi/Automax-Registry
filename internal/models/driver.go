package models

import (
	"time"

	"github.com/google/uuid"
)

// Driver is a person stored in the registry and registered in Caguar.
type Driver struct {
	ID            uuid.UUID `gorm:"column:id;type:uuid;primaryKey"`
	Name          string    `gorm:"column:name;not null"`
	UniqueID      string    `gorm:"column:unique_id;not null"`
	PendingAction string    `gorm:"column:pending_action;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (Driver) TableName() string { return "driver" }
