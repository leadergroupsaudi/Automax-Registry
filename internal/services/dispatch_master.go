package services

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/automax/registry/internal/models"
	"github.com/google/uuid"
)

var hexColor = regexp.MustCompile(`^#(?:[0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)

func mergeResourceType(current *models.ResourceType, req models.ResourceTypeWriteRequest) (*models.ResourceType, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	timeout := timeoutSeconds(fields, req.AcceptanceTimeoutSeconds)
	base := models.ResourceType{IsActive: true}
	create := current == nil
	if current != nil {
		base = *current
	}
	minSpeed, maxSpeed := boundedPair(fields, "min_speed", "max_speed", req.MinSpeed, req.MaxSpeed, base.MinSpeed, base.MaxSpeed)
	minWeight, maxWeight := boundedPair(fields, "min_weight", "max_weight", req.MinWeight, req.MaxWeight, base.MinWeight, base.MaxWeight)
	colors, colorFields := optionalColors(req.Colors, base.Colors, create)
	for key, msg := range colorFields {
		fields[key] = msg
	}
	code := optionalText(fields, "code", req.Code, base.Code, 64)
	altName := optionalText(fields, "alt_name", req.AltName, base.AltName, 255)
	icon := optionalText(fields, "icon", req.Icon, base.Icon, 2048)
	activeTill := optionalDate(fields, "active_till_date", req.ActiveTillDate, base.ActiveTillDate)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	row := &models.ResourceType{
		DepartmentID:             req.DepartmentID,
		Name:                     name,
		Code:                     code,
		AltName:                  altName,
		AcceptanceTimeoutSeconds: timeout,
		MinSpeed:                 minSpeed,
		MaxSpeed:                 maxSpeed,
		MinWeight:                minWeight,
		MaxWeight:                maxWeight,
		ActiveTillDate:           activeTill,
		IsActive:                 optionalActive(req.IsActive, base.IsActive, create),
		Icon:                     icon,
		Colors:                   colors,
	}
	if current != nil {
		row.ID = current.ID
		row.CreatedAt = current.CreatedAt
	}
	return row, nil
}

func mergeServicePoint(current *models.ServicePoint, req models.ServicePointWriteRequest) (*models.ServicePoint, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	lat, lng := validateLocation(fields, req.Location)
	base := models.ServicePoint{IsActive: true}
	create := current == nil
	if current != nil {
		base = *current
	}
	if req.ResourceIDs != nil {
		for _, id := range *req.ResourceIDs {
			if id == uuid.Nil {
				fields["resource_ids"] = "resource_ids must be UUIDs"
				break
			}
		}
	}
	altName := optionalText(fields, "alt_name", req.AltName, base.AltName, 255)
	radioID := optionalText(fields, "radio_id", req.RadioID, base.RadioID, 100)
	if radioID == "" {
		fields["radio_id"] = "radio_id is required"
	}
	locationName := optionalText(fields, "location_name", req.LocationName, base.LocationName, 255)
	locationID := optionalUUID(fields, "location_id", req.LocationID, base.LocationID)
	icon := optionalText(fields, "icon", req.Icon, base.Icon, 2048)
	activeTill := optionalDate(fields, "active_till_date", req.ActiveTillDate, base.ActiveTillDate)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	row := &models.ServicePoint{
		Name:           name,
		AltName:        altName,
		DepartmentID:   req.DepartmentID,
		RadioID:        radioID,
		LocationName:   locationName,
		LocationID:     locationID,
		Latitude:       lat,
		Longitude:      lng,
		ActiveTillDate: activeTill,
		IsActive:       optionalActive(req.IsActive, base.IsActive, create),
		Icon:           icon,
	}
	if current != nil {
		row.ID = current.ID
		row.ResourceIDs = current.ResourceIDs
		row.CreatedAt = current.CreatedAt
	}
	return row, nil
}

func applyResourceDetails(row *models.Resource, current *models.Resource, req models.ResourceWriteRequest) error {
	base := models.Resource{IsActive: true, Classification: models.JSON(`[]`)}
	create := current == nil
	if current != nil {
		base = *current
	}
	fields := map[string]string{}
	row.AlternateName = optionalText(fields, "alternate_name", req.AlternateName, base.AlternateName, 255)
	row.PlateNumber = optionalText(fields, "plate_number", req.PlateNumber, base.PlateNumber, 64)
	row.LocationName = optionalText(fields, "location_name", req.LocationName, base.LocationName, 255)
	row.Description = optionalText(fields, "description", req.Description, base.Description, 4000)
	row.RadioID = optionalText(fields, "radio_id", req.RadioID, base.RadioID, 100)
	row.CallCode = optionalText(fields, "call_code", req.CallCode, base.CallCode, 64)
	row.UnitCode = optionalText(fields, "unit_code", req.UnitCode, base.UnitCode, 64)
	row.GPRS = optionalText(fields, "gprs", req.GPRS, base.GPRS, 64)
	row.SMS = optionalText(fields, "sms", req.SMS, base.SMS, 64)
	row.SMSVIP = optionalText(fields, "sms_vip", req.SMSVIP, base.SMSVIP, 64)
	row.SIM = optionalText(fields, "sim", req.SIM, base.SIM, 64)
	row.LocationID = optionalUUID(fields, "location_id", req.LocationID, base.LocationID)
	row.DriverUserID = optionalUUID(fields, "driver_user_id", req.DriverUserID, base.DriverUserID)
	row.ShiftID = optionalUUID(fields, "shift_id", req.ShiftID, base.ShiftID)
	row.PartnerUserID = optionalUUID(fields, "partner_user_id", req.PartnerUserID, base.PartnerUserID)
	row.ActiveTillDate = optionalDate(fields, "active_till_date", req.ActiveTillDate, base.ActiveTillDate)
	row.IsActive = optionalActive(req.IsActive, base.IsActive, create)
	row.Classification = optionalClasses(fields, req.Classification, base.Classification)
	row.ClassificationIDs = optionalIDList(fields, "classification_ids", req.ClassificationIDs, base.ClassificationIDs)
	if row.DriverUserID == nil && (req.ShiftID != nil || req.PartnerUserID != nil) {
		fields["driver_user_id"] = "driver_user_id is required when shift_id or partner_user_id is set"
	}
	if len(fields) > 0 {
		return Invalid("validation failed", fields)
	}
	return nil
}

func boundedPair(fields map[string]string, minField, maxField string, minPtr, maxPtr *int, curMin, curMax int) (int, int) {
	minV, maxV := curMin, curMax
	if minPtr != nil {
		if *minPtr < 0 {
			fields[minField] = minField + " must be at least 0"
		} else {
			minV = *minPtr
		}
	}
	if maxPtr != nil {
		if *maxPtr < 0 {
			fields[maxField] = maxField + " must be at least 0"
		} else {
			maxV = *maxPtr
		}
	}
	if _, badMin := fields[minField]; !badMin {
		if _, badMax := fields[maxField]; !badMax && minV > maxV {
			fields[maxField] = maxField + " must be greater than or equal to " + minField
		}
	}
	return minV, maxV
}

func optionalText(fields map[string]string, field string, value *string, current string, max int) string {
	if value == nil {
		return current
	}
	text := strings.TrimSpace(*value)
	if fields != nil && utf8.RuneCountInString(text) > max {
		fields[field] = field + " must be at most " + itoa(max) + " characters"
	}
	return text
}

func optionalDate(fields map[string]string, field string, value *string, current *time.Time) *time.Time {
	if value == nil {
		return current
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", text)
	if err != nil {
		if parsed, rfcErr := time.Parse(time.RFC3339, text); rfcErr == nil {
			t = parsed
			err = nil
		}
	}
	if err != nil {
		if fields != nil {
			fields[field] = field + " must be a date (YYYY-MM-DD)"
		}
		return current
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return &day
}

func optionalActive(value *bool, current bool, create bool) bool {
	if value == nil {
		if create {
			return true
		}
		return current
	}
	return *value
}

func optionalColors(value *models.AttributeColors, current models.JSON, create bool) (models.JSON, map[string]string) {
	if value == nil {
		if !create && len(current) > 0 && string(current) != "{}" && string(current) != "null" {
			return current, nil
		}
		return mustColors(models.DefaultAttributeColors()), nil
	}
	merged := value.Filled()
	fields := map[string]string{}
	check := func(field, color string) {
		if !hexColor.MatchString(color) {
			fields["colors."+field] = "colors." + field + " must be a hex color"
		}
	}
	check("available", merged.Available)
	check("busy", merged.Busy)
	check("moving_vehicle", merged.MovingVehicle)
	check("offline", merged.Offline)
	check("ignition_on", merged.IgnitionOn)
	check("ignition_off", merged.IgnitionOff)
	check("over_speed", merged.OverSpeed)
	check("stop_over_20_min", merged.StopOver20Min)
	if len(fields) > 0 {
		return nil, fields
	}
	return mustColors(merged), nil
}

func mustColors(colors models.AttributeColors) models.JSON {
	raw, err := json.Marshal(colors)
	if err != nil {
		return models.JSON(`{}`)
	}
	return models.JSON(raw)
}

func optionalClasses(fields map[string]string, value *[]string, current models.JSON) models.JSON {
	if value == nil {
		if len(current) == 0 {
			return models.JSON(`[]`)
		}
		return current
	}
	if len(*value) > 20 {
		fields["classification"] = "classification must have at most 20 items"
		return current
	}
	out := make([]string, 0, len(*value))
	seen := map[string]bool{}
	for _, item := range *value {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		if utf8.RuneCountInString(item) > 100 {
			fields["classification"] = "classification items must be at most 100 characters"
			return current
		}
		seen[item] = true
		out = append(out, item)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return models.JSON(`[]`)
	}
	return models.JSON(raw)
}

func optionalUUID(fields map[string]string, field string, value, current *uuid.UUID) *uuid.UUID {
	if value == nil {
		return current
	}
	if *value == uuid.Nil {
		fields[field] = field + " must be a UUID"
		return current
	}
	id := *value
	return &id
}

func optionalIDList(fields map[string]string, field string, value *[]uuid.UUID, current models.JSON) models.JSON {
	if value == nil {
		if len(current) == 0 {
			return models.JSON(`[]`)
		}
		return current
	}
	if len(*value) > 20 {
		fields[field] = field + " must have at most 20 items"
		return current
	}
	out := make([]uuid.UUID, 0, len(*value))
	seen := map[uuid.UUID]struct{}{}
	for _, id := range *value {
		if id == uuid.Nil {
			fields[field] = field + " must be UUIDs"
			return current
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return models.JSON(`[]`)
	}
	return models.JSON(raw)
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func publishEvent(pub Publisher, subject string, id uuid.UUID, data any) {
	if pub == nil || id == uuid.Nil {
		return
	}
	body, err := json.Marshal(map[string]any{
		"event":       subject,
		"id":          id,
		"occurred_at": time.Now().UTC(),
		"data":        data,
	})
	if err != nil {
		log.Printf("registry: failed to encode %s for %s: %v", subject, id, err)
		return
	}
	if err := pub.Publish(subject, body); err != nil {
		log.Printf("registry: failed to publish %s for %s: %v", subject, id, err)
	}
}
