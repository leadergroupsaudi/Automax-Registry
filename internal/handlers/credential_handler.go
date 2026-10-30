package handlers

import (
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type CredentialHandler struct {
	svc *services.CredentialService
}

func NewCredentialHandler(svc *services.CredentialService) *CredentialHandler {
	return &CredentialHandler{svc: svc}
}

func (h *CredentialHandler) Issue(c *fiber.Ctx) error {
	thingID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	issued, err := h.svc.Issue(c.UserContext(), thingID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "credential issued", issued)
}

func (h *CredentialHandler) List(c *fiber.Ctx) error {
	thingID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	items, err := h.svc.List(c.UserContext(), thingID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "credentials retrieved", items)
}

func (h *CredentialHandler) Revoke(c *fiber.Ctx) error {
	thingID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	credentialID, err := pathID(c, "credentialId")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	item, err := h.svc.Revoke(c.UserContext(), thingID, credentialID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "credential revoked", item)
}
