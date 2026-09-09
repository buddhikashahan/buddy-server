package batch

import (
	"context"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
	fs "buddy/server/internal/platform/firestore"
)

// FirestoreBatchRepository implements domain.BatchRepository using Cloud Firestore.
type FirestoreBatchRepository struct {
	client *firestore.Client
}

// NewFirestoreBatchRepository initializes a new Firestore batch repository.
func NewFirestoreBatchRepository(client *firestore.Client) domain.BatchRepository {
	return &FirestoreBatchRepository{client: client}
}

func (r *FirestoreBatchRepository) CreateBatch(ctx context.Context, batch *domain.Batch) error {
	_, err := r.client.Collection(fs.CollectionBatches).Doc(batch.ID).Set(ctx, batch)
	return err
}

func (r *FirestoreBatchRepository) GetBatch(ctx context.Context, id string) (*domain.Batch, error) {
	docSnap, err := r.client.Collection(fs.CollectionBatches).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var b domain.Batch
	if err := docSnap.DataTo(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *FirestoreBatchRepository) ListBatches(ctx context.Context) ([]*domain.Batch, error) {
	iter := r.client.Collection(fs.CollectionBatches).Documents(ctx)
	var batches []*domain.Batch

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var b domain.Batch
		if err := doc.DataTo(&b); err == nil {
			batches = append(batches, &b)
		}
	}

	sort.Slice(batches, func(i, j int) bool {
		return batches[i].CreatedAt.After(batches[j].CreatedAt)
	})

	return batches, nil
}

func (r *FirestoreBatchRepository) UpdateBatch(ctx context.Context, batch *domain.Batch) error {
	batch.UpdatedAt = time.Now()
	_, err := r.client.Collection(fs.CollectionBatches).Doc(batch.ID).Set(ctx, batch)
	return err
}

func (r *FirestoreBatchRepository) DeleteBatch(ctx context.Context, id string) error {
	_, err := r.client.Collection(fs.CollectionBatches).Doc(id).Delete(ctx)
	return err
}

func (r *FirestoreBatchRepository) IncrementStudentCount(ctx context.Context, id string, delta int) error {
	_, err := r.client.Collection(fs.CollectionBatches).Doc(id).Update(ctx, []firestore.Update{
		{
			Path:  "student_count",
			Value: firestore.Increment(delta),
		},
		{
			Path:  "updated_at",
			Value: time.Now(),
		},
	})
	return err
}

// MemoryBatchRepository provides an in-memory implementation for tests/offline.
type MemoryBatchRepository struct {
	mu      sync.RWMutex
	batches map[string]*domain.Batch
}

func NewMemoryBatchRepository() domain.BatchRepository {
	return &MemoryBatchRepository{
		batches: make(map[string]*domain.Batch),
	}
}

func (r *MemoryBatchRepository) CreateBatch(ctx context.Context, batch *domain.Batch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches[batch.ID] = batch
	return nil
}

func (r *MemoryBatchRepository) GetBatch(ctx context.Context, id string) (*domain.Batch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.batches[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return b, nil
}

func (r *MemoryBatchRepository) ListBatches(ctx context.Context) ([]*domain.Batch, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*domain.Batch
	for _, b := range r.batches {
		list = append(list, b)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (r *MemoryBatchRepository) UpdateBatch(ctx context.Context, batch *domain.Batch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	batch.UpdatedAt = time.Now()
	r.batches[batch.ID] = batch
	return nil
}

func (r *MemoryBatchRepository) DeleteBatch(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.batches, id)
	return nil
}

func (r *MemoryBatchRepository) IncrementStudentCount(ctx context.Context, id string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.batches[id]; ok {
		b.StudentCount += delta
		if b.StudentCount < 0 {
			b.StudentCount = 0
		}
	}
	return nil
}
