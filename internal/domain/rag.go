package domain

import (
	"context"
	"time"
)

// KnowledgeDocument represents an ingested curriculum document, note, or foundational guide.
type KnowledgeDocument struct {
	ID         string    `json:"id" firestore:"id"`
	Title      string    `json:"title" firestore:"title"`
	Subject    string    `json:"subject,omitempty" firestore:"subject,omitempty"`
	AuthorID   string    `json:"author_id,omitempty" firestore:"author_id,omitempty"`
	AuthorName string    `json:"author_name,omitempty" firestore:"author_name,omitempty"`
	Tags       []string  `json:"tags,omitempty" firestore:"tags,omitempty"`
	Content    string    `json:"content" firestore:"content"`
	ChunkCount int       `json:"chunk_count" firestore:"chunk_count"`
	CreatedAt  time.Time `json:"created_at" firestore:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" firestore:"updated_at"`
}

// KnowledgeChunk represents a semantically segmented passage with a 768-dim vector embedding.
type KnowledgeChunk struct {
	ID            string    `json:"id" firestore:"id"`
	DocumentID    string    `json:"document_id" firestore:"document_id"`
	DocumentTitle string    `json:"document_title" firestore:"document_title"`
	ChunkIndex    int       `json:"chunk_index" firestore:"chunk_index"`
	Text          string    `json:"text" firestore:"text"`
	Embedding     []float32 `json:"embedding" firestore:"embedding"`
	CreatedAt     time.Time `json:"created_at" firestore:"created_at"`
}

// ChunkMatch represents a search result with calculated cosine similarity score.
type ChunkMatch struct {
	Chunk      *KnowledgeChunk `json:"chunk"`
	Similarity float64         `json:"similarity"`
}

// CreateDocumentRequest DTO for uploading and ingesting notes.
type CreateDocumentRequest struct {
	Title      string   `json:"title"`
	Subject    string   `json:"subject,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Content    string   `json:"content"`
	AuthorName string   `json:"author_name,omitempty"`
}

// RAGSearchRequest DTO for testing vector search.
type RAGSearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

// RAGRepository handles storage and retrieval of documents and vector chunks.
type RAGRepository interface {
	SaveDocument(ctx context.Context, doc *KnowledgeDocument, chunks []*KnowledgeChunk) error
	GetDocument(ctx context.Context, id string) (*KnowledgeDocument, error)
	ListDocuments(ctx context.Context, limit int) ([]*KnowledgeDocument, error)
	DeleteDocument(ctx context.Context, id string) error
	ListAllChunks(ctx context.Context) ([]*KnowledgeChunk, error)
}
