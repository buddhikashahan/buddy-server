package rag_test

import (
	"context"
	"testing"

	"buddy/server/internal/domain"
	"buddy/server/internal/rag"
)

func setupTestRAGService() *rag.Service {
	repo := rag.NewMemoryRAGRepository()
	// vertexClient = nil runs in mock vector mode for deterministic unit testing
	return rag.NewService(repo, repo, nil)
}

func TestRAGService_IngestAndRetrieve(t *testing.T) {
	svc := setupTestRAGService()
	ctx := context.Background()

	// 1. Ingest sample document
	doc, err := svc.IngestDocument(ctx, "teacher-001", domain.CreateDocumentRequest{
		Title:      "Introduction to Thermodynamics",
		Subject:    "Physics & Mechanical Engineering",
		AuthorName: "Prof. Alan Smith",
		Tags:       []string{"thermodynamics", "physics", "heat", "energy"},
		Content: `The first law of thermodynamics states that energy cannot be created or destroyed, only transformed from one form to another. 
In mechanical engineering systems, heat added to a system equals the increase in internal energy plus the work done by the system.
The second law of thermodynamics asserts that the total entropy of an isolated system can never decrease over time. 
For students, understanding energy conservation is essential for solving heat engine and turbine cycles.`,
	})
	if err != nil {
		t.Fatalf("failed to ingest document: %v", err)
	}

	if doc.ChunkCount == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	// 2. Retrieve document metadata
	fetched, err := svc.GetDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("failed to get document: %v", err)
	}
	if fetched.Title != doc.Title {
		t.Errorf("expected title %s, got %s", doc.Title, fetched.Title)
	}

	// 3. List documents
	docs, err := svc.ListDocuments(ctx, 10)
	if err != nil {
		t.Fatalf("failed to list documents: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected at least 1 document in list")
	}
}

func TestRAG_ChunkText(t *testing.T) {
	longText := `Word1 Word2 Word3 Word4 Word5 Word6 Word7 Word8 Word9 Word10 
Word11 Word12 Word13 Word14 Word15 Word16 Word17 Word18 Word19 Word20`

	chunks := rag.ChunkText(longText, 5, 2)
	if len(chunks) < 3 {
		t.Errorf("expected at least 3 chunks, got %d", len(chunks))
	}
}
