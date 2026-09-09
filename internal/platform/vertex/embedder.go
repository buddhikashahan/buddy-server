package vertex

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/genai"
)

const DefaultEmbeddingModel = "text-embedding-004"

// EmbedText generates a 768-dimensional vector embedding using Vertex AI.
func (c *Client) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("cannot embed empty text")
	}

	contents := []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: text},
			},
		},
	}

	result, err := c.client.Models.EmbedContent(ctx, DefaultEmbeddingModel, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to embed content with vertex ai: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("vertex ai returned empty embeddings")
	}

	return result.Embeddings[0].Values, nil
}

// EmbedBatch generates vector embeddings for multiple texts sequentially or in batch.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, txt := range texts {
		vec, err := c.EmbedText(ctx, txt)
		if err != nil {
			return nil, fmt.Errorf("failed to embed chunk %d: %w", i, err)
		}
		results[i] = vec
	}
	return results, nil
}

// CosineSimilarity computes the similarity between two normalized or raw float32 vectors.
// Returns a value between -1.0 and 1.0 (higher means more semantically similar).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		va := float64(a[i])
		vb := float64(b[i])
		dotProduct += va * vb
		normA += va * va
		normB += vb * vb
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
