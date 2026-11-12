package utils

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func SuccessResponse(c *fiber.Ctx, statusCode int, message string, data interface{}) error {
	return c.Status(statusCode).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func ErrorResponse(c *fiber.Ctx, statusCode int, message string) error {
	return c.Status(statusCode).JSON(Response{
		Success: false,
		Error:   message,
	})
}

func InternalErrorResponse(c *fiber.Ctx, err error, userMessage string) error {
	log.Printf("registry: %s %s: %s", c.Method(), c.Path(), Redact(err.Error()))
	return ErrorResponse(c, fiber.StatusInternalServerError, userMessage)
}

type PagedResponse struct {
	Success    bool        `json:"success"`
	Message    string      `json:"message,omitempty"`
	Data       interface{} `json:"data"`
	Page       int         `json:"page"`
	Limit      int         `json:"limit"`
	TotalItems int64       `json:"total_items"`
	TotalPages int         `json:"total_pages"`
}

func PagedSuccessResponse(c *fiber.Ctx, message string, data interface{}, page, limit int, total int64) error {
	if limit < 1 {
		limit = 1
	}
	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}
	if data == nil {
		data = []any{}
	}
	return c.Status(fiber.StatusOK).JSON(PagedResponse{
		Success:    true,
		Message:    message,
		Data:       data,
		Page:       page,
		Limit:      limit,
		TotalItems: total,
		TotalPages: totalPages,
	})
}
