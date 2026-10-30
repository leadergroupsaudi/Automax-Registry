package handlers

import (
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// NewAdapter is the Caugar position process. It does not serve registry routes.
func NewAdapter(token string, store services.PositionStore) *fiber.App {
	return NewAdapterWithThings(token, store, nil)
}

// NewAdapterWithThings stores the live fix and copies it onto the linked thing.
func NewAdapterWithThings(token string, store services.PositionStore, things services.ThingCoordinateWriter) *fiber.App {
	svc := services.NewPositionService(store)
	svc.UseThingCoordinates(things)
	return newAdapter(token, svc)
}

func newAdapter(token string, svc *services.PositionService) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          apiErrorHandler,
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${method} ${path} ${latency}\n",
	}))
	app.Get("/health", func(c *fiber.Ctx) error {
		return utils.SuccessResponse(c, fiber.StatusOK, "ok", fiber.Map{"service": "adapter"})
	})
	app.Post("/ingest/positions", NewPositionHandler(svc, token).Ingest)
	return app
}
