package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ResourceAvailable   = "available"
	ResourceOffered     = "offered"
	ResourceDispatched  = "dispatched"
	ResourceMaintenance = "maintenance"

	AssignmentScheduled = "scheduled"
	AssignmentActive    = "active"
	AssignmentCompleted = "completed"
	AssignmentCancelled = "cancelled"
)

type ResourceType struct {
	ID                       uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	DepartmentID             uuid.UUID `gorm:"column:department_id;type:uuid;not null" json:"department_id"`
	Name                     string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	AcceptanceTimeoutSeconds int       `gorm:"column:acceptance_timeout_seconds;not null;default:120" json:"acceptance_timeout_seconds"`
	CreatedAt                time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt                time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (ResourceType) TableName() string { return "resource_type" }

func (r *ResourceType) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type ResourceTypeResponse struct {
	ID                       uuid.UUID `json:"id"`
	DepartmentID             uuid.UUID `json:"department_id"`
	Name                     string    `json:"name"`
	AcceptanceTimeoutSeconds int       `json:"acceptance_timeout_seconds"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (r ResourceType) Response() ResourceTypeResponse {
	return ResourceTypeResponse{
		ID:                       r.ID,
		DepartmentID:             r.DepartmentID,
		Name:                     r.Name,
		AcceptanceTimeoutSeconds: r.AcceptanceTimeoutSeconds,
		CreatedAt:                r.CreatedAt,
		UpdatedAt:                r.UpdatedAt,
	}
}

type ResourceTypeWriteRequest struct {
	DepartmentID             uuid.UUID `json:"department_id"`
	Name                     string    `json:"name"`
	AcceptanceTimeoutSeconds *int      `json:"acceptance_timeout_seconds"`
}

type ResourceTypeListFilter struct {
	DepartmentID *uuid.UUID
}

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type LocationInput struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

type ServicePoint struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	DepartmentID uuid.UUID `json:"department_id"`
	Latitude     float64   `json:"-"`
	Longitude    float64   `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ServicePointResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	DepartmentID uuid.UUID `json:"department_id"`
	Location     Location  `json:"location"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (p ServicePoint) Response() ServicePointResponse {
	return ServicePointResponse{
		ID:           p.ID,
		Name:         p.Name,
		DepartmentID: p.DepartmentID,
		Location:     Location{Lat: p.Latitude, Lng: p.Longitude},
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

type ServicePointWriteRequest struct {
	Name         string         `json:"name"`
	DepartmentID uuid.UUID      `json:"department_id"`
	Location     *LocationInput `json:"location"`
}

type Shift struct {
	ID           uuid.UUID `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	DepartmentID uuid.UUID `gorm:"column:department_id;type:uuid;not null" json:"department_id"`
	Name         string    `gorm:"column:name;type:varchar(100);not null" json:"name"`
	StartTime    string    `gorm:"column:start_time;type:time;not null" json:"start_time"`
	EndTime      string    `gorm:"column:end_time;type:time;not null" json:"end_time"`
	DaysOfWeek   string    `gorm:"column:days_of_week;type:varchar(20);not null" json:"days_of_week"`
	IsActive     bool      `gorm:"column:is_active;not null;default:true" json:"is_active"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Shift) TableName() string { return "shift" }

func (s *Shift) BeforeCreate(_ *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

type ShiftResponse struct {
	ID           uuid.UUID `json:"id"`
	DepartmentID uuid.UUID `json:"department_id"`
	Name         string    `json:"name"`
	StartTime    string    `json:"start_time"`
	EndTime      string    `json:"end_time"`
	DaysOfWeek   string    `json:"days_of_week"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s Shift) Response() ShiftResponse {
	return ShiftResponse{
		ID:           s.ID,
		DepartmentID: s.DepartmentID,
		Name:         s.Name,
		StartTime:    s.StartTime,
		EndTime:      s.EndTime,
		DaysOfWeek:   s.DaysOfWeek,
		IsActive:     s.IsActive,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}

type ShiftWriteRequest struct {
	DepartmentID uuid.UUID `json:"department_id"`
	Name         string    `json:"name"`
	StartTime    string    `json:"start_time"`
	EndTime      string    `json:"end_time"`
	DaysOfWeek   string    `json:"days_of_week"`
	IsActive     *bool     `json:"is_active"`
}

// Resource is one registry thing used by dispatch. ID is the resource row.
// ThingID is that thing, and the things table is updated with the same type,
// station, and status.
type Resource struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ThingID        uuid.UUID  `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	Name           string     `gorm:"-" json:"name"`
	ResourceTypeID uuid.UUID  `gorm:"column:resource_type_id;type:uuid;not null" json:"resource_type_id"`
	ServicePointID *uuid.UUID `gorm:"column:service_point_id;type:uuid" json:"service_point_id"`
	Status         string     `gorm:"column:status;type:varchar(20);not null;default:available" json:"status"`
	Latitude       *float64   `gorm:"-" json:"latitude"`
	Longitude      *float64   `gorm:"-" json:"longitude"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Resource) TableName() string { return "resource" }

func (r *Resource) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type ResourceResponse struct {
	ID             uuid.UUID  `json:"id"`
	ThingID        uuid.UUID  `json:"thing_id"`
	Name           string     `json:"name"`
	ResourceTypeID uuid.UUID  `json:"resource_type_id"`
	ServicePointID *uuid.UUID `json:"service_point_id"`
	Status         string     `json:"status"`
	Latitude       *float64   `json:"latitude"`
	Longitude      *float64   `json:"longitude"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (r Resource) Response() ResourceResponse {
	return ResourceResponse{
		ID:             r.ID,
		ThingID:        r.ThingID,
		Name:           r.Name,
		ResourceTypeID: r.ResourceTypeID,
		ServicePointID: r.ServicePointID,
		Status:         r.Status,
		Latitude:       r.Latitude,
		Longitude:      r.Longitude,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

type ResourceWriteRequest struct {
	ThingID        uuid.UUID  `json:"thing_id"`
	ResourceTypeID uuid.UUID  `json:"resource_type_id"`
	ServicePointID *uuid.UUID `json:"service_point_id"`
	Status         string     `json:"status"`
}

type ResourceListFilter struct {
	ResourceTypeID *uuid.UUID
	ServicePointID *uuid.UUID
	Status         string
}

// GeoCandidate is one member of a department GEO set, with distance in meters.
type GeoCandidate struct {
	ResourceID uuid.UUID
	DistanceM  float64
}

// NearestServicePoint is one station ordered by geographic distance.
type NearestServicePoint struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	DepartmentID   uuid.UUID `json:"department_id"`
	Location       Location  `json:"location"`
	DistanceMeters float64   `json:"distance_meters"`
}

// NearestResource is one on-duty, available resource returned by FindNearestAvailable.
type NearestResource struct {
	ResourceID     uuid.UUID `json:"resource_id"`
	ResourceTypeID uuid.UUID `json:"resource_type_id"`
	OperatorID     uuid.UUID `json:"operator_id"`
	Status         string    `json:"status"`
	DistanceMeters float64   `json:"distance_meters"`
}

type ResourceShiftAssignment struct {
	ID         uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ResourceID uuid.UUID  `gorm:"column:resource_id;type:uuid;not null" json:"resource_id"`
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	ShiftID    *uuid.UUID `gorm:"column:shift_id;type:uuid" json:"shift_id"`
	StartAt    *time.Time `gorm:"column:start_at" json:"start_at"`
	EndAt      *time.Time `gorm:"column:end_at" json:"end_at"`
	Status     string     `gorm:"column:status;type:varchar(20);not null;default:scheduled" json:"status"`
}

func (ResourceShiftAssignment) TableName() string { return "resource_shift_assignment" }

func (a *ResourceShiftAssignment) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type AssignmentResponse struct {
	ID         uuid.UUID  `json:"id"`
	ResourceID uuid.UUID  `json:"resource_id"`
	UserID     uuid.UUID  `json:"user_id"`
	ShiftID    *uuid.UUID `json:"shift_id"`
	StartAt    *time.Time `json:"start_at"`
	EndAt      *time.Time `json:"end_at"`
	Status     string     `json:"status"`
}

func (a ResourceShiftAssignment) Response() AssignmentResponse {
	return AssignmentResponse{
		ID:         a.ID,
		ResourceID: a.ResourceID,
		UserID:     a.UserID,
		ShiftID:    a.ShiftID,
		StartAt:    a.StartAt,
		EndAt:      a.EndAt,
		Status:     a.Status,
	}
}

type AssignmentWriteRequest struct {
	ResourceID uuid.UUID  `json:"resource_id"`
	UserID     uuid.UUID  `json:"user_id"`
	ShiftID    *uuid.UUID `json:"shift_id"`
	StartAt    *time.Time `json:"start_at"`
	EndAt      *time.Time `json:"end_at"`
	Status     string     `json:"status"`
}

// Occupancy is one scheduled or active assignment, with its shift clock when it has one.
type Occupancy struct {
	ID         uuid.UUID
	ShiftID    *uuid.UUID
	StartAt    *time.Time
	EndAt      *time.Time
	DaysOfWeek string
	StartTime  string
	EndTime    string
}
