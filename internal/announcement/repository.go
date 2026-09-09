package announcement

import (
	"context"
	"sort"
	"sync"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
)

const CollectionAnnouncements = "announcements"

// FirestoreAnnouncementRepository implements domain.AnnouncementRepository backed by
// Cloud Firestore.
type FirestoreAnnouncementRepository struct {
	client *firestore.Client
}

// NewFirestoreAnnouncementRepository initializes a Firestore-backed repository.
func NewFirestoreAnnouncementRepository(client *firestore.Client) domain.AnnouncementRepository {
	return &FirestoreAnnouncementRepository{client: client}
}

func (r *FirestoreAnnouncementRepository) CreateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	_, err := r.client.Collection(CollectionAnnouncements).Doc(a.ID).Set(ctx, a)
	return err
}

func (r *FirestoreAnnouncementRepository) GetAnnouncement(ctx context.Context, id string) (*domain.Announcement, error) {
	doc, err := r.client.Collection(CollectionAnnouncements).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var a domain.Announcement
	if err := doc.DataTo(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *FirestoreAnnouncementRepository) ListAnnouncements(ctx context.Context) ([]*domain.Announcement, error) {
	iter := r.client.Collection(CollectionAnnouncements).Documents(ctx)
	var announcements []*domain.Announcement
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var a domain.Announcement
		if err := doc.DataTo(&a); err == nil {
			announcements = append(announcements, &a)
		}
	}
	sortAnnouncements(announcements)
	return announcements, nil
}

func sortAnnouncements(announcements []*domain.Announcement) {
	// Pinned announcements first, newest first within each group.
	sort.Slice(announcements, func(i, j int) bool {
		if announcements[i].Pinned != announcements[j].Pinned {
			return announcements[i].Pinned
		}
		return announcements[i].CreatedAt.After(announcements[j].CreatedAt)
	})
}

func (r *FirestoreAnnouncementRepository) UpdateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	_, err := r.client.Collection(CollectionAnnouncements).Doc(a.ID).Set(ctx, a)
	return err
}

func (r *FirestoreAnnouncementRepository) DeleteAnnouncement(ctx context.Context, id string) error {
	_, err := r.client.Collection(CollectionAnnouncements).Doc(id).Delete(ctx)
	return err
}

// MemoryAnnouncementRepository provides thread-safe in-memory storage for unit testing
// and offline/local development.
type MemoryAnnouncementRepository struct {
	mu            sync.RWMutex
	announcements map[string]*domain.Announcement
}

// NewMemoryAnnouncementRepository initializes an empty in-memory repository.
func NewMemoryAnnouncementRepository() domain.AnnouncementRepository {
	return &MemoryAnnouncementRepository{announcements: make(map[string]*domain.Announcement)}
}

func (m *MemoryAnnouncementRepository) CreateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.announcements[a.ID] = a
	return nil
}

func (m *MemoryAnnouncementRepository) GetAnnouncement(ctx context.Context, id string) (*domain.Announcement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.announcements[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *a
	return &copy, nil
}

func (m *MemoryAnnouncementRepository) ListAnnouncements(ctx context.Context) ([]*domain.Announcement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var announcements []*domain.Announcement
	for _, a := range m.announcements {
		announcements = append(announcements, a)
	}
	sortAnnouncements(announcements)
	return announcements, nil
}

func (m *MemoryAnnouncementRepository) UpdateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.announcements[a.ID] = a
	return nil
}

func (m *MemoryAnnouncementRepository) DeleteAnnouncement(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.announcements, id)
	return nil
}
