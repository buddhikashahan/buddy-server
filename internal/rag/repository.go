package rag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
)

const (
	CollectionRAGDocuments = "rag_documents"
	CollectionRAGChunks    = "rag_chunks"
)

// FirestoreRAGRepository implements domain.RAGRepository with an active vector cache for zero-latency retrieval.
type FirestoreRAGRepository struct {
	client *firestore.Client
	mu     sync.RWMutex
	cache  []*domain.KnowledgeChunk
}

// NewFirestoreRAGRepository initializes a Firestore RAG repository and warms up the vector cache.
func NewFirestoreRAGRepository(ctx context.Context, client *firestore.Client) (*FirestoreRAGRepository, error) {
	repo := &FirestoreRAGRepository{
		client: client,
		cache:  make([]*domain.KnowledgeChunk, 0),
	}

	// Warm up vector cache on boot
	chunks, err := repo.ListAllChunks(ctx)
	if err == nil {
		repo.cache = chunks
	}

	return repo, nil
}

func (r *FirestoreRAGRepository) SaveDocument(ctx context.Context, doc *domain.KnowledgeDocument, chunks []*KnowledgeChunkDoc) error {
	doc.UpdatedAt = time.Now()

	// 1. Save Document record
	_, err := r.client.Collection(CollectionRAGDocuments).Doc(doc.ID).Set(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// 2. Save Chunks
	batch := r.client.Batch()
	for _, chunk := range chunks {
		chunkRef := r.client.Collection(CollectionRAGChunks).Doc(chunk.ID)
		batch.Set(chunkRef, chunk)
	}

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit chunks batch: %w", err)
	}

	// 3. Update Vector Cache
	r.mu.Lock()
	r.cache = append(r.cache, chunks...)
	r.mu.Unlock()

	return nil
}

func (r *FirestoreRAGRepository) GetDocument(ctx context.Context, id string) (*domain.KnowledgeDocument, error) {
	docSnap, err := r.client.Collection(CollectionRAGDocuments).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var doc domain.KnowledgeDocument
	if err := docSnap.DataTo(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *FirestoreRAGRepository) ListDocuments(ctx context.Context, limit int) ([]*domain.KnowledgeDocument, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.client.Collection(CollectionRAGDocuments).Limit(limit).Documents(ctx)
	var docs []*domain.KnowledgeDocument

	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var doc domain.KnowledgeDocument
		if err := docSnap.DataTo(&doc); err == nil {
			docs = append(docs, &doc)
		}
	}
	return docs, nil
}

func (r *FirestoreRAGRepository) DeleteDocument(ctx context.Context, id string) error {
	// 1. Delete document
	_, err := r.client.Collection(CollectionRAGDocuments).Doc(id).Delete(ctx)
	if err != nil {
		return err
	}

	// 2. Delete chunks
	iter := r.client.Collection(CollectionRAGChunks).Where("document_id", "==", id).Documents(ctx)
	batch := r.client.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
		batch.Delete(doc.Ref)
	}
	_, _ = batch.Commit(ctx)

	// 3. Invalidate from cache
	r.mu.Lock()
	var newCache []*domain.KnowledgeChunk
	for _, c := range r.cache {
		if c.DocumentID != id {
			newCache = append(newCache, c)
		}
	}
	r.cache = newCache
	r.mu.Unlock()

	return nil
}

func (r *FirestoreRAGRepository) ListAllChunks(ctx context.Context) ([]*domain.KnowledgeChunk, error) {
	iter := r.client.Collection(CollectionRAGChunks).Limit(1000).Documents(ctx)
	var chunks []*domain.KnowledgeChunk

	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var c domain.KnowledgeChunk
		if err := docSnap.DataTo(&c); err == nil {
			chunks = append(chunks, &c)
		}
	}
	return chunks, nil
}

// GetCachedChunks returns a snapshot of all indexed vector chunks for similarity scanning.
func (r *FirestoreRAGRepository) GetCachedChunks() []*domain.KnowledgeChunk {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := make([]*domain.KnowledgeChunk, len(r.cache))
	copy(copied, r.cache)
	return copied
}

type KnowledgeChunkDoc = domain.KnowledgeChunk

// MemoryRAGRepository is an in-memory implementation for unit testing and offline development.
type MemoryRAGRepository struct {
	mu        sync.RWMutex
	documents map[string]*domain.KnowledgeDocument
	chunks    map[string]*domain.KnowledgeChunk
}

// NewMemoryRAGRepository initializes an empty in-memory repository.
func NewMemoryRAGRepository() *MemoryRAGRepository {
	return &MemoryRAGRepository{
		documents: make(map[string]*domain.KnowledgeDocument),
		chunks:    make(map[string]*domain.KnowledgeChunk),
	}
}

func (m *MemoryRAGRepository) SaveDocument(ctx context.Context, doc *domain.KnowledgeDocument, chunks []*domain.KnowledgeChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.documents[doc.ID] = doc
	for _, c := range chunks {
		m.chunks[c.ID] = c
	}
	return nil
}

func (m *MemoryRAGRepository) GetDocument(ctx context.Context, id string) (*domain.KnowledgeDocument, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	doc, ok := m.documents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *doc
	return &copy, nil
}

func (m *MemoryRAGRepository) ListDocuments(ctx context.Context, limit int) ([]*domain.KnowledgeDocument, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var docs []*domain.KnowledgeDocument
	for _, d := range m.documents {
		docs = append(docs, d)
	}
	return docs, nil
}

func (m *MemoryRAGRepository) DeleteDocument(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.documents, id)
	for cid, c := range m.chunks {
		if c.DocumentID == id {
			delete(m.chunks, cid)
		}
	}
	return nil
}

func (m *MemoryRAGRepository) ListAllChunks(ctx context.Context) ([]*domain.KnowledgeChunk, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var chunks []*domain.KnowledgeChunk
	for _, c := range m.chunks {
		chunks = append(chunks, c)
	}
	return chunks, nil
}

func (m *MemoryRAGRepository) GetCachedChunks() []*domain.KnowledgeChunk {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var chunks []*domain.KnowledgeChunk
	for _, c := range m.chunks {
		chunks = append(chunks, c)
	}
	return chunks
}
