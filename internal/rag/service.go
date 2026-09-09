package rag

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"buddy/server/internal/domain"
	platformVertex "buddy/server/internal/platform/vertex"
	"buddy/server/pkg/validator"
)

// ChunkProvider abstracts accessing cached vector chunks for similarity scanning.
type ChunkProvider interface {
	GetCachedChunks() []*domain.KnowledgeChunk
}

// Service provides end-to-end RAG ingestion, chunk embedding, and vector search.
type Service struct {
	repo          domain.RAGRepository
	chunkProvider ChunkProvider
	vertexClient  *platformVertex.Client
}

// NewService initializes the RAG Service.
func NewService(repo domain.RAGRepository, chunkProvider ChunkProvider, vertexClient *platformVertex.Client) *Service {
	s := &Service{
		repo:          repo,
		chunkProvider: chunkProvider,
		vertexClient:  vertexClient,
	}

	// Seed foundational knowledge if empty
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.SeedFoundationalKnowledge(ctx)
	}()

	return s
}

// IngestDocument segments content into overlapping chunks, generates vector embeddings with Vertex AI, and persists them.
func (s *Service) IngestDocument(ctx context.Context, authorID string, req domain.CreateDocumentRequest) (*domain.KnowledgeDocument, error) {
	v := validator.New()
	v.Required("title", req.Title)
	v.Required("content", req.Content)
	if v.HasErrors() {
		return nil, v.Error()
	}

	docID := uuid.New().String()
	now := time.Now()

	// 1. Chunk document
	chunkTexts := ChunkText(req.Content, 250, 40)
	if len(chunkTexts) == 0 {
		return nil, fmt.Errorf("no content chunks produced")
	}

	// 2. Generate Real Vector Embeddings with Vertex AI
	var embeddings [][]float32
	if s.vertexClient != nil {
		var err error
		embeddings, err = s.vertexClient.EmbedBatch(ctx, chunkTexts)
		if err != nil {
			return nil, fmt.Errorf("embedding generation failed: %w", err)
		}
	} else {
		// Mock vectors if offline / dev mode
		embeddings = make([][]float32, len(chunkTexts))
		for i := range embeddings {
			embeddings[i] = make([]float32, 768)
		}
	}

	// 3. Assemble KnowledgeChunk documents
	chunks := make([]*domain.KnowledgeChunk, len(chunkTexts))
	for i, txt := range chunkTexts {
		chunks[i] = &domain.KnowledgeChunk{
			ID:            fmt.Sprintf("%s-chunk-%d", docID, i),
			DocumentID:    docID,
			DocumentTitle: req.Title,
			ChunkIndex:    i,
			Text:          txt,
			Embedding:     embeddings[i],
			CreatedAt:     now,
		}
	}

	// 4. Assemble KnowledgeDocument
	doc := &domain.KnowledgeDocument{
		ID:         docID,
		Title:      req.Title,
		Subject:    req.Subject,
		AuthorID:   authorID,
		AuthorName: req.AuthorName,
		Tags:       req.Tags,
		Content:    req.Content,
		ChunkCount: len(chunks),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.repo.SaveDocument(ctx, doc, chunks); err != nil {
		return nil, fmt.Errorf("failed to save document and chunks: %w", err)
	}

	return doc, nil
}

// RetrieveContext implements vertex.RAGEngine for grounding chat messages with real vectors.
func (s *Service) RetrieveContext(ctx context.Context, query string, topK int) ([]domain.RAGSource, error) {
	return s.RetrieveGroundingSources(ctx, query, topK)
}

// RetrieveGroundingSources performs vector cosine similarity search and returns matching RAG sources for Gemini grounding.
func (s *Service) RetrieveGroundingSources(ctx context.Context, query string, topK int) ([]domain.RAGSource, error) {
	if topK <= 0 {
		topK = 3
	}

	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	matches, err := s.Search(ctx, query, topK)
	if err != nil || len(matches) == 0 {
		return nil, err
	}

	sources := make([]domain.RAGSource, len(matches))
	for i, m := range matches {
		sources[i] = domain.RAGSource{
			Title:   fmt.Sprintf("%s (Section %d)", m.Chunk.DocumentTitle, m.Chunk.ChunkIndex+1),
			Snippet: m.Chunk.Text,
			Score:   m.Similarity,
		}
	}

	return sources, nil
}

// Search performs semantic vector search against all indexed document chunks.
func (s *Service) Search(ctx context.Context, query string, topK int) ([]*domain.ChunkMatch, error) {
	if topK <= 0 {
		topK = 3
	}

	chunks := s.chunkProvider.GetCachedChunks()
	if len(chunks) == 0 {
		return nil, nil
	}

	// 1. Generate query vector embedding with Vertex AI
	var queryVec []float32
	if s.vertexClient != nil {
		var err error
		queryVec, err = s.vertexClient.EmbedText(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to embed query: %w", err)
		}
	} else {
		return nil, nil
	}

	// 2. Compute Cosine Similarity against all cached chunks
	var matches []*domain.ChunkMatch
	for _, chunk := range chunks {
		if len(chunk.Embedding) == 0 {
			continue
		}

		score := platformVertex.CosineSimilarity(queryVec, chunk.Embedding)
		if score > 0.35 { // Semantic relevance threshold
			matches = append(matches, &domain.ChunkMatch{
				Chunk:      chunk,
				Similarity: score,
			})
		}
	}

	// 3. Sort descending by similarity
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})

	if len(matches) > topK {
		matches = matches[:topK]
	}

	return matches, nil
}

// GetDocument retrieves a document metadata.
func (s *Service) GetDocument(ctx context.Context, id string) (*domain.KnowledgeDocument, error) {
	return s.repo.GetDocument(ctx, id)
}

// ListDocuments lists ingested documents.
func (s *Service) ListDocuments(ctx context.Context, limit int) ([]*domain.KnowledgeDocument, error) {
	return s.repo.ListDocuments(ctx, limit)
}

// DeleteDocument removes a document and its indexed chunks.
func (s *Service) DeleteDocument(ctx context.Context, id string) error {
	return s.repo.DeleteDocument(ctx, id)
}

// SeedFoundationalKnowledge initializes foundational teachings if no documents are found.
func (s *Service) SeedFoundationalKnowledge(ctx context.Context) error {
	existing, err := s.repo.ListDocuments(ctx, 1)
	if err == nil && len(existing) > 0 {
		return nil // Already seeded
	}

	seedDocs := []domain.CreateDocumentRequest{
		{
			Title:      "Dr. Tissa Jinasena: Principles of Holistic Education & Vocational Excellence",
			Subject:    "Institutional Values & Philosophy",
			AuthorName: "Jinasena Training Foundation",
			Tags:       []string{"jinasena", "philosophy", "mentorship", "education"},
			Content: `Dr. Tissa Jinasena (ආචාර්ය තිස්ස ජිනසේන) envisioned an educational model where theoretical knowledge meets practical vocational excellence.
He taught that true learning rests on three harmonized pillars:
1. Intellectual Competence (The Tutor): Mastery of science, engineering, and mathematics through active problem solving rather than superficial memorization.
2. Practical Self-Reliance (The Mentor): Cultivating personal responsibility, entrepreneurial spirit, disciplined craftsmanship, and work ethic to uplift oneself and one's family.
3. Spiritual Tranquility & Compassion (The Spiritual Guider): Inner stillness, mindfulness (සතිමත් බව), emotional resilience, and Metta (loving-kindness) towards all beings.

During times of academic anxiety or vocational challenge, students are guided to pause, practice mindful breathing, organize their tasks into small achievable steps, and persevere with patience and integrity.`,
		},
		{
			Title:      "Jinasena Training Foundation: Student Well-being & Counseling Charter",
			Subject:    "Student Support & Welfare",
			AuthorName: "Jinasena Training Foundation",
			Tags:       []string{"welfare", "mental-health", "counseling", "support"},
			Content: `The Jinasena Training Foundation Student Support Charter guarantees holistic care for every learner.
Recognizing that academic performance is deeply tied to personal background, health, and family dynamics, mentors and tutors must:
- Actively listen without judgment to student anxieties, domestic challenges, or exam stress.
- Provide customized learning pacing for students with learning difficulties or medical needs.
- Encourage physical wellness, adequate rest, hydration, and positive self-talk.
- Reassure students that temporary failures or obstacles are valuable stepping stones toward technical mastery and character development.`,
		},
	}

	for _, doc := range seedDocs {
		_, _ = s.IngestDocument(ctx, "system-foundation", doc)
	}

	return nil
}
