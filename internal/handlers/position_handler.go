package handlers

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"strings"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type PositionHandler struct {
	svc   *services.PositionService
	token string
}

func NewPositionHandler(svc *services.PositionService, token string) *PositionHandler {
	return &PositionHandler{svc: svc, token: token}
}

func (h *PositionHandler) Ingest(c *fiber.Ctx) error {
	if !bearerOK(c.Get(fiber.HeaderAuthorization), h.token) {
		return utils.ErrorResponse(c, fiber.StatusUnauthorized, "unauthorized")
	}
	req, ignored, err := decodePosition(c.Body())
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	if len(ignored) > 0 {
		log.Printf("registry: position fields ignored names=%s", strings.Join(ignored, ","))
	}
	if err := h.svc.Ingest(c.UserContext(), req); err != nil {
		return writeServiceErr(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// decodePosition accepts the flat webhook body and Traccar's {"position":{...}} forwarder body.
// ignored lists JSON fields that are not stored.
func decodePosition(body []byte) (models.PositionIngestRequest, []string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || raw == nil {
		return models.PositionIngestRequest{}, nil, errOrInvalid(err)
	}
	ignored := unknownFields(raw)
	if dev, ok := raw["device"]; ok && !isJSONObject(dev) {
		ignored = append(ignored, "device")
	}
	sort.Strings(ignored)

	var req models.PositionIngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return models.PositionIngestRequest{}, nil, err
	}
	flat := req.DeviceID != "" || strings.TrimSpace(req.DeviceName) != "" || req.Latitude != nil || req.Longitude != nil || strings.TrimSpace(req.FixTime) != "" || req.Valid != nil
	if !flat {
		if pos, ok := raw["position"]; ok {
			if err := json.Unmarshal(pos, &req); err != nil {
				return models.PositionIngestRequest{}, nil, err
			}
		}
	}
	if strings.TrimSpace(req.DeviceName) == "" {
		if dev, ok := raw["device"]; ok && isJSONObject(dev) {
			var device struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(dev, &device); err != nil {
				return models.PositionIngestRequest{}, nil, err
			}
			req.DeviceName = strings.TrimSpace(device.Name)
		}
	}
	return req, ignored, nil
}

func unknownFields(raw map[string]json.RawMessage) []string {
	known := map[string]struct{}{
		"deviceId": {}, "deviceName": {}, "latitude": {}, "longitude": {}, "fixTime": {}, "valid": {},
		"position": {}, "device": {},
	}
	var ignored []string
	for key := range raw {
		if _, ok := known[key]; !ok {
			ignored = append(ignored, key)
		}
	}
	return ignored
}

func isJSONObject(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '{'
}

func errOrInvalid(err error) error {
	if err != nil {
		return err
	}
	return errInvalidJSON
}

var errInvalidJSON = errors.New("invalid request body")

func bearerOK(header, want string) bool {
	want = strings.TrimSpace(want)
	got := bearerToken(header)
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
