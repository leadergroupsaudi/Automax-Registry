package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type ACLHandler struct {
	svc *services.ACLService
}

func NewACLHandler(svc *services.ACLService) *ACLHandler {
	return &ACLHandler{svc: svc}
}

func (h *ACLHandler) Create(c *fiber.Ctx) error {
	channelID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ChannelACLCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	acl, err := h.svc.Create(c.UserContext(), channelID, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "acl entry created", acl)
}

func (h *ACLHandler) Update(c *fiber.Ctx) error {
	channelID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	aclID, err := pathID(c, "aclId")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ChannelACLUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	acl, err := h.svc.Update(c.UserContext(), channelID, aclID, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "acl entry updated", acl)
}

func (h *ACLHandler) Delete(c *fiber.Ctx) error {
	channelID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	aclID, err := pathID(c, "aclId")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), channelID, aclID); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "acl entry deleted", nil)
}

func (h *ACLHandler) List(c *fiber.Ctx) error {
	channelID, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	rows, err := h.svc.List(c.UserContext(), channelID)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "acl entries retrieved", rows)
}
