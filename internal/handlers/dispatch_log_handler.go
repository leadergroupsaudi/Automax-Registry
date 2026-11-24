package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type DispatchLogHandler struct {
	svc *services.DispatchLogService
}

func NewDispatchLogHandler(svc *services.DispatchLogService) *DispatchLogHandler {
	return &DispatchLogHandler{svc: svc}
}

func (h *DispatchLogHandler) List(c *fiber.Ctx) error {
	incidentID, err := loggedQueryID(c, "incident_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	resourceID, err := loggedQueryID(c, "resource_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	assignmentID, err := loggedQueryID(c, "assignment_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), models.DispatchLogFilter{
		IncidentID:   incidentID,
		ResourceID:   resourceID,
		AssignmentID: assignmentID,
	})
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "dispatch logs retrieved", rows)
}
