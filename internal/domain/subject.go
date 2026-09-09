package domain

import (
	"context"
	"time"
)

// Subject is a top-level content area (e.g. "Applied Thermodynamics") that any
// teacher or admin can publish notes into and any signed-in user can browse. Unlike
// the app's Batch/StudentProfile model, a subject is deliberately not scoped to a
// batch or owned by a single teacher — it's shared library content, not a
// per-cohort class roster, so every field beyond its title, description, and cover
// image is bookkeeping metadata rather than something the caller configures.
type Subject struct {
	ID            string    `json:"id" firestore:"id"`
	Title         string    `json:"title" firestore:"title"`
	Description   string    `json:"description,omitempty" firestore:"description,omitempty"`
	CoverImageURL string    `json:"cover_image_url,omitempty" firestore:"cover_image_url,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty" firestore:"created_by,omitempty"`
	CreatedByName string    `json:"created_by_name,omitempty" firestore:"created_by_name,omitempty"`
	CreatedAt     time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" firestore:"updated_at"`
}

// CreateSubjectRequest is the payload to create a new subject. The cover image, like
// every other file in this app, is uploaded directly from the browser to Firebase
// Storage first (see storage.rules); this only carries the resulting download URL.
type CreateSubjectRequest struct {
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	CoverImageURL string `json:"cover_image_url,omitempty"`
}

// UpdateSubjectRequest is the payload to modify an existing subject.
type UpdateSubjectRequest struct {
	Title         *string `json:"title,omitempty"`
	Description   *string `json:"description,omitempty"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
}

// SubjectMaterial is a downloadable file (lecture slides, PDFs, worksheets) a teacher
// has uploaded to a subject. As with the cover image, the file itself lives in
// Firebase Storage — this only stores its metadata and download URL.
type SubjectMaterial struct {
	ID             string    `json:"id" firestore:"id"`
	SubjectID      string    `json:"subject_id" firestore:"subject_id"`
	Title          string    `json:"title" firestore:"title"`
	Description    string    `json:"description,omitempty" firestore:"description,omitempty"`
	FileName       string    `json:"file_name" firestore:"file_name"`
	FileURL        string    `json:"file_url" firestore:"file_url"`
	MimeType       string    `json:"mime_type,omitempty" firestore:"mime_type,omitempty"`
	SizeBytes      int64     `json:"size_bytes,omitempty" firestore:"size_bytes,omitempty"`
	UploadedBy     string    `json:"uploaded_by" firestore:"uploaded_by"`
	UploadedByName string    `json:"uploaded_by_name,omitempty" firestore:"uploaded_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at" firestore:"created_at"`
}

// CreateMaterialRequest is the payload to attach an already-uploaded file to a subject.
type CreateMaterialRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	FileName    string `json:"file_name"`
	FileURL     string `json:"file_url"`
	MimeType    string `json:"mime_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// SubjectRepository provides persistence for subjects and the materials inside them.
type SubjectRepository interface {
	CreateSubject(ctx context.Context, s *Subject) error
	GetSubject(ctx context.Context, id string) (*Subject, error)
	ListSubjects(ctx context.Context) ([]*Subject, error)
	UpdateSubject(ctx context.Context, s *Subject) error
	// DeleteSubject cascades to every material belonging to the subject — Firestore
	// has no cascading delete of its own, and leaving that content behind with no
	// subject to attach to would be exactly the kind of orphaned data this app has
	// already had to fix once for chat sessions on user deletion.
	DeleteSubject(ctx context.Context, id string) error

	CreateMaterial(ctx context.Context, material *SubjectMaterial) error
	GetMaterial(ctx context.Context, id string) (*SubjectMaterial, error)
	ListMaterials(ctx context.Context, subjectID string) ([]*SubjectMaterial, error)
	DeleteMaterial(ctx context.Context, id string) error
}
