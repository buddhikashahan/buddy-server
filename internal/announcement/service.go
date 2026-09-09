package announcement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"buddy/server/internal/domain"
	"buddy/server/pkg/validator"
)

// Service implements the platform-wide announcement feed. Any signed-in user may read
// it; every mutation is gated by RequireRoles(admin, teacher) at the route level, and
// (like the subject module it replaced per-course announcements in) any admin or
// teacher may edit or delete any announcement — there's no per-author ownership
// restriction to check here.
type Service struct {
	repo domain.AnnouncementRepository
}

// NewService instantiates an announcement service.
func NewService(repo domain.AnnouncementRepository) *Service {
	return &Service{repo: repo}
}

// CreateAnnouncement posts a new announcement to the shared feed.
func (s *Service) CreateAnnouncement(ctx context.Context, req domain.CreateAnnouncementRequest) (*domain.Announcement, error) {
	authUser, ok := domain.AuthUserFromContext(ctx)
	if !ok || authUser == nil {
		return nil, domain.ErrUnauthorized
	}

	v := validator.New()
	v.Required("title", req.Title)
	v.Required("content", req.Content)
	if v.HasErrors() {
		return nil, v.Error()
	}

	now := time.Now()
	a := &domain.Announcement{
		ID:         uuid.New().String(),
		Title:      strings.TrimSpace(req.Title),
		Content:    strings.TrimSpace(req.Content),
		Pinned:     req.Pinned,
		AuthorID:   authUser.UID,
		AuthorName: authUser.DisplayName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.CreateAnnouncement(ctx, a); err != nil {
		return nil, fmt.Errorf("failed to post announcement: %w", err)
	}
	return a, nil
}

// ListAnnouncements returns the full shared feed. Any signed-in user may read it.
func (s *Service) ListAnnouncements(ctx context.Context) ([]*domain.Announcement, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	return s.repo.ListAnnouncements(ctx)
}

// UpdateAnnouncement edits an announcement.
func (s *Service) UpdateAnnouncement(ctx context.Context, id string, req domain.UpdateAnnouncementRequest) (*domain.Announcement, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	a, err := s.repo.GetAnnouncement(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		a.Title = strings.TrimSpace(*req.Title)
	}
	if req.Content != nil && strings.TrimSpace(*req.Content) != "" {
		a.Content = strings.TrimSpace(*req.Content)
	}
	if req.Pinned != nil {
		a.Pinned = *req.Pinned
	}
	a.UpdatedAt = time.Now()

	if err := s.repo.UpdateAnnouncement(ctx, a); err != nil {
		return nil, fmt.Errorf("failed to update announcement: %w", err)
	}
	return a, nil
}

// DeleteAnnouncement removes an announcement from the feed.
func (s *Service) DeleteAnnouncement(ctx context.Context, id string) error {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return domain.ErrUnauthorized
	}
	if _, err := s.repo.GetAnnouncement(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteAnnouncement(ctx, id)
}
