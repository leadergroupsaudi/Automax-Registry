package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func apiErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal server error"
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
		switch code {
		case fiber.StatusNotFound:
			msg = fmt.Sprintf("no %s route at %s", c.Method(), c.Path())
		case fiber.StatusMethodNotAllowed:
			msg = fmt.Sprintf("%s is not allowed for %s", c.Method(), c.Path())
		default:
			msg = fe.Message
		}
	}
	if code >= 500 {
		log.Printf("registry: %s %s status=%d err=%s", c.Method(), c.Path(), code, utils.Redact(err.Error()))
	} else {
		log.Printf("registry: %s %s rejected status=%d reason=%s", c.Method(), c.Path(), code, msg)
	}
	return utils.ErrorResponse(c, code, msg)
}

func writeServiceErr(c *fiber.Ctx, err error) error {
	var se *services.Error
	if errors.As(err, &se) {
		switch se.Kind {
		case services.KindInvalid:
			reason := se.Message
			if len(se.Fields) > 0 {
				reason = se.Message + " " + fieldList(se.Fields)
				log.Printf("registry: %s %s rejected status=400 reason=%s", c.Method(), c.Path(), reason)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"success": false,
					"error":   se.Message,
					"errors":  se.Fields,
				})
			}
			log.Printf("registry: %s %s rejected status=400 reason=%s", c.Method(), c.Path(), reason)
			return utils.ErrorResponse(c, fiber.StatusBadRequest, se.Message)
		case services.KindNotFound:
			log.Printf("registry: %s %s rejected status=404 reason=%s", c.Method(), c.Path(), se.Message)
			return utils.ErrorResponse(c, fiber.StatusNotFound, se.Message)
		case services.KindConflict:
			log.Printf("registry: %s %s rejected status=409 reason=%s", c.Method(), c.Path(), se.Message)
			return utils.ErrorResponse(c, fiber.StatusConflict, se.Message)
		}
	}
	return utils.InternalErrorResponse(c, err, "internal server error")
}

func fieldList(fields map[string]string) string {
	parts := make([]string, 0, len(fields))
	for name, msg := range fields {
		parts = append(parts, name+"="+msg)
	}
	return strings.Join(parts, ", ")
}

func badJSON(c *fiber.Ctx) error {
	log.Printf("registry: %s %s rejected status=400 reason=request body must be JSON", c.Method(), c.Path())
	return utils.ErrorResponse(c, fiber.StatusBadRequest, "request body must be JSON")
}

func pathID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	raw := strings.TrimSpace(c.Params(name))
	id, err := uuid.Parse(raw)
	if err != nil {
		label := idLabel(c.Path(), name)
		log.Printf("registry: %s %s rejected status=400 reason=%s must be a UUID value=%q", c.Method(), c.Path(), label, clip(raw))
		return uuid.Nil, fmt.Errorf("%s must be a UUID", label)
	}
	return id, nil
}

func idLabel(path, param string) string {
	switch param {
	case "credentialId":
		return "credential id"
	case "aclId":
		return "acl id"
	}
	switch {
	case strings.Contains(path, "/groups/"):
		return "group id"
	case strings.Contains(path, "/channels/"):
		return "channel id"
	case strings.Contains(path, "/things/"):
		return "thing id"
	default:
		return "id"
	}
}

func clip(raw string) string {
	if len(raw) > 80 {
		return raw[:80]
	}
	return raw
}
