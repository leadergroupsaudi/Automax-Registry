package services

import (
	"context"

	"github.com/automax/registry/internal/models"
	"github.com/automax/registry/internal/repository"
	"github.com/google/uuid"
)

type ChannelService struct {
	repo repository.ChannelRepository
}

func NewChannelService(repo repository.ChannelRepository) *ChannelService {
	return &ChannelService{repo: repo}
}

func (s *ChannelService) Create(ctx context.Context, req models.ChannelWriteRequest) (*models.ChannelResponse, error) {
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	channel := &models.Channel{Name: name}
	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, asDuplicateName(err, "a channel with this name already exists")
	}
	resp := channel.Response()
	return &resp, nil
}

func (s *ChannelService) Update(ctx context.Context, id uuid.UUID, req models.ChannelWriteRequest) (*models.ChannelResponse, error) {
	channel, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("channel not found")
	}
	if err != nil {
		return nil, err
	}
	fields := map[string]string{}
	name := validateName(fields, "name", req.Name, 255)
	if len(fields) > 0 {
		return nil, Invalid("validation failed", fields)
	}
	channel.Name = name
	if err := s.repo.Update(ctx, channel); err != nil {
		return nil, asDuplicateName(err, "a channel with this name already exists")
	}
	resp := channel.Response()
	return &resp, nil
}

func (s *ChannelService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if repository.IsNotFound(err) {
			return NotFound("channel not found")
		}
		return err
	}
	return nil
}

func (s *ChannelService) Get(ctx context.Context, id uuid.UUID) (*models.ChannelResponse, error) {
	channel, err := s.repo.FindByID(ctx, id)
	if repository.IsNotFound(err) {
		return nil, NotFound("channel not found")
	}
	if err != nil {
		return nil, err
	}
	resp := channel.Response()
	return &resp, nil
}

func (s *ChannelService) List(ctx context.Context) ([]models.ChannelResponse, error) {
	channels, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.ChannelResponse, 0, len(channels))
	for _, channel := range channels {
		out = append(out, channel.Response())
	}
	return out, nil
}
