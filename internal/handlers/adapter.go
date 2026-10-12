package handlers

import (
	"errors"

	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// NewAdapter is the Caugar position process. It does not serve registry routes.
func NewAdapter(token string, store services.PositionStore) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var fe *fiber.Error
			if errors.As(err, &fe) {
				code = fe.Code
			}
			return utils.ErrorResponse(c, code, err.Error())
		},
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${method} ${path} ${latency}\n",
	}))
	app.Get("/health", func(c *fiber.Ctx) error {
		return utils.SuccessResponse(c, fiber.StatusOK, "ok", fiber.Map{"service": "adapter"})
	})
	app.Post("/ingest/positions", NewPositionHandler(services.NewPositionService(store), token).Ingest)
	return app
}
