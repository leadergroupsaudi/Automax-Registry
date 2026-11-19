package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type FireStationAlertHandler struct {
	svc *services.FireStationAlertService
}

func NewFireStationAlertHandler(svc *services.FireStationAlertService) *FireStationAlertHandler {
	return &FireStationAlertHandler{svc: svc}
}

func (h *FireStationAlertHandler) AlertSelection(c *fiber.Ctx) error {
	var req models.FireStationAlertRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.AlertSelection(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "fire stations alerted", row)
}

func (h *FireStationAlertHandler) Alert(c *fiber.Ctx) error {
	incidentID, err := pathID(c, "incident_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.FireStationAlertRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Alert(c.UserContext(), incidentID, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "fire stations alerted", row)
}

func (h *FireStationAlertHandler) List(c *fiber.Ctx) error {
	incidentID, err := pathID(c, "incident_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), incidentID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "fire station alerts retrieved", rows)
}
