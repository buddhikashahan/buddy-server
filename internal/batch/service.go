package batch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"buddy/server/internal/domain"
	"buddy/server/pkg/validator"
)

// Service provides domain logic for cohort/batch lifecycle operations.
type Service struct {
	repo domain.BatchRepository
}

// NewService instantiates a batch service.
func NewService(repo domain.BatchRepository) *Service {
	return &Service{repo: repo}
}

// CreateBatch provisions a new student cohort.
func (s *Service) CreateBatch(ctx context.Context, req domain.CreateBatchRequest) (*domain.Batch, error) {
	v := validator.New()
	v.Required("name", req.Name)
	if v.HasErrors() {
		return nil, v.Error()
	}

	year := req.Year
	if year <= 0 {
		year = time.Now().Year()
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "active"
	}

	now := time.Now()
	batch := &domain.Batch{
		ID:           uuid.New().String(),
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Year:         year,
		Status:       status,
		StudentCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to create batch: %w", err)
	}

	return batch, nil
}

// GetBatch returns a batch by ID.
func (s *Service) GetBatch(ctx context.Context, id string) (*domain.Batch, error) {
	if id == "" {
		return nil, domain.ErrInvalidInput
	}
	return s.repo.GetBatch(ctx, id)
}

// ListBatches returns all configured cohorts.
func (s *Service) ListBatches(ctx context.Context) ([]*domain.Batch, error) {
	return s.repo.ListBatches(ctx)
}

// UpdateBatch modifies batch metadata.
func (s *Service) UpdateBatch(ctx context.Context, id string, req domain.UpdateBatchRequest) (*domain.Batch, error) {
	batch, err := s.repo.GetBatch(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		batch.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		batch.Description = strings.TrimSpace(*req.Description)
	}
	if req.Year != nil && *req.Year > 0 {
		batch.Year = *req.Year
	}
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		batch.Status = strings.ToLower(strings.TrimSpace(*req.Status))
	}

	if err := s.repo.UpdateBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to update batch: %w", err)
	}

	return batch, nil
}

// DeleteBatch removes a cohort from persistence.
func (s *Service) DeleteBatch(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrInvalidInput
	}
	return s.repo.DeleteBatch(ctx, id)
}

// IncrementStudentCount updates the batch enrollment counter.
func (s *Service) IncrementStudentCount(ctx context.Context, id string, delta int) error {
	if id == "" {
		return nil
	}
	return s.repo.IncrementStudentCount(ctx, id, delta)
}
