package services

import (
	"context"
	"encoding/json"
	"log"
	"time"
	"unicode/utf8"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ThingService struct {
	things repository.ThingRepository
	groups repository.GroupRepository
	pub    Publisher
}

func NewThingService(things repository.ThingRepository, groups repository.GroupRepository, pub Publisher) *ThingService {
	return &ThingService{things: things, groups: groups, pub: pub}
}

func (s *ThingService) Create(ctx context.Context, req models.ThingCreateRequest) (*models.ThingResponse, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	typ := validateName(fields, "type", req.Type, 50)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	meta := validateMetadata(fields, req.Metadata)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if err := s.ensureGroup(ctx, req.GroupID); err != nil {
		return nil, err
	}

	thing := &models.Thing{
		Name:         name,
		Type:         typ,
		IsTrackable:  req.IsTrackable,
		DepartmentID: req.DepartmentID,
		GroupID:      req.GroupID,
		Metadata:     meta,
	}
	if err := s.things.Create(ctx, thing); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"group_id": "group not found"})
		}
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a thing with this name already exists")
		}
		return nil, err
	}
	s.publish(*thing, models.SubjectThingCreated)
	resp := thing.Response()
	return &resp, nil
}

func (s *ThingService) Update(ctx context.Context, id uuid.UUID, req models.ThingUpdateRequest) (*models.ThingResponse, error) {
	thing, err := s.things.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("thing not found")
	}
	if err != nil {
		return nil, err
	}

	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	typ := validateName(fields, "type", req.Type, 50)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	meta := validateMetadata(fields, req.Metadata)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if err := s.ensureGroup(ctx, req.GroupID); err != nil {
		return nil, err
	}

	thing.Name = name
	thing.Type = typ
	thing.IsTrackable = req.IsTrackable
	thing.DepartmentID = req.DepartmentID
	thing.GroupID = req.GroupID
	thing.Metadata = meta
	if err := s.things.Update(ctx, thing); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"group_id": "group not found"})
		}
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a thing with this name already exists")
		}
		return nil, err
	}
	s.publish(*thing, models.SubjectThingUpdated)
	resp := thing.Response()
	return &resp, nil
}

func (s *ThingService) Delete(ctx context.Context, id uuid.UUID) error {
	thing, err := s.things.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("thing not found")
	}
	if err != nil {
		return err
	}
	n, err := s.things.BlockingRefCount(ctx, id)
	if err != nil {
		return err
	}
	resources, err := s.things.ResourceCount(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 && resources > 0 {
		return Conflict("thing still has credentials or external references and is a resource")
	}
	if resources > 0 {
		return Conflict("thing is a resource")
	}
	if n > 0 {
		return Conflict("thing still has credentials or external references")
	}
	if err := s.things.Delete(ctx, id); err != nil {
		if repository.IsNotFound(err) {
			return NotFound("thing not found")
		}
		if repository.IsForeignKeyViolation(err) {
			return Conflict("thing still has credentials or external references")
		}
		return err
	}
	s.publish(*thing, models.SubjectThingDeleted)
	return nil
}

func (s *ThingService) Get(ctx context.Context, id uuid.UUID) (*models.ThingResponse, error) {
	thing, err := s.things.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("thing not found")
	}
	if err != nil {
		return nil, err
	}
	resp := thing.Response()
	return &resp, nil
}

func (s *ThingService) List(ctx context.Context, filter models.ThingListFilter) ([]models.ThingResponse, error) {
	if utf8.RuneCountInString(filter.Type) > 50 {
		return nil, Invalid("validation failed", map[string]string{"type": "type must be at most 50 characters"})
	}
	things, err := s.things.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]models.ThingResponse, 0, len(things))
	for _, thing := range things {
		out = append(out, thing.Response())
	}
	return out, nil
}

func (s *ThingService) ensureGroup(ctx context.Context, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	ok, err := s.groups.Exists(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return Invalid("validation failed", map[string]string{"group_id": "group not found"})
	}
	return nil
}

func (s *ThingService) publish(thing models.Thing, subject string) {
	if s.pub == nil {
		log.Printf("registry: publisher is nil, skipped %s for thing %s", subject, thing.ID)
		return
	}
	body, err := json.Marshal(thing.Event(subject, time.Now().UTC()))
	if err != nil {
		log.Printf("registry: failed to encode %s for thing %s: %v", subject, thing.ID, err)
		return
	}
	if err := s.pub.Publish(subject, body); err != nil {
		log.Printf("registry: failed to publish %s for thing %s: %v", subject, thing.ID, err)
	}
}
