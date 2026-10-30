package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ThingHandler struct {
	svc *services.ThingService
}

func NewThingHandler(svc *services.ThingService) *ThingHandler {
	return &ThingHandler{svc: svc}
}

func (h *ThingHandler) Create(c *fiber.Ctx) error {
	var req models.ThingCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	thing, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "thing created", thing)
}

func (h *ThingHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.ThingUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	thing, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "thing updated", thing)
}

func (h *ThingHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "thing deleted", nil)
}

func (h *ThingHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	thing, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "thing retrieved", thing)
}

func (h *ThingHandler) List(c *fiber.Ctx) error {
	filter, err := thingFilter(c)
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	things, err := h.svc.List(c.UserContext(), filter)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "things retrieved", things)
}

func thingFilter(c *fiber.Ctx) (models.ThingListFilter, error) {
	var filter models.ThingListFilter
	filter.Type = c.Query("type")
	departmentID, err := optionalUUIDQuery(c, "department_id")
	if err != nil {
		return filter, err
	}
	groupID, err := optionalUUIDQuery(c, "group_id")
	if err != nil {
		return filter, err
	}
	filter.DepartmentID = departmentID
	filter.GroupID = groupID
	return filter, nil
}

func optionalUUIDQuery(c *fiber.Ctx, name string) (*uuid.UUID, error) {
	raw := c.Query(name)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, errInvalidQuery(name)
	}
	return &id, nil
}

type queryError string

func (e queryError) Error() string { return string(e) }

func errInvalidQuery(name string) error {
	return queryError("invalid " + name)
}
