package services

import (
	"context"
	"errors"
	"strings"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type GroupService struct {
	repo repository.GroupRepository
}

func NewGroupService(repo repository.GroupRepository) *GroupService {
	return &GroupService{repo: repo}
}

func (s *GroupService) Create(ctx context.Context, req models.GroupWriteRequest) (*models.GroupResponse, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if err := s.checkParent(ctx, uuid.Nil, req.ParentGroupID); err != nil {
		return nil, err
	}

	group := &models.Group{
		Name:          name,
		ParentGroupID: req.ParentGroupID,
		DepartmentID:  req.DepartmentID,
	}
	if err := s.repo.Create(ctx, group); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"parent_group_id": "parent group not found"})
		}
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a group with this name already exists")
		}
		return nil, err
	}
	resp := group.Response()
	return &resp, nil
}

func (s *GroupService) Update(ctx context.Context, id uuid.UUID, req models.GroupWriteRequest) (*models.GroupResponse, error) {
	group, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("group not found")
	}
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if req.DepartmentID == uuid.Nil {
		fields["department_id"] = "department_id is required"
	}
	if req.ParentGroupID != nil && *req.ParentGroupID == id {
		fields["parent_group_id"] = "group cannot be its own parent"
	}
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	if err := s.checkParent(ctx, id, req.ParentGroupID); err != nil {
		return nil, err
	}

	group.Name = name
	group.ParentGroupID = req.ParentGroupID
	group.DepartmentID = req.DepartmentID
	if err := s.repo.Update(ctx, group); err != nil {
		if repository.IsForeignKeyViolation(err) {
			return nil, Invalid("validation failed", map[string]string{"parent_group_id": "parent group not found"})
		}
		if repository.IsUniqueViolation(err) {
			return nil, Conflict("a group with this name already exists")
		}
		return nil, err
	}
	resp := group.Response()
	return &resp, nil
}

func (s *GroupService) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.repo.DeleteIfUnused(ctx, id)
	if repository.IsNotFound(err) {
		return NotFound("group not found")
	}
	var inUse *repository.InUseError
	if errors.As(err, &inUse) {
		return Conflict(groupDeleteMessage(inUse))
	}
	if repository.IsForeignKeyViolation(err) {
		return Conflict("Cannot delete this group: it has child groups; one or more things belong to it")
	}
	return err
}

func (s *GroupService) Get(ctx context.Context, id uuid.UUID) (*models.GroupResponse, error) {
	group, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("group not found")
	}
	if err != nil {
		return nil, err
	}
	resp := group.Response()
	return &resp, nil
}

func (s *GroupService) List(ctx context.Context, filter models.GroupListFilter) ([]models.GroupResponse, error) {
	groups, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]models.GroupResponse, 0, len(groups))
	for _, group := range groups {
		out = append(out, group.Response())
	}
	return out, nil
}

func (s *GroupService) checkParent(ctx context.Context, groupID uuid.UUID, parentID *uuid.UUID) error {
	if parentID == nil {
		return nil
	}
	cycle, err := s.repo.CreatesCycle(ctx, groupID, *parentID)
	if errors.Is(err, repository.ErrParentMissing) {
		return Invalid("validation failed", map[string]string{"parent_group_id": "parent group not found"})
	}
	if err != nil {
		return err
	}
	if cycle {
		return Invalid("validation failed", map[string]string{"parent_group_id": "parent group would create a cycle"})
	}
	return nil
}

func groupDeleteMessage(e *repository.InUseError) string {
	var parts []string
	if e.Children > 0 {
		parts = append(parts, "it has child groups")
	}
	if e.Members > 0 {
		parts = append(parts, "one or more things belong to it")
	}
	return "Cannot delete this group: " + strings.Join(parts, "; ")
}
