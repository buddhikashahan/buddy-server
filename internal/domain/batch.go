package domain

import (
	"context"
	"time"
)

// Batch represents an academic cohort or training batch (e.g., "2026 Multi Skill").
type Batch struct {
	ID           string    `json:"id" firestore:"id"`
	Name         string    `json:"name" firestore:"name"`
	Description  string    `json:"description" firestore:"description"`
	Year         int       `json:"year" firestore:"year"`
	Status       string    `json:"status" firestore:"status"` // "active", "completed", "upcoming"
	StudentCount int       `json:"student_count" firestore:"student_count"`
	CreatedAt    time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" firestore:"updated_at"`
}

// CreateBatchRequest defines the payload required to create a new batch.
type CreateBatchRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Year        int    `json:"year,omitempty"`
	Status      string `json:"status,omitempty"`
}

// UpdateBatchRequest defines the payload to modify an existing batch.
type UpdateBatchRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Year        *int    `json:"year,omitempty"`
	Status      *string `json:"status,omitempty"`
}

// BatchRepository provides persistence abstractions for Batches.
type BatchRepository interface {
	CreateBatch(ctx context.Context, batch *Batch) error
	GetBatch(ctx context.Context, id string) (*Batch, error)
	ListBatches(ctx context.Context) ([]*Batch, error)
	UpdateBatch(ctx context.Context, batch *Batch) error
	DeleteBatch(ctx context.Context, id string) error
	IncrementStudentCount(ctx context.Context, id string, delta int) error
}
