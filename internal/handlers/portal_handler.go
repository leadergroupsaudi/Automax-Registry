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

func (h *DevicePortalHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req services.DeviceRegistration
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "device updated", row)
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

func (h *DriverPortalHandler) UpdateByUniqueID(c *fiber.Ctx) error {
	id, err := pathID(c, "unique_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req services.DriverNameUpdate
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.UpdateByUniqueID(c.UserContext(), id.String(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "driver updated", row)
}

func (h *DriverPortalHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req services.DriverNameUpdate
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	row, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "driver updated", row)
}

func (h *DriverPortalHandler) DeleteByUniqueID(c *fiber.Ctx) error {
	id, err := pathID(c, "unique_id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.DeleteByUniqueID(c.UserContext(), id.String()); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "driver deleted", nil)
}

func (h *DriverPortalHandler) List(c *fiber.Ctx) error {
	rows, err := h.svc.List(c.UserContext())
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "drivers retrieved", rows)
}
