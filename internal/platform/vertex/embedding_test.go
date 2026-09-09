package vertex_test

import (
	"context"
	"os"
	"testing"

	"buddy/server/internal/config"

	"google.golang.org/genai"
)

func TestVertexAI_LiveEmbedding(t *testing.T) {
	cfg := config.Load()

	// This test makes real, billed calls to Vertex AI embedding models, so it only
	// runs with real local credentials in place — never in CI or a fresh clone.
	if _, err := os.Stat(cfg.FirebaseKeyPath); err != nil {
		t.Skipf("skipping live Vertex AI embedding test: no local credentials at %q", cfg.FirebaseKeyPath)
	}

	ctx := context.Background()

	_ = os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", cfg.FirebaseKeyPath)

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  cfg.GCPProjectID,
		Location: cfg.VertexLocation,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	embeddingModels := []string{
		"text-embedding-004",
		"text-embedding-005",
		"textembedding-gecko@003",
	}

	for _, model := range embeddingModels {
		t.Run(model, func(t *testing.T) {
			result, err := client.Models.EmbedContent(ctx, model, []*genai.Content{
				{
					Parts: []*genai.Part{
						{Text: "Dr. Tissa Jinasena vocational education and engineering ethics."},
					},
				},
			}, nil)

			if err != nil {
				t.Logf("Model %s failed: %v", model, err)
				return
			}

			if len(result.Embeddings) > 0 {
				vec := result.Embeddings[0].Values
				t.Logf("SUCCESS! Model %s generated embedding vector of dimension: %d (sample: %v...)", model, len(vec), vec[:3])
			}
		})
	}
}
