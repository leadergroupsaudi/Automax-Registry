package handlers

import (
	"encoding/json"
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
					"message": fieldMessage(se.Message, se.Fields),
					"errors":  fieldErrors(se.Fields),
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
		case services.KindUnavailable:
			log.Printf("registry: %s %s rejected status=503 reason=%s", c.Method(), c.Path(), se.Message)
			return utils.ErrorResponse(c, fiber.StatusServiceUnavailable, se.Message)
		case services.KindUpstream:
			log.Printf("registry: %s %s rejected status=502 reason=%s", c.Method(), c.Path(), se.Message)
			return utils.ErrorResponse(c, fiber.StatusBadGateway, se.Message)
		}
	}
	return utils.InternalErrorResponse(c, err, "internal server error")
}

func fieldMessage(fallback string, fields map[string]string) string {
	if len(fields) == 1 {
		for _, msg := range fields {
			return msg
		}
	}
	return fallback
}

// fieldErrors wraps each message in a list. The unit, unit type, and fire station
// forms show errors[field][0], so a plain string is displayed as its first letter.
func fieldErrors(fields map[string]string) map[string][]string {
	out := make(map[string][]string, len(fields))
	for name, msg := range fields {
		out[name] = []string{msg}
	}
	return out
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

// adoptCaguarIdentifier treats a non-UUID thing_id on create as the Caguar identifier.
// The phone's tracker id is then stored as uniqueId, and the registry still assigns the thing UUID.
func adoptCaguarIdentifier(c *fiber.Ctx) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &probe); err != nil {
		return
	}
	raw, ok := probe["thing_id"]
	if !ok || string(raw) == "null" {
		return
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if _, err := uuid.Parse(text); err == nil {
		return
	}
	delete(probe, "thing_id")
	encoded, err := json.Marshal(text)
	if err != nil {
		return
	}
	probe["unique_id"] = encoded
	next, err := json.Marshal(probe)
	if err != nil {
		return
	}
	c.Request().SetBody(next)
	log.Printf("registry: %s %s thing id used as caguar identifier unique_id=%s", c.Method(), c.Path(), clip(text))
}

// requireUUIDString rejects a text value that is not a UUID before Fiber parses the body.
// A Caguar device id in thing_id otherwise fails UUID decoding and is reported as bad JSON.
// A missing value, an empty string, or a non-string is left for the normal parser.
func requireUUIDString(c *fiber.Ctx, field string) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &probe); err != nil {
		return nil
	}
	raw, ok := probe[field]
	if !ok || string(raw) == "null" {
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if _, err := uuid.Parse(text); err == nil {
		return nil
	}
	msg := field + " must be a UUID"
	log.Printf("registry: %s %s rejected status=400 reason=%s value=%q", c.Method(), c.Path(), msg, clip(text))
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"success": false,
		"error":   "validation failed",
		"errors":  map[string]string{field: msg},
	})
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
	case "incident_id":
		return "incident id"
	}
	switch {
	case strings.Contains(path, "/dispatch/assignments/"):
		return "dispatch assignment id"
	case strings.Contains(path, "/resource-shift-assignments/"):
		return "assignment id"
	case strings.Contains(path, "/resource-types/"):
		return "resource type id"
	case strings.Contains(path, "/service-points/"):
		return "service point id"
	case strings.Contains(path, "/shifts/"):
		return "shift id"
	case strings.Contains(path, "/resources/"):
		return "resource id"
	case strings.Contains(path, "/groups/"):
		return "group id"
	case strings.Contains(path, "/channels/"):
		return "channel id"
	case strings.Contains(path, "/devices/"):
		return "device id"
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

func loggedQueryID(c *fiber.Ctx, name string) (*uuid.UUID, error) {
	id, err := optionalUUIDQuery(c, name)
	if err != nil {
		log.Printf("registry: %s %s rejected status=400 reason=%s", c.Method(), c.Path(), err.Error())
		return nil, err
	}
	return id, nil
}
