package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type ExternalRefHandler struct {
	svc *services.ExternalRefService
}

func NewExternalRefHandler(svc *services.ExternalRefService) *ExternalRefHandler {
	return &ExternalRefHandler{svc: svc}
}

func (h *ExternalRefHandler) Upsert(c *fiber.Ctx) error {
	thingID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "invalid id")
	}
	var req models.ExternalRefRequest
	if err := c.BodyParser(&req); err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	ref, created, err := h.svc.Upsert(c.UserContext(), thingID, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	code := fiber.StatusOK
	if created {
		code = fiber.StatusCreated
	}
	return utils.SuccessResponse(c, code, "external ref saved", ref)
}
