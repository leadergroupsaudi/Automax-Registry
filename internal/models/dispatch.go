package models

import (
	"encoding/json"
	"fmt"
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

	SubjectResourceTypeCreated = "registry.dispatch.resource_type.created"
	SubjectResourceTypeUpdated = "registry.dispatch.resource_type.updated"
	SubjectResourceTypeDeleted = "registry.dispatch.resource_type.deleted"
	SubjectResourceCreated     = "registry.dispatch.resource.created"
	SubjectResourceUpdated     = "registry.dispatch.resource.updated"
	SubjectResourceDeleted     = "registry.dispatch.resource.deleted"
	SubjectServicePointCreated = "registry.dispatch.service_point.created"
	SubjectServicePointUpdated = "registry.dispatch.service_point.updated"
	SubjectServicePointDeleted = "registry.dispatch.service_point.deleted"
)

// AttributeColors are the unit-type map colors from the unit type screen.
type AttributeColors struct {
	Available     string `json:"available"`
	Busy          string `json:"busy"`
	MovingVehicle string `json:"moving_vehicle"`
	Offline       string `json:"offline"`
	IgnitionOn    string `json:"ignition_on"`
	IgnitionOff   string `json:"ignition_off"`
	OverSpeed     string `json:"over_speed"`
	StopOver20Min string `json:"stop_over_20_min"`
}

func DefaultAttributeColors() AttributeColors {
	return AttributeColors{
		Available:     "#22A45D",
		Busy:          "#E23B3B",
		MovingVehicle: "#E0A100",
		Offline:       "#6B7280",
		IgnitionOn:    "#7C3AED",
		IgnitionOff:   "#2563EB",
		OverSpeed:     "#D946EF",
		StopOver20Min: "#4C1D95",
	}
}

func (c AttributeColors) Filled() AttributeColors {
	base := DefaultAttributeColors()
	if c.Available != "" {
		base.Available = c.Available
	}
	if c.Busy != "" {
		base.Busy = c.Busy
	}
	if c.MovingVehicle != "" {
		base.MovingVehicle = c.MovingVehicle
	}
	if c.Offline != "" {
		base.Offline = c.Offline
	}
	if c.IgnitionOn != "" {
		base.IgnitionOn = c.IgnitionOn
	}
	if c.IgnitionOff != "" {
		base.IgnitionOff = c.IgnitionOff
	}
	if c.OverSpeed != "" {
		base.OverSpeed = c.OverSpeed
	}
	if c.StopOver20Min != "" {
		base.StopOver20Min = c.StopOver20Min
	}
	return base
}

// UUIDList scans a JSON array of UUIDs from json_agg.
type UUIDList []uuid.UUID

func (u *UUIDList) Scan(src any) error {
	if u == nil {
		return fmt.Errorf("nil UUIDList")
	}
	var raw []byte
	switch v := src.(type) {
	case nil:
		*u = nil
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into UUIDList", src)
	}
	if len(raw) == 0 || string(raw) == "null" {
		*u = nil
		return nil
	}
	var ids []uuid.UUID
	if err := json.Unmarshal(raw, &ids); err != nil {
		return err
	}
	*u = ids
	return nil
}

type ResourceType struct {
	ID                       uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	DepartmentID             uuid.UUID  `gorm:"column:department_id;type:uuid;not null" json:"department_id"`
	Name                     string     `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Code                     string     `gorm:"column:code;type:varchar(64);not null;default:''" json:"code"`
	AltName                  string     `gorm:"column:alt_name;type:varchar(255);not null;default:''" json:"alt_name"`
	AcceptanceTimeoutSeconds int        `gorm:"column:acceptance_timeout_seconds;not null;default:120" json:"acceptance_timeout_seconds"`
	MinSpeed                 int        `gorm:"column:min_speed;not null;default:0" json:"min_speed"`
	MaxSpeed                 int        `gorm:"column:max_speed;not null;default:0" json:"max_speed"`
	MinWeight                int        `gorm:"column:min_weight;not null;default:0" json:"min_weight"`
	MaxWeight                int        `gorm:"column:max_weight;not null;default:0" json:"max_weight"`
	ActiveTillDate           *time.Time `gorm:"column:active_till_date;type:date" json:"-"`
	IsActive                 bool       `gorm:"column:is_active;not null;default:true" json:"is_active"`
	Icon                     string     `gorm:"column:icon;type:varchar(2048);not null;default:''" json:"icon"`
	Colors                   JSON       `gorm:"column:colors;type:jsonb;not null" json:"-"`
	CreatedAt                time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (ResourceType) TableName() string { return "resource_type" }

func (r *ResourceType) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type ResourceTypeResponse struct {
	ID                       uuid.UUID       `json:"id"`
	DepartmentID             uuid.UUID       `json:"department_id"`
	Name                     string          `json:"name"`
	Code                     string          `json:"code"`
	AltName                  string          `json:"alt_name"`
	AcceptanceTimeoutSeconds int             `json:"acceptance_timeout_seconds"`
	MinSpeed                 int             `json:"min_speed"`
	MaxSpeed                 int             `json:"max_speed"`
	MinWeight                int             `json:"min_weight"`
	MaxWeight                int             `json:"max_weight"`
	ActiveTillDate           *string         `json:"active_till_date"`
	IsActive                 bool            `json:"is_active"`
	Icon                     string          `json:"icon"`
	Colors                   AttributeColors `json:"colors"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

func (r ResourceType) Response() ResourceTypeResponse {
	return ResourceTypeResponse{
		ID:                       r.ID,
		DepartmentID:             r.DepartmentID,
		Name:                     r.Name,
		Code:                     r.Code,
		AltName:                  r.AltName,
		AcceptanceTimeoutSeconds: r.AcceptanceTimeoutSeconds,
		MinSpeed:                 r.MinSpeed,
		MaxSpeed:                 r.MaxSpeed,
		MinWeight:                r.MinWeight,
		MaxWeight:                r.MaxWeight,
		ActiveTillDate:           formatDate(r.ActiveTillDate),
		IsActive:                 r.IsActive,
		Icon:                     r.Icon,
		Colors:                   decodeColors(r.Colors),
		CreatedAt:                r.CreatedAt,
		UpdatedAt:                r.UpdatedAt,
	}
}

type ResourceTypeWriteRequest struct {
	DepartmentID             uuid.UUID        `json:"department_id"`
	Name                     string           `json:"name"`
	Code                     *string          `json:"code"`
	AltName                  *string          `json:"alt_name"`
	AcceptanceTimeoutSeconds *int             `json:"acceptance_timeout_seconds"`
	MinSpeed                 *int             `json:"min_speed"`
	MaxSpeed                 *int             `json:"max_speed"`
	MinWeight                *int             `json:"min_weight"`
	MaxWeight                *int             `json:"max_weight"`
	ActiveTillDate           *string          `json:"active_till_date"`
	IsActive                 *bool            `json:"is_active"`
	Icon                     *string          `json:"icon"`
	Colors                   *AttributeColors `json:"colors"`
}

type ResourceTypeListFilter struct {
	DepartmentID *uuid.UUID
	Search       string
	Active       *bool
	Page         int
	Limit        int
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
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	AltName        string     `json:"alt_name"`
	DepartmentID   uuid.UUID  `json:"department_id"`
	RadioID        string     `json:"radio_id"`
	LocationName   string     `json:"location_name"`
	LocationID     *uuid.UUID `json:"location_id"`
	Latitude       float64    `json:"-"`
	Longitude      float64    `json:"-"`
	ActiveTillDate *time.Time `json:"-"`
	IsActive       bool       `json:"is_active"`
	Icon           string     `json:"icon"`
	ResourceIDs    UUIDList   `gorm:"column:resource_ids" json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type ServicePointResponse struct {
	ID             uuid.UUID   `json:"id"`
	Name           string      `json:"name"`
	AltName        string      `json:"alt_name"`
	DepartmentID   uuid.UUID   `json:"department_id"`
	RadioID        string      `json:"radio_id"`
	LocationName   string      `json:"location_name"`
	LocationID     *uuid.UUID  `json:"location_id"`
	Location       Location    `json:"location"`
	ActiveTillDate *string     `json:"active_till_date"`
	IsActive       bool        `json:"is_active"`
	Icon           string      `json:"icon"`
	ResourceIDs    []uuid.UUID `json:"resource_ids"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func (p ServicePoint) Response() ServicePointResponse {
	ids := []uuid.UUID(p.ResourceIDs)
	if ids == nil {
		ids = []uuid.UUID{}
	}
	return ServicePointResponse{
		ID:             p.ID,
		Name:           p.Name,
		AltName:        p.AltName,
		DepartmentID:   p.DepartmentID,
		RadioID:        p.RadioID,
		LocationName:   p.LocationName,
		LocationID:     p.LocationID,
		Location:       Location{Lat: p.Latitude, Lng: p.Longitude},
		ActiveTillDate: formatDate(p.ActiveTillDate),
		IsActive:       p.IsActive,
		Icon:           p.Icon,
		ResourceIDs:    ids,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

type ServicePointWriteRequest struct {
	Name           string         `json:"name"`
	AltName        *string        `json:"alt_name"`
	DepartmentID   uuid.UUID      `json:"department_id"`
	RadioID        *string        `json:"radio_id"`
	LocationName   *string        `json:"location_name"`
	LocationID     *uuid.UUID     `json:"location_id"`
	Location       *LocationInput `json:"location"`
	ActiveTillDate *string        `json:"active_till_date"`
	IsActive       *bool          `json:"is_active"`
	Icon           *string        `json:"icon"`
	ResourceIDs    *[]uuid.UUID   `json:"resource_ids"`
}

type ServicePointListFilter struct {
	DepartmentID *uuid.UUID
	Search       string
	Active       *bool
	Page         int
	Limit        int
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
	ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ThingID           uuid.UUID  `gorm:"column:thing_id;type:uuid;not null" json:"thing_id"`
	Name              string     `gorm:"-" json:"name"`
	ResourceTypeID    uuid.UUID  `gorm:"column:resource_type_id;type:uuid;not null" json:"resource_type_id"`
	ServicePointID    *uuid.UUID `gorm:"column:service_point_id;type:uuid" json:"service_point_id"`
	Status            string     `gorm:"column:status;type:varchar(20);not null;default:available" json:"status"`
	AlternateName     string     `gorm:"column:alternate_name;type:varchar(255);not null;default:''" json:"alternate_name"`
	PlateNumber       string     `gorm:"column:plate_number;type:varchar(64);not null;default:''" json:"plate_number"`
	LocationName      string     `gorm:"column:location_name;type:varchar(255);not null;default:''" json:"location_name"`
	Classification    JSON       `gorm:"column:classification;type:jsonb;not null" json:"-"`
	Description       string     `gorm:"column:description;type:text;not null;default:''" json:"description"`
	RadioID           string     `gorm:"column:radio_id;type:varchar(100);not null;default:''" json:"radio_id"`
	IsActive          bool       `gorm:"column:is_active;not null;default:true" json:"is_active"`
	ActiveTillDate    *time.Time `gorm:"column:active_till_date;type:date" json:"-"`
	CallCode          string     `gorm:"column:call_code;type:varchar(64);not null;default:''" json:"call_code"`
	UnitCode          string     `gorm:"column:unit_code;type:varchar(64);not null;default:''" json:"unit_code"`
	LocationID        *uuid.UUID `gorm:"column:location_id;type:uuid" json:"location_id"`
	ClassificationIDs JSON       `gorm:"column:classification_ids;type:jsonb;not null" json:"-"`
	GPRS              string     `gorm:"column:gprs;type:varchar(64);not null;default:''" json:"gprs"`
	SMS               string     `gorm:"column:sms;type:varchar(64);not null;default:''" json:"sms"`
	SMSVIP            string     `gorm:"column:sms_vip;type:varchar(64);not null;default:''" json:"sms_vip"`
	SIM               string     `gorm:"column:sim;type:varchar(64);not null;default:''" json:"sim"`
	DriverUserID      *uuid.UUID `gorm:"column:driver_user_id;type:uuid" json:"driver_user_id"`
	ShiftID           *uuid.UUID `gorm:"column:shift_id;type:uuid" json:"shift_id"`
	PartnerUserID     *uuid.UUID `gorm:"column:partner_user_id;type:uuid" json:"partner_user_id"`
	Latitude          *float64   `gorm:"-" json:"latitude"`
	Longitude         *float64   `gorm:"-" json:"longitude"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;autoUpdateTime" json:"updated_at"`
}

func (Resource) TableName() string { return "resource" }

func (r *Resource) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type ResourceResponse struct {
	ID                uuid.UUID   `json:"id"`
	ThingID           uuid.UUID   `json:"thing_id"`
	Name              string      `json:"name"`
	ResourceTypeID    uuid.UUID   `json:"resource_type_id"`
	ServicePointID    *uuid.UUID  `json:"service_point_id"`
	Status            string      `json:"status"`
	AlternateName     string      `json:"alternate_name"`
	PlateNumber       string      `json:"plate_number"`
	LocationName      string      `json:"location_name"`
	Classification    []string    `json:"classification"`
	Description       string      `json:"description"`
	RadioID           string      `json:"radio_id"`
	IsActive          bool        `json:"is_active"`
	ActiveTillDate    *string     `json:"active_till_date"`
	CallCode          string      `json:"call_code"`
	UnitCode          string      `json:"unit_code"`
	LocationID        *uuid.UUID  `json:"location_id"`
	ClassificationIDs []uuid.UUID `json:"classification_ids"`
	GPRS              string      `json:"gprs"`
	SMS               string      `json:"sms"`
	SMSVIP            string      `json:"sms_vip"`
	SIM               string      `json:"sim"`
	DriverUserID      *uuid.UUID  `json:"driver_user_id"`
	ShiftID           *uuid.UUID  `json:"shift_id"`
	PartnerUserID     *uuid.UUID  `json:"partner_user_id"`
	Latitude          *float64    `json:"latitude"`
	Longitude         *float64    `json:"longitude"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

func (r Resource) Response() ResourceResponse {
	return ResourceResponse{
		ID:                r.ID,
		ThingID:           r.ThingID,
		Name:              r.Name,
		ResourceTypeID:    r.ResourceTypeID,
		ServicePointID:    r.ServicePointID,
		Status:            r.Status,
		AlternateName:     r.AlternateName,
		PlateNumber:       r.PlateNumber,
		LocationName:      r.LocationName,
		Classification:    decodeStrings(r.Classification),
		Description:       r.Description,
		RadioID:           r.RadioID,
		IsActive:          r.IsActive,
		ActiveTillDate:    formatDate(r.ActiveTillDate),
		CallCode:          r.CallCode,
		UnitCode:          r.UnitCode,
		LocationID:        r.LocationID,
		ClassificationIDs: decodeUUIDs(r.ClassificationIDs),
		GPRS:              r.GPRS,
		SMS:               r.SMS,
		SMSVIP:            r.SMSVIP,
		SIM:               r.SIM,
		DriverUserID:      r.DriverUserID,
		ShiftID:           r.ShiftID,
		PartnerUserID:     r.PartnerUserID,
		Latitude:          r.Latitude,
		Longitude:         r.Longitude,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
}

type ResourceWriteRequest struct {
	ThingID           uuid.UUID    `json:"thing_id"`
	Name              *string      `json:"name"`
	ResourceTypeID    uuid.UUID    `json:"resource_type_id"`
	ServicePointID    *uuid.UUID   `json:"service_point_id"`
	Status            string       `json:"status"`
	AlternateName     *string      `json:"alternate_name"`
	PlateNumber       *string      `json:"plate_number"`
	LocationName      *string      `json:"location_name"`
	Classification    *[]string    `json:"classification"`
	Description       *string      `json:"description"`
	RadioID           *string      `json:"radio_id"`
	IsActive          *bool        `json:"is_active"`
	ActiveTillDate    *string      `json:"active_till_date"`
	CallCode          *string      `json:"call_code"`
	UnitCode          *string      `json:"unit_code"`
	LocationID        *uuid.UUID   `json:"location_id"`
	ClassificationIDs *[]uuid.UUID `json:"classification_ids"`
	GPRS              *string      `json:"gprs"`
	SMS               *string      `json:"sms"`
	SMSVIP            *string      `json:"sms_vip"`
	SIM               *string      `json:"sim"`
	DriverUserID      *uuid.UUID   `json:"driver_user_id"`
	ShiftID           *uuid.UUID   `json:"shift_id"`
	PartnerUserID     *uuid.UUID   `json:"partner_user_id"`
}

type ResourceListFilter struct {
	ResourceTypeID   *uuid.UUID
	ServicePointID   *uuid.UUID
	Status           string
	Search           string
	Active           *bool
	DriverUserID     *uuid.UUID
	LocationID       *uuid.UUID
	ClassificationID *uuid.UUID
	ActiveTillDate   string
	Page             int
	Limit            int
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
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey" json:"id"`
	ResourceID    uuid.UUID  `gorm:"column:resource_id;type:uuid;not null" json:"resource_id"`
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;not null" json:"user_id"`
	PartnerUserID *uuid.UUID `gorm:"column:partner_user_id;type:uuid" json:"partner_user_id"`
	ShiftID       *uuid.UUID `gorm:"column:shift_id;type:uuid" json:"shift_id"`
	StartAt       *time.Time `gorm:"column:start_at" json:"start_at"`
	EndAt         *time.Time `gorm:"column:end_at" json:"end_at"`
	Status        string     `gorm:"column:status;type:varchar(20);not null;default:scheduled" json:"status"`
}

func (ResourceShiftAssignment) TableName() string { return "resource_shift_assignment" }

func (a *ResourceShiftAssignment) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type AssignmentResponse struct {
	ID            uuid.UUID  `json:"id"`
	ResourceID    uuid.UUID  `json:"resource_id"`
	UserID        uuid.UUID  `json:"user_id"`
	PartnerUserID *uuid.UUID `json:"partner_user_id"`
	ShiftID       *uuid.UUID `json:"shift_id"`
	StartAt       *time.Time `json:"start_at"`
	EndAt         *time.Time `json:"end_at"`
	Status        string     `json:"status"`
}

func (a ResourceShiftAssignment) Response() AssignmentResponse {
	return AssignmentResponse{
		ID:            a.ID,
		ResourceID:    a.ResourceID,
		UserID:        a.UserID,
		PartnerUserID: a.PartnerUserID,
		ShiftID:       a.ShiftID,
		StartAt:       a.StartAt,
		EndAt:         a.EndAt,
		Status:        a.Status,
	}
}

type AssignmentWriteRequest struct {
	ResourceID    uuid.UUID  `json:"resource_id"`
	UserID        uuid.UUID  `json:"user_id"`
	PartnerUserID *uuid.UUID `json:"partner_user_id"`
	ShiftID       *uuid.UUID `json:"shift_id"`
	StartAt       *time.Time `json:"start_at"`
	EndAt         *time.Time `json:"end_at"`
	Status        string     `json:"status"`
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

func formatDate(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.In(time.UTC).Format("2006-01-02")
	return &s
}

func decodeColors(raw JSON) AttributeColors {
	colors := AttributeColors{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &colors)
	}
	return colors.Filled()
}

func decodeStrings(raw JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func decodeUUIDs(raw JSON) []uuid.UUID {
	if len(raw) == 0 {
		return []uuid.UUID{}
	}
	var out []uuid.UUID
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []uuid.UUID{}
	}
	return out
}
