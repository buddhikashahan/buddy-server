package domain

import (
	"context"
	"time"
)

// Announcement is a platform-wide message a teacher or admin posts for everyone to
// see — there is one shared feed rather than one per subject, so students have a
// single place to check instead of having to open every subject individually.
type Announcement struct {
	ID         string    `json:"id" firestore:"id"`
	Title      string    `json:"title" firestore:"title"`
	Content    string    `json:"content" firestore:"content"`
	Pinned     bool      `json:"pinned" firestore:"pinned"`
	AuthorID   string    `json:"author_id" firestore:"author_id"`
	AuthorName string    `json:"author_name,omitempty" firestore:"author_name,omitempty"`
	CreatedAt  time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" firestore:"updated_at"`
}

// CreateAnnouncementRequest is the payload to post a new announcement.
type CreateAnnouncementRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Pinned  bool   `json:"pinned,omitempty"`
}

// UpdateAnnouncementRequest is the payload to edit an existing announcement.
type UpdateAnnouncementRequest struct {
	Title   *string `json:"title,omitempty"`
	Content *string `json:"content,omitempty"`
	Pinned  *bool   `json:"pinned,omitempty"`
}

// AnnouncementRepository provides persistence for the platform-wide announcement feed.
type AnnouncementRepository interface {
	CreateAnnouncement(ctx context.Context, a *Announcement) error
	GetAnnouncement(ctx context.Context, id string) (*Announcement, error)
	ListAnnouncements(ctx context.Context) ([]*Announcement, error)
	UpdateAnnouncement(ctx context.Context, a *Announcement) error
	DeleteAnnouncement(ctx context.Context, id string) error
}
