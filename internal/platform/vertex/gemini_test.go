package vertex_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"buddy/server/internal/config"
	platformVertex "buddy/server/internal/platform/vertex"
)

func TestVertexAI_LiveMarkdownResponse(t *testing.T) {
	cfg := config.Load()

	// This test makes a real, billed call to Gemini, so it only runs with real
	// local credentials in place — never in CI or a fresh clone.
	if _, err := os.Stat(cfg.FirebaseKeyPath); err != nil {
		t.Skipf("skipping live Gemini generation test: no local credentials at %q", cfg.FirebaseKeyPath)
	}

	ctx := context.Background()

	client, err := platformVertex.NewClient(ctx, cfg.GCPProjectID, cfg.VertexLocation, cfg.FirebaseKeyPath, cfg.GeminiModel)
	if err != nil {
		t.Fatalf("Failed to create Vertex AI client: %v", err)
	}
	defer client.Close()

	resp, err := client.GenerateChatResponse(
		ctx,
		"You are Buddy, an empathetic AI tutor created by the Jinasena Training Foundation, inspired by Dr. Tissa Jinasena.",
		nil,
		nil,
		nil,
		"Hi Buddy, can you give me 3 practical tips to manage exam stress in markdown?",
		nil,
		nil,
	)

	if err != nil {
		t.Fatalf("GenerateChatResponse failed: %v", err)
	}

	t.Logf("Received live Markdown from Gemini (%s):\n\n%s", cfg.GeminiModel, resp.ReplyText)

	// Verify that response contains Markdown elements
	if !strings.Contains(resp.ReplyText, "#") && !strings.Contains(resp.ReplyText, "*") && !strings.Contains(resp.ReplyText, "-") {
		t.Errorf("expected response to be formatted in Markdown, got: %s", resp.ReplyText)
	}
}
