package handlers

import (
	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type GroupHandler struct {
	svc *services.GroupService
}

func NewGroupHandler(svc *services.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

func (h *GroupHandler) Create(c *fiber.Ctx) error {
	var req models.GroupWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	group, err := h.svc.Create(c.UserContext(), req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusCreated, "group created", group)
}

func (h *GroupHandler) Update(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	var req models.GroupWriteRequest
	if err := c.BodyParser(&req); err != nil {
		return badJSON(c)
	}
	group, err := h.svc.Update(c.UserContext(), id, req)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "group updated", group)
}

func (h *GroupHandler) Delete(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	if err := h.svc.Delete(c.UserContext(), id); err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "group deleted", nil)
}

func (h *GroupHandler) Get(c *fiber.Ctx) error {
	id, err := pathID(c, "id")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
	}
	group, err := h.svc.Get(c.UserContext(), id)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "group retrieved", group)
}

func (h *GroupHandler) List(c *fiber.Ctx) error {
	filter := models.GroupListFilter{}
	raw := c.Query("parent_group_id")
	switch raw {
	case "":
	case "null":
		filter.RootsOnly = true
	default:
		id, err := optionalUUIDQuery(c, "parent_group_id")
		if err != nil {
			return utils.ErrorResponse(c, fiber.StatusBadRequest, err.Error())
		}
		filter.ParentGroupID = id
	}
	groups, err := h.svc.List(c.UserContext(), filter)
	if err != nil {
		return writeServiceErr(c, err)
	}
	return utils.SuccessResponse(c, fiber.StatusOK, "groups retrieved", groups)
}
