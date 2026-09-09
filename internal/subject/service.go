package subject

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"buddy/server/internal/domain"
	"buddy/server/pkg/validator"
)

// Service implements subject and material business logic. Unlike the earlier
// per-batch "course" concept this replaced, a subject has no owner and no batch
// scoping: any signed-in user (admin, teacher, or student) can read every subject and
// its materials, and any admin or teacher can manage any subject — matching how the
// RAG knowledge base already works. Route-level RequireRoles(admin, teacher) gates
// every mutation, so this service doesn't need to re-check the caller's role itself.
type Service struct {
	repo domain.SubjectRepository
}

// NewService instantiates a subject service.
func NewService(repo domain.SubjectRepository) *Service {
	return &Service{repo: repo}
}

// CreateSubject creates a new subject.
func (s *Service) CreateSubject(ctx context.Context, req domain.CreateSubjectRequest) (*domain.Subject, error) {
	authUser, ok := domain.AuthUserFromContext(ctx)
	if !ok || authUser == nil {
		return nil, domain.ErrUnauthorized
	}

	v := validator.New()
	v.Required("title", req.Title)
	if v.HasErrors() {
		return nil, v.Error()
	}

	now := time.Now()
	subj := &domain.Subject{
		ID:            uuid.New().String(),
		Title:         strings.TrimSpace(req.Title),
		Description:   strings.TrimSpace(req.Description),
		CoverImageURL: strings.TrimSpace(req.CoverImageURL),
		CreatedBy:     authUser.UID,
		CreatedByName: authUser.DisplayName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.CreateSubject(ctx, subj); err != nil {
		return nil, fmt.Errorf("failed to create subject: %w", err)
	}
	return subj, nil
}

// GetSubject returns a single subject. Any signed-in user may read any subject.
func (s *Service) GetSubject(ctx context.Context, id string) (*domain.Subject, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	return s.repo.GetSubject(ctx, id)
}

// ListSubjects returns every subject. Any signed-in user may read the full list.
func (s *Service) ListSubjects(ctx context.Context) ([]*domain.Subject, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	return s.repo.ListSubjects(ctx)
}

// UpdateSubject modifies subject metadata.
func (s *Service) UpdateSubject(ctx context.Context, id string, req domain.UpdateSubjectRequest) (*domain.Subject, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	subj, err := s.repo.GetSubject(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		subj.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		subj.Description = strings.TrimSpace(*req.Description)
	}
	if req.CoverImageURL != nil {
		subj.CoverImageURL = strings.TrimSpace(*req.CoverImageURL)
	}
	subj.UpdatedAt = time.Now()

	if err := s.repo.UpdateSubject(ctx, subj); err != nil {
		return nil, fmt.Errorf("failed to update subject: %w", err)
	}
	return subj, nil
}

// DeleteSubject removes a subject and cascades to its materials (see
// domain.SubjectRepository.DeleteSubject).
func (s *Service) DeleteSubject(ctx context.Context, id string) error {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return domain.ErrUnauthorized
	}
	if _, err := s.repo.GetSubject(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteSubject(ctx, id)
}

// ListMaterials returns a subject's files. Any signed-in user may read them.
func (s *Service) ListMaterials(ctx context.Context, subjectID string) ([]*domain.SubjectMaterial, error) {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return nil, domain.ErrUnauthorized
	}
	if _, err := s.repo.GetSubject(ctx, subjectID); err != nil {
		return nil, err
	}
	return s.repo.ListMaterials(ctx, subjectID)
}

// CreateMaterial attaches an already-uploaded file to a subject.
func (s *Service) CreateMaterial(ctx context.Context, subjectID string, req domain.CreateMaterialRequest) (*domain.SubjectMaterial, error) {
	authUser, ok := domain.AuthUserFromContext(ctx)
	if !ok || authUser == nil {
		return nil, domain.ErrUnauthorized
	}
	if _, err := s.repo.GetSubject(ctx, subjectID); err != nil {
		return nil, err
	}

	v := validator.New()
	v.Required("title", req.Title)
	v.Required("file_name", req.FileName)
	v.Required("file_url", req.FileURL)
	if v.HasErrors() {
		return nil, v.Error()
	}

	m := &domain.SubjectMaterial{
		ID:             uuid.New().String(),
		SubjectID:      subjectID,
		Title:          strings.TrimSpace(req.Title),
		Description:    strings.TrimSpace(req.Description),
		FileName:       req.FileName,
		FileURL:        req.FileURL,
		MimeType:       req.MimeType,
		SizeBytes:      req.SizeBytes,
		UploadedBy:     authUser.UID,
		UploadedByName: authUser.DisplayName,
		CreatedAt:      time.Now(),
	}
	if err := s.repo.CreateMaterial(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to save subject material: %w", err)
	}
	return m, nil
}

// DeleteMaterial removes a file from a subject.
func (s *Service) DeleteMaterial(ctx context.Context, materialID string) error {
	if _, ok := domain.AuthUserFromContext(ctx); !ok {
		return domain.ErrUnauthorized
	}
	if _, err := s.repo.GetMaterial(ctx, materialID); err != nil {
		return err
	}
	return s.repo.DeleteMaterial(ctx, materialID)
}
