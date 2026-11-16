package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type DispatchAssignmentHandler struct {
	svc *services.DispatchAssignmentService
}

func NewDispatchAssignmentHandler(svc *services.DispatchAssignmentService) *DispatchAssignmentHandler {
	return &DispatchAssignmentHandler{svc: svc}
}

func (h *DispatchAssignmentHandler) Create(c *fiber.Ctx) error {
	var req models.DispatchAssignmentWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "dispatch assignment created", row)
}

func (h *DispatchAssignmentHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.DispatchAssignmentWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "dispatch assignment updated", row)
}

func (h *DispatchAssignmentHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "dispatch assignment deleted", nil)
}

func (h *DispatchAssignmentHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	row, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "dispatch assignment retrieved", row)
}

func (h *DispatchAssignmentHandler) List(c *fiber.Ctx) error {
	incidentID, err := loggedQueryID(c, "incident_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	resourceID, err := loggedQueryID(c, "resource_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), models.DispatchAssignmentFilter{
		IncidentID: incidentID,
		ResourceID: resourceID,
		Status:     c.Query("status"),
	})
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "dispatch assignments retrieved", rows)
}
