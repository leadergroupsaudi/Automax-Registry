package handlers

import (
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type DevicePortalHandler struct {
	svc *services.DevicePortal
}

func NewDevicePortalHandler(svc *services.DevicePortal) *DevicePortalHandler {
	return &DevicePortalHandler{svc: svc}
}

func (h *DevicePortalHandler) Register(c *fiber.Ctx) error {
	var req services.DeviceRegistration
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Register(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "device registered", row)
}

type DriverPortalHandler struct {
	svc *services.DriverPortal
}

func NewDriverPortalHandler(svc *services.DriverPortal) *DriverPortalHandler {
	return &DriverPortalHandler{svc: svc}
}

func (h *DriverPortalHandler) Register(c *fiber.Ctx) error {
	var req services.DriverRegistration
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Register(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "driver registered", row)
}
