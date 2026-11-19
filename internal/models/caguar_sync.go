package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CaguarSyncCreate = "create"
	CaguarSyncUpdate = "update"
	CaguarSyncDelete = "delete"
)

// CaguarSyncJob is one device create, update, or delete waiting for Caguar.
type CaguarSyncJob struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ThingID   uuid.UUID `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	Action    string    `gorm:"column:action;type:varchar(16);not null" json:"action"`
	UniqueID  string    `gorm:"column:unique_id;type:varchar(255);not null;default:''" json:"unique_id"`
	Payload   JSON      `gorm:"column:payload;type:jsonb;not null" json:"payload"`
	Attempts  int       `gorm:"column:attempts;not null;default:0" json:"attempts"`
	LastError string    `gorm:"column:last_error;type:text;not null;default:''" json:"last_error"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (CaguarSyncJob) TableName() string { return "caguar_device_sync" }

func (j *CaguarSyncJob) BeforeCreate(_ *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	if len(j.Payload) == 0 {
		j.Payload = JSON(`{}`)
	}
	return nil
}
