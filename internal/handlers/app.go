package handlers

import (
	"errors"
	"strings"

	"github.com/automax/registry/internal/repository"
	"github.com/automax/registry/internal/services"
	"github.com/automax/registry/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func NewApp(db *gorm.DB, pub services.Publisher, health HealthTargets, allowOrigins string) *fiber.App {
	things := repository.NewThingRepository(db)
	channels := repository.NewChannelRepository(db)
	groups := repository.NewGroupRepository(db)

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
	app.Use(cors.New(cors.Config{AllowOrigins: origins(allowOrigins)}))

	if health.DB == nil {
		health.DB = db
	}
	app.Get("/health", health.handle)

	thingHandler := NewThingHandler(services.NewThingService(things, groups, pub))
	channelHandler := NewChannelHandler(services.NewChannelService(channels))
	aclHandler := NewACLHandler(services.NewACLService(repository.NewACLRepository(db), channels, things))
	groupHandler := NewGroupHandler(services.NewGroupService(groups))
	credentialHandler := NewCredentialHandler(services.NewCredentialService(things, repository.NewCredentialRepository(db)))
	refs := repository.NewExternalRefRepository(db)
	externalRefHandler := NewExternalRefHandler(services.NewExternalRefService(refs, things))

	registry := app.Group("/registry")

	registry.Get("/things", thingHandler.List)
	registry.Get("/things/:id", thingHandler.Get)
	registry.Post("/things", thingHandler.Create)
	registry.Put("/things/:id", thingHandler.Update)
	registry.Delete("/things/:id", thingHandler.Delete)
	registry.Post("/things/:id/external-refs", externalRefHandler.Upsert)
	registry.Get("/things/:id/credentials", credentialHandler.List)
	registry.Post("/things/:id/credentials", credentialHandler.Issue)
	registry.Delete("/things/:id/credentials/:credentialId", credentialHandler.Revoke)

	registry.Get("/channels", channelHandler.List)
	registry.Post("/channels", channelHandler.Create)
	registry.Get("/channels/:id", channelHandler.Get)
	registry.Put("/channels/:id", channelHandler.Update)
	registry.Delete("/channels/:id", channelHandler.Delete)
	registry.Get("/channels/:id/acl", aclHandler.List)
	registry.Post("/channels/:id/acl", aclHandler.Create)
	registry.Put("/channels/:id/acl/:aclId", aclHandler.Update)
	registry.Delete("/channels/:id/acl/:aclId", aclHandler.Delete)

	registry.Get("/groups", groupHandler.List)
	registry.Get("/groups/:id", groupHandler.Get)
	registry.Post("/groups", groupHandler.Create)
	registry.Put("/groups/:id", groupHandler.Update)
	registry.Delete("/groups/:id", groupHandler.Delete)

	return app
}

func origins(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "*"
	}
	return raw
}
