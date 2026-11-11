package handlers

import (
	"strconv"
	"strings"

	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const defaultNearestLimit = 10

type NearestHandler struct {
	svc *services.NearestService
}

func NewNearestHandler(svc *services.NearestService) *NearestHandler {
	return &NearestHandler{svc: svc}
}

func (h *NearestHandler) Find(c *fiber.Ctx) error {
	lat, lng, limit, departmentID, typeID, err := nearestQuery(c)
	if err != nil {
		return writeServiceErr(c, err)
	}
	rows, err := h.svc.FindNearestAvailable(c.UserContext(), lat, lng, typeID, departmentID, limit)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "nearest resources retrieved", rows)
}

func nearestQuery(c *fiber.Ctx) (float64, float64, int, uuid.UUID, *uuid.UUID, error) {
	fields := map[string]string{}
	lat, latOK := queryFloat(c, "lat", "latitude")
	lng, lngOK := queryFloat(c, "lng", "longitude")
	if !latOK {
		fields["lat"] = "lat must be between -90 and 90"
	}
	if !lngOK {
		fields["lng"] = "lng must be between -180 and 180"
	}
	departmentID, deptErr := optionalUUIDQuery(c, "department_id")
	if deptErr != nil {
		fields["department_id"] = "department_id must be a UUID"
	} else if departmentID == nil {
		fields["department_id"] = "department_id is required"
	}
	typeID, typeErr := optionalUUIDQuery(c, "resource_type_id")
	if typeErr != nil {
		fields["resource_type_id"] = "resource_type_id must be a UUID"
	}
	limit := defaultNearestLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 {
			fields["limit"] = "limit must be at least 1"
		} else {
			limit = n
		}
	}
	if len(fields) > 0 {
		return 0, 0, 0, uuid.Nil, nil, services.Invalid("validation failed", fields)
	}
	return lat, lng, limit, *departmentID, typeID, nil
}

func queryFloat(c *fiber.Ctx, names ...string) (float64, bool) {
	raw := ""
	for _, name := range names {
		raw = strings.TrimSpace(c.Query(name))
		if raw != "" {
			break
		}
	}
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
