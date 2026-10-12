package services

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ExternalRefService struct {
	refs   repository.ExternalRefRepository
	things repository.ThingRepository
}

func NewExternalRefService(refs repository.ExternalRefRepository, things repository.ThingRepository) *ExternalRefService {
	return &ExternalRefService{refs: refs, things: things}
}

// Upsert links a thing to one id in an external system. The same thing and system keep a single row.
func (s *ExternalRefService) Upsert(ctx context.Context, thingID uuid.UUID, req models.ExternalRefRequest) (*models.ThingExternalRef, bool, error) {
	fields := map[string]string{}
	system := strings.TrimSpace(req.ExternalSystem)
	if system == "" {
		system = models.ExternalSystemTraccar
	}
	if utf8.RuneCountInString(system) > 50 {
		fields["external_system"] = "external_system must be at most 50 characters"
	}
	externalID := strings.TrimSpace(req.ExternalID)
	switch {
	case externalID == "":
		fields["external_id"] = "external_id is required"
	case utf8.RuneCountInString(externalID) > 255:
		fields["external_id"] = "external_id must be at most 255 characters"
	}
	if len(fields) > 0 {
		return nil, false, Invalid("validation failed", fields)
	}

	exists, err := s.things.Exists(ctx, thingID)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, NotFound("thing not found")
	}

	owners, err := s.refs.CountOtherOwners(ctx, system, externalID, thingID)
	if err != nil {
		return nil, false, err
	}
	if owners > 0 {
		return nil, false, Conflict("external id is already linked to another thing")
	}

	existing, err := s.refs.FindByThingSystem(ctx, thingID, system)
	if err != nil && !repository.IsNotFound(err) {
		return nil, false, err
	}
	if existing == nil {
		ref := &models.ThingExternalRef{
			ThingID:        thingID,
			ExternalSystem: system,
			ExternalID:     externalID,
		}
		if err := s.refs.Create(ctx, ref); err != nil {
			if repository.IsUniqueViolation(err) {
				return nil, false, Conflict("external ref already exists")
			}
			if repository.IsForeignKeyViolation(err) {
				return nil, false, NotFound("thing not found")
			}
			return nil, false, err
		}
		return ref, true, nil
	}

	syncedAt := time.Now().UTC()
	if err := s.refs.UpdateExternalID(ctx, existing.ID, externalID, syncedAt); err != nil {
		if repository.IsUniqueViolation(err) {
			return nil, false, Conflict("external ref already exists")
		}
		return nil, false, err
	}
	existing.ExternalID = externalID
	existing.SyncedAt = syncedAt
	return existing, false, nil
}
