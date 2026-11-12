package handlers

import (
	"bytes"
	"io"
	"strings"

	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const iconMaxBytes = 2 << 20

type iconRow struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	ContentType string    `gorm:"column:content_type"`
	Body        []byte
}

func (iconRow) TableName() string { return "dispatch_icon" }

type IconHandler struct{ db *gorm.DB }

func NewIconHandler(db *gorm.DB) *IconHandler { return &IconHandler{db: db} }

func (h *IconHandler) Create(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file is required")
	}
	if file.Size > iconMaxBytes {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file must be at most 2MB")
	}
	opened, err := file.Open()
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file could not be read")
	}
	defer opened.Close()
	body, err := io.ReadAll(io.LimitReader(opened, iconMaxBytes+1))
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file could not be read")
	}
	if len(body) == 0 || len(body) > iconMaxBytes {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file must be a PNG, JPG, or SVG of at most 2MB")
	}
	kind := iconType(body)
	if kind == "" {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "file must be a PNG, JPG, or SVG")
	}
	row := iconRow{ID: uuid.New(), ContentType: kind, Body: body}
	if err := h.db.WithContext(c.UserContext()).Create(&row).Error; err != nil {
		return utils.ErrorResponse(c, fiber.StatusInternalServerError, "icon could not be stored")
	}
	path := "/dispatch/icons/" + row.ID.String()
	return utils.SuccessResponse(c, fiber.StatusCreated, "icon stored", fiber.Map{
		"id":           row.ID,
		"path":         path,
		"content_type": kind,
	})
}

func (h *IconHandler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusBadRequest, "icon id must be a UUID")
	}
	var row iconRow
	err = h.db.WithContext(c.UserContext()).First(&row, "id = ?", id).Error
	if err != nil {
		return utils.ErrorResponse(c, fiber.StatusNotFound, "icon not found")
	}
	c.Set("Content-Type", row.ContentType)
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Security-Policy", "default-src 'none'")
	return c.Send(row.Body)
}

func iconType(body []byte) string {
	if bytes.HasPrefix(body, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "image/png"
	}
	if bytes.HasPrefix(body, []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg"
	}
	text := strings.ToLower(string(bytes.TrimSpace(body)))
	if strings.Contains(text, "<svg") && !strings.Contains(text, "<script") {
		return "image/svg+xml"
	}
	return ""
}
