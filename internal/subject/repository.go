package subject

import (
	"context"
	"sort"
	"sync"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
)

const (
	CollectionSubjects         = "subjects"
	CollectionSubjectMaterials = "subject_materials"
)

// FirestoreSubjectRepository implements domain.SubjectRepository backed by Cloud Firestore.
type FirestoreSubjectRepository struct {
	client *firestore.Client
}

// NewFirestoreSubjectRepository initializes a Firestore-backed subject repository.
func NewFirestoreSubjectRepository(client *firestore.Client) domain.SubjectRepository {
	return &FirestoreSubjectRepository{client: client}
}

func (r *FirestoreSubjectRepository) CreateSubject(ctx context.Context, s *domain.Subject) error {
	_, err := r.client.Collection(CollectionSubjects).Doc(s.ID).Set(ctx, s)
	return err
}

func (r *FirestoreSubjectRepository) GetSubject(ctx context.Context, id string) (*domain.Subject, error) {
	doc, err := r.client.Collection(CollectionSubjects).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var s domain.Subject
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *FirestoreSubjectRepository) ListSubjects(ctx context.Context) ([]*domain.Subject, error) {
	iter := r.client.Collection(CollectionSubjects).Documents(ctx)
	var subjects []*domain.Subject
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var s domain.Subject
		if err := doc.DataTo(&s); err == nil {
			subjects = append(subjects, &s)
		}
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].CreatedAt.After(subjects[j].CreatedAt) })
	return subjects, nil
}

func (r *FirestoreSubjectRepository) UpdateSubject(ctx context.Context, s *domain.Subject) error {
	_, err := r.client.Collection(CollectionSubjects).Doc(s.ID).Set(ctx, s)
	return err
}

func (r *FirestoreSubjectRepository) DeleteSubject(ctx context.Context, id string) error {
	// Cascade delete every material that belongs to this subject — Firestore doesn't
	// do this on its own.
	matIter := r.client.Collection(CollectionSubjectMaterials).Where("subject_id", "==", id).Documents(ctx)
	for {
		doc, err := matIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
		_, _ = doc.Ref.Delete(ctx)
	}

	_, err := r.client.Collection(CollectionSubjects).Doc(id).Delete(ctx)
	return err
}

func (r *FirestoreSubjectRepository) CreateMaterial(ctx context.Context, m *domain.SubjectMaterial) error {
	_, err := r.client.Collection(CollectionSubjectMaterials).Doc(m.ID).Set(ctx, m)
	return err
}

func (r *FirestoreSubjectRepository) GetMaterial(ctx context.Context, id string) (*domain.SubjectMaterial, error) {
	doc, err := r.client.Collection(CollectionSubjectMaterials).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var m domain.SubjectMaterial
	if err := doc.DataTo(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *FirestoreSubjectRepository) ListMaterials(ctx context.Context, subjectID string) ([]*domain.SubjectMaterial, error) {
	iter := r.client.Collection(CollectionSubjectMaterials).Where("subject_id", "==", subjectID).Documents(ctx)
	var materials []*domain.SubjectMaterial
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var m domain.SubjectMaterial
		if err := doc.DataTo(&m); err == nil {
			materials = append(materials, &m)
		}
	}
	sort.Slice(materials, func(i, j int) bool { return materials[i].CreatedAt.After(materials[j].CreatedAt) })
	return materials, nil
}

func (r *FirestoreSubjectRepository) DeleteMaterial(ctx context.Context, id string) error {
	_, err := r.client.Collection(CollectionSubjectMaterials).Doc(id).Delete(ctx)
	return err
}

// MemorySubjectRepository provides thread-safe in-memory storage for unit testing and
// offline/local development.
type MemorySubjectRepository struct {
	mu        sync.RWMutex
	subjects  map[string]*domain.Subject
	materials map[string]*domain.SubjectMaterial
}

// NewMemorySubjectRepository initializes an empty in-memory subject repository.
func NewMemorySubjectRepository() domain.SubjectRepository {
	return &MemorySubjectRepository{
		subjects:  make(map[string]*domain.Subject),
		materials: make(map[string]*domain.SubjectMaterial),
	}
}

func (m *MemorySubjectRepository) CreateSubject(ctx context.Context, s *domain.Subject) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subjects[s.ID] = s
	return nil
}

func (m *MemorySubjectRepository) GetSubject(ctx context.Context, id string) (*domain.Subject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.subjects[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *s
	return &copy, nil
}

func (m *MemorySubjectRepository) ListSubjects(ctx context.Context) ([]*domain.Subject, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var subjects []*domain.Subject
	for _, s := range m.subjects {
		subjects = append(subjects, s)
	}
	sort.Slice(subjects, func(i, j int) bool { return subjects[i].CreatedAt.After(subjects[j].CreatedAt) })
	return subjects, nil
}

func (m *MemorySubjectRepository) UpdateSubject(ctx context.Context, s *domain.Subject) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subjects[s.ID] = s
	return nil
}

func (m *MemorySubjectRepository) DeleteSubject(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subjects, id)
	for mid, mat := range m.materials {
		if mat.SubjectID == id {
			delete(m.materials, mid)
		}
	}
	return nil
}

func (m *MemorySubjectRepository) CreateMaterial(ctx context.Context, mat *domain.SubjectMaterial) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.materials[mat.ID] = mat
	return nil
}

func (m *MemorySubjectRepository) GetMaterial(ctx context.Context, id string) (*domain.SubjectMaterial, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mat, ok := m.materials[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *mat
	return &copy, nil
}

func (m *MemorySubjectRepository) ListMaterials(ctx context.Context, subjectID string) ([]*domain.SubjectMaterial, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var materials []*domain.SubjectMaterial
	for _, mat := range m.materials {
		if mat.SubjectID == subjectID {
			materials = append(materials, mat)
		}
	}
	sort.Slice(materials, func(i, j int) bool { return materials[i].CreatedAt.After(materials[j].CreatedAt) })
	return materials, nil
}

func (m *MemorySubjectRepository) DeleteMaterial(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.materials, id)
	return nil
}
