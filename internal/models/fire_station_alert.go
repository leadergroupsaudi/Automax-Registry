package models

import (
	"time"

	"github.com/google/uuid"
)

// FireStationAlertRequest is the body for alerting fire stations about an incident.
// All alerts every active station in the department. Otherwise only ServicePointIDs are alerted.
type FireStationAlertRequest struct {
	DepartmentID    uuid.UUID   `json:"department_id"`
	IncidentID      *uuid.UUID  `json:"incident_id"`
	All             bool        `json:"all"`
	ServicePointIDs []uuid.UUID `json:"service_point_ids"`
}

// FireStationAlert is one station that has been alerted for an incident.
type FireStationAlert struct {
	ID             uuid.UUID  `gorm:"column:id;type:uuid;primaryKey"`
	IncidentID     *uuid.UUID `gorm:"column:incident_id;type:uuid"`
	ServicePointID uuid.UUID  `gorm:"column:service_point_id;type:uuid;not null"`
	DepartmentID   uuid.UUID  `gorm:"column:department_id;type:uuid;not null"`
	Name           string     `gorm:"column:name;->"`
	RadioID        string     `gorm:"column:radio_id;->"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
}

func (FireStationAlert) TableName() string { return "fire_station_alert" }

// FireStationAlertResponse is one alerted station returned to the dispatcher.
type FireStationAlertResponse struct {
	ServicePointID uuid.UUID `json:"service_point_id"`
	Name           string    `json:"name"`
	RadioID        string    `json:"radio_id"`
	AlertedAt      time.Time `json:"alerted_at"`
}

func (a FireStationAlert) Response() FireStationAlertResponse {
	return FireStationAlertResponse{
		ServicePointID: a.ServicePointID,
		Name:           a.Name,
		RadioID:        a.RadioID,
		AlertedAt:      a.CreatedAt,
	}
}

// FireStationAlertResult is the response after stations are alerted.
type FireStationAlertResult struct {
	IncidentID *uuid.UUID                 `json:"incident_id"`
	Alerted    []FireStationAlertResponse `json:"alerted"`
}
