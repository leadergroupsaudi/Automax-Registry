package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	SubjectThingCreated = "registry.thing.created"
	SubjectThingUpdated = "registry.thing.updated"
	SubjectThingDeleted = "registry.thing.deleted"
)

// ThingEvent is the payload REG-08's provisioning bridge consumes.
type ThingEvent struct {
	Event        string     `json:"event"`
	ThingID      uuid.UUID  `json:"thing_id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	IsTrackable  bool       `json:"is_trackable"`
	DepartmentID uuid.UUID  `json:"department_id"`
	GroupID      *uuid.UUID `json:"group_id"`
	OccurredAt   time.Time  `json:"occurred_at"`
}

type Thing struct {
	ID           uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	Name         string     `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Type         string     `gorm:"column:type;type:varchar(50);not null" json:"type"`
	IsTrackable  bool       `gorm:"column:is_trackable;not null;default:false" json:"is_trackable"`
	DepartmentID uuid.UUID  `gorm:"column:department_id;type:uuid;not null" json:"department_id"`
	GroupID      *uuid.UUID `gorm:"column:group_id;type:uuid" json:"group_id"`
	Metadata     JSON       `gorm:"column:metadata;type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Thing) TableName() string { return "things" }

func (t *Thing) BeforeCreate(_ *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

func (t *Thing) BeforeSave(_ *gorm.DB) error {
	if len(t.Metadata) == 0 {
		t.Metadata = JSON(`{}`)
	}
	return nil
}

func (t Thing) Event(subject string, at time.Time) ThingEvent {
	return ThingEvent{
		Event:        subject,
		ThingID:      t.ID,
		Name:         t.Name,
		Type:         t.Type,
		IsTrackable:  t.IsTrackable,
		DepartmentID: t.DepartmentID,
		GroupID:      t.GroupID,
		OccurredAt:   at,
	}
}

type ThingResponse struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	IsTrackable  bool       `json:"is_trackable"`
	DepartmentID uuid.UUID  `json:"department_id"`
	GroupID      *uuid.UUID `json:"group_id"`
	Metadata     JSON       `json:"metadata"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (t Thing) Response() ThingResponse {
	meta := t.Metadata
	if len(meta) == 0 {
		meta = JSON(`{}`)
	}
	return ThingResponse{
		ID:           t.ID,
		Name:         t.Name,
		Type:         t.Type,
		IsTrackable:  t.IsTrackable,
		DepartmentID: t.DepartmentID,
		GroupID:      t.GroupID,
		Metadata:     meta,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

type ThingCreateRequest struct {
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	IsTrackable  bool       `json:"is_trackable"`
	DepartmentID uuid.UUID  `json:"department_id"`
	GroupID      *uuid.UUID `json:"group_id"`
	Metadata     JSON       `json:"metadata"`
}

type ThingUpdateRequest struct {
	Name         string     `json:"name"`
	Type         string     `json:"type"`
	IsTrackable  bool       `json:"is_trackable"`
	DepartmentID uuid.UUID  `json:"department_id"`
	GroupID      *uuid.UUID `json:"group_id"`
	Metadata     JSON       `json:"metadata"`
}

type ThingListFilter struct {
	Type         string
	DepartmentID *uuid.UUID
	GroupID      *uuid.UUID
}

type Channel struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	Name      string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Channel) TableName() string { return "channels" }

func (c *Channel) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type ChannelResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c Channel) Response() ChannelResponse {
	return ChannelResponse{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

type ChannelWriteRequest struct {
	Name string `json:"name"`
}

type ChannelACL struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ChannelID    uuid.UUID `gorm:"column:channel_id;type:uuid;not null" json:"channel_id"`
	ThingID      uuid.UUID `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	CanPublish   bool      `gorm:"column:can_publish;not null;default:false" json:"can_publish"`
	CanSubscribe bool      `gorm:"column:can_subscribe;not null;default:false" json:"can_subscribe"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
}

func (ChannelACL) TableName() string { return "channel_acl" }

func (a *ChannelACL) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type ChannelACLResponse struct {
	ID           uuid.UUID `json:"id"`
	ChannelID    uuid.UUID `json:"channel_id"`
	ThingID      uuid.UUID `json:"thing_id"`
	CanPublish   bool      `json:"can_publish"`
	CanSubscribe bool      `json:"can_subscribe"`
	CreatedAt    time.Time `json:"created_at"`
}

func (a ChannelACL) Response() ChannelACLResponse {
	return ChannelACLResponse{
		ID:           a.ID,
		ChannelID:    a.ChannelID,
		ThingID:      a.ThingID,
		CanPublish:   a.CanPublish,
		CanSubscribe: a.CanSubscribe,
		CreatedAt:    a.CreatedAt,
	}
}

type ChannelACLCreateRequest struct {
	ThingID      uuid.UUID `json:"thing_id"`
	CanPublish   bool      `json:"can_publish"`
	CanSubscribe bool      `json:"can_subscribe"`
}

type ChannelACLUpdateRequest struct {
	CanPublish   bool `json:"can_publish"`
	CanSubscribe bool `json:"can_subscribe"`
}

type Group struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	Name          string     `gorm:"column:name;type:varchar(255);not null" json:"name"`
	ParentGroupID *uuid.UUID `gorm:"column:parent_group_id;type:uuid" json:"parent_group_id"`
	DepartmentID  uuid.UUID  `gorm:"column:department_id;type:uuid;not null" json:"department_id"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Group) TableName() string { return "groups" }

func (g *Group) BeforeCreate(_ *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

type GroupResponse struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	ParentGroupID *uuid.UUID `json:"parent_group_id"`
	DepartmentID  uuid.UUID  `json:"department_id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (g Group) Response() GroupResponse {
	return GroupResponse{
		ID:            g.ID,
		Name:          g.Name,
		ParentGroupID: g.ParentGroupID,
		DepartmentID:  g.DepartmentID,
		CreatedAt:     g.CreatedAt,
		UpdatedAt:     g.UpdatedAt,
	}
}

type GroupWriteRequest struct {
	Name          string     `json:"name"`
	ParentGroupID *uuid.UUID `json:"parent_group_id"`
	DepartmentID  uuid.UUID  `json:"department_id"`
}

type GroupListFilter struct {
	ParentGroupID *uuid.UUID
	RootsOnly     bool
}

// DeviceCredential is the stored credential. APISecretHash is omitted from JSON
// and redacted in logs. The plaintext secret is returned only by the issue
// response, which uses CredentialIssued and never this struct.
type DeviceCredential struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ThingID       uuid.UUID  `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	APIKey        string     `gorm:"column:api_key;type:varchar(64);not null" json:"api_key"`
	APISecretHash SecretHash `gorm:"column:api_secret_hash;type:varchar(255);not null" json:"-"`
	Status        string     `gorm:"column:status;type:varchar(20);not null;default:active" json:"status"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	RevokedAt     *time.Time `gorm:"column:revoked_at" json:"revoked_at"`
}

func (DeviceCredential) TableName() string { return "device_credentials" }

func (d DeviceCredential) String() string {
	return "DeviceCredential{id=" + d.ID.String() + " thing_id=" + d.ThingID.String() + " status=" + d.Status + "}"
}

func (d DeviceCredential) GoString() string { return d.String() }

func (d *DeviceCredential) BeforeCreate(_ *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.Status == "" {
		d.Status = "active"
	}
	return nil
}

// CredentialIssued is the one response that contains the plaintext secret.
type CredentialIssued struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

// CredentialListItem is a stored credential without the secret or its hash.
type CredentialListItem struct {
	ID        uuid.UUID  `json:"id"`
	APIKey    string     `json:"api_key"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

// ExternalSystemTraccar is the Caugar tracer-go system name stored on external refs.
const ExternalSystemTraccar = "traccar-go"

// ThingExternalRef maps a Thing to an id in an external system (REG-08 / DSP-05).
// Routes for this table belong to REG-13.
type ThingExternalRef struct {
	ID             uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ThingID        uuid.UUID `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	ExternalSystem string    `gorm:"column:external_system;type:varchar(50);not null" json:"external_system"`
	ExternalID     string    `gorm:"column:external_id;type:varchar(255);not null" json:"external_id"`
	SyncedAt       time.Time `gorm:"column:synced_at;not null;autoCreateTime" json:"synced_at"`
}

func (ThingExternalRef) TableName() string { return "thing_external_refs" }

func (r *ThingExternalRef) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
