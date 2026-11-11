package handlers

import (
	"strconv"
	"strings"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ResourceTypeHandler struct {
	svc *services.ResourceTypeService
}

func NewResourceTypeHandler(svc *services.ResourceTypeService) *ResourceTypeHandler {
	return &ResourceTypeHandler{svc: svc}
}

func (h *ResourceTypeHandler) Create(c *fiber.Ctx) error {
	var req models.ResourceTypeWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "resource type created", row)
}

func (h *ResourceTypeHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ResourceTypeWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource type updated", row)
}

func (h *ResourceTypeHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource type deleted", nil)
}

func (h *ResourceTypeHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource type retrieved", row)
}

func (h *ResourceTypeHandler) List(c *fiber.Ctx) error {
	departmentID, err := loggedQueryID(c, "department_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), models.ResourceTypeListFilter{DepartmentID: departmentID})
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource types retrieved", rows)
}

type ServicePointHandler struct {
	svc *services.ServicePointService
}

func NewServicePointHandler(svc *services.ServicePointService) *ServicePointHandler {
	return &ServicePointHandler{svc: svc}
}

func (h *ServicePointHandler) Create(c *fiber.Ctx) error {
	var req models.ServicePointWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "service point created", row)
}

func (h *ServicePointHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ServicePointWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "service point updated", row)
}

func (h *ServicePointHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "service point deleted", nil)
}

func (h *ServicePointHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "service point retrieved", row)
}

func (h *ServicePointHandler) Nearest(c *fiber.Ctx) error {
	lat, lng, limit, typeID, err := servicePointNearestQuery(c)
	if err != nil {
		return writeServiceErr(c, err)
	}
	rows, err := h.svc.FindNearest(c.UserContext(), lat, lng, typeID, limit)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "nearest service points retrieved", rows)
}

func servicePointNearestQuery(c *fiber.Ctx) (float64, float64, int, *uuid.UUID, error) {
	fields := map[string]string{}
	lat, latOK := queryFloat(c, "lat", "latitude")
	lng, lngOK := queryFloat(c, "lng", "longitude")
	if !latOK {
		fields["lat"] = "lat must be between -90 and 90"
	}
	if !lngOK {
		fields["lng"] = "lng must be between -180 and 180"
	}
	typeID, err := optionalUUIDQuery(c, "resourceTypeId")
	if err != nil {
		fields["resourceTypeId"] = "resourceTypeId must be a UUID"
	}
	if typeID == nil {
		typeID, err = optionalUUIDQuery(c, "resource_type_id")
		if err != nil {
			fields["resource_type_id"] = "resource_type_id must be a UUID"
		}
	}
	limit := defaultNearestLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil || n < 1 {
			fields["limit"] = "limit must be at least 1"
		} else if n > 100 {
			fields["limit"] = "limit must be at most 100"
		} else {
			limit = n
		}
	}
	if len(fields) > 0 {
		return 0, 0, 0, nil, services.Invalid("validation failed", fields)
	}
	return lat, lng, limit, typeID, nil
}

func (h *ServicePointHandler) List(c *fiber.Ctx) error {
	rows, err := h.svc.List(c.UserContext())
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "service points retrieved", rows)
}

type ShiftHandler struct {
	svc *services.ShiftService
}

func NewShiftHandler(svc *services.ShiftService) *ShiftHandler {
	return &ShiftHandler{svc: svc}
}

func (h *ShiftHandler) Create(c *fiber.Ctx) error {
	var req models.ShiftWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "shift created", row)
}

func (h *ShiftHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ShiftWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "shift updated", row)
}

func (h *ShiftHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "shift deleted", nil)
}

func (h *ShiftHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "shift retrieved", row)
}

func (h *ShiftHandler) List(c *fiber.Ctx) error {
	rows, err := h.svc.List(c.UserContext())
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "shifts retrieved", rows)
}

type ResourceHandler struct {
	svc *services.ResourceService
}

func NewResourceHandler(svc *services.ResourceService) *ResourceHandler {
	return &ResourceHandler{svc: svc}
}

func (h *ResourceHandler) Create(c *fiber.Ctx) error {
	var req models.ResourceWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "resource created", row)
}

func (h *ResourceHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ResourceWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource updated", row)
}

func (h *ResourceHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource deleted", nil)
}

func (h *ResourceHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resource retrieved", row)
}

func (h *ResourceHandler) List(c *fiber.Ctx) error {
	filter, err := resourceFilter(c)
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), filter)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "resources retrieved", rows)
}

func resourceFilter(c *fiber.Ctx) (models.ResourceListFilter, error) {
	var filter models.ResourceListFilter
	typeID, err := loggedQueryID(c, "resource_type_id")
	if err != nil {
		return filter, err
	}
	pointID, err := loggedQueryID(c, "service_point_id")
	if err != nil {
		return filter, err
	}
	filter.ResourceTypeID = typeID
	filter.ServicePointID = pointID
	filter.Status = c.Query("status")
	return filter, nil
}

type AssignmentHandler struct {
	svc *services.AssignmentService
}

func NewAssignmentHandler(svc *services.AssignmentService) *AssignmentHandler {
	return &AssignmentHandler{svc: svc}
}

func (h *AssignmentHandler) Create(c *fiber.Ctx) error {
	var req models.AssignmentWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "assignment created", row)
}

func (h *AssignmentHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.AssignmentWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "assignment updated", row)
}

func (h *AssignmentHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "assignment deleted", nil)
}

func (h *AssignmentHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "assignment retrieved", row)
}

func (h *AssignmentHandler) List(c *fiber.Ctx) error {
	rows, err := h.svc.List(c.UserContext())
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "assignments retrieved", rows)
}
