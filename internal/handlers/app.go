package handlers

import (
	"strings"

	"github.com/automax/registry/internal/caguar"
	"github.com/automax/registry/internal/repository"
	"github.com/automax/registry/internal/services"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func NewApp(db *gorm.DB, pub services.Publisher, health HealthTargets, allowOrigins string, nearest *services.NearestService, portal *caguar.Client) *fiber.App {
	things := repository.NewThingRepository(db)
	channels := repository.NewChannelRepository(db)
	groups := repository.NewGroupRepository(db)

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          apiErrorHandler,
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

	refs := repository.NewExternalRefRepository(db)
	refSvc := services.NewExternalRefService(refs, things)
	thingSvc := services.NewThingService(things, groups, pub)
	thingHandler := NewThingHandler(thingSvc)
	channelHandler := NewChannelHandler(services.NewChannelService(channels))
	aclHandler := NewACLHandler(services.NewACLService(repository.NewACLRepository(db), channels, things))
	groupHandler := NewGroupHandler(services.NewGroupService(groups))
	credentialHandler := NewCredentialHandler(services.NewCredentialService(things, repository.NewCredentialRepository(db)))
	externalRefHandler := NewExternalRefHandler(refSvc)
	var portalAPI services.Portal
	if portal != nil {
		portalAPI = portal
	}
	deviceHandler := NewDevicePortalHandler(services.NewDevicePortal(thingSvc, refSvc, refs, portalAPI))
	driverHandler := NewDriverPortalHandler(services.NewDriverPortal(portalAPI))

	resourceTypes := repository.NewResourceTypeRepository(db)
	servicePoints := repository.NewServicePointRepository(db)
	shifts := repository.NewShiftRepository(db)
	resources := repository.NewResourceRepository(db)
	assignments := repository.NewAssignmentRepository(db)
	resourceTypeHandler := NewResourceTypeHandler(services.NewResourceTypeService(resourceTypes))
	servicePointHandler := NewServicePointHandler(services.NewServicePointService(servicePoints))
	shiftHandler := NewShiftHandler(services.NewShiftService(shifts))
	resourceSvc := services.NewResourceService(resources, things, resourceTypes, servicePoints)
	assignmentSvc := services.NewAssignmentService(assignments, resources, shifts)
	if nearest != nil {
		resourceSvc.UseStateCache(nearest)
		assignmentSvc.UseDutyCache(nearest)
	}
	resourceHandler := NewResourceHandler(resourceSvc)
	assignmentHandler := NewAssignmentHandler(assignmentSvc)
	var nearestHandler *NearestHandler
	if nearest != nil {
		nearestHandler = NewNearestHandler(nearest)
	}

	registry := app.Group("/registry")

	registry.Get("/things", thingHandler.List)
	registry.Get("/things/:id", thingHandler.Get)
	registry.Post("/things", thingHandler.Create)
	registry.Put("/things/:id", thingHandler.Update)
	registry.Delete("/things/:id", thingHandler.Delete)
	registry.Post("/things/:id/external-refs", externalRefHandler.Upsert)
	registry.Post("/devices", deviceHandler.Register)
	registry.Post("/drivers", driverHandler.Register)
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

	dispatch := app.Group("/dispatch")
	dispatch.Get("/resource-types", resourceTypeHandler.List)
	dispatch.Post("/resource-types", resourceTypeHandler.Create)
	dispatch.Get("/resource-types/:id", resourceTypeHandler.Get)
	dispatch.Put("/resource-types/:id", resourceTypeHandler.Update)
	dispatch.Delete("/resource-types/:id", resourceTypeHandler.Delete)

	dispatch.Get("/resources", resourceHandler.List)
	dispatch.Post("/resources", resourceHandler.Create)
	if nearestHandler != nil {
		dispatch.Get("/resources/nearest", nearestHandler.Find)
	}
	dispatch.Get("/resources/:id", resourceHandler.Get)
	dispatch.Put("/resources/:id", resourceHandler.Update)
	dispatch.Delete("/resources/:id", resourceHandler.Delete)

	dispatch.Get("/service-points", servicePointHandler.List)
	dispatch.Post("/service-points", servicePointHandler.Create)
	dispatch.Get("/service-points/nearest", servicePointHandler.Nearest)
	dispatch.Get("/service-points/:id", servicePointHandler.Get)
	dispatch.Put("/service-points/:id", servicePointHandler.Update)
	dispatch.Delete("/service-points/:id", servicePointHandler.Delete)

	dispatch.Get("/shifts", shiftHandler.List)
	dispatch.Post("/shifts", shiftHandler.Create)
	dispatch.Get("/shifts/:id", shiftHandler.Get)
	dispatch.Put("/shifts/:id", shiftHandler.Update)
	dispatch.Delete("/shifts/:id", shiftHandler.Delete)

	dispatch.Get("/resource-shift-assignments", assignmentHandler.List)
	dispatch.Post("/resource-shift-assignments", assignmentHandler.Create)
	dispatch.Get("/resource-shift-assignments/:id", assignmentHandler.Get)
	dispatch.Put("/resource-shift-assignments/:id", assignmentHandler.Update)
	dispatch.Delete("/resource-shift-assignments/:id", assignmentHandler.Delete)

	return app
}

func origins(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "*"
	}
	return raw
}
