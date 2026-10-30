package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type ChannelHandler struct {
	svc *services.ChannelService
}

func NewChannelHandler(svc *services.ChannelService) *ChannelHandler {
	return &ChannelHandler{svc: svc}
}

func (h *ChannelHandler) Create(c *fiber.Ctx) error {
	var req models.ChannelWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	channel, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "channel created", channel)
}

func (h *ChannelHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ChannelWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	channel, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "channel updated", channel)
}

func (h *ChannelHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "channel deleted", nil)
}

func (h *ChannelHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	channel, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "channel retrieved", channel)
}

func (h *ChannelHandler) List(c *fiber.Ctx) error {
	channels, err := h.svc.List(c.UserContext())
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "channels retrieved", channels)
}
