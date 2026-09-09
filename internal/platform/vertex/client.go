package vertex

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

// Client wraps Google GenAI SDK client for Vertex AI.
type Client struct {
	client     *genai.Client
	liveClient *genai.Client
	modelName  string
	projectID  string
	location   string
}

// NewClient initializes a new Vertex AI GenAI client using official Google GenAI SDK.
func NewClient(ctx context.Context, projectID, location, credentialsFile, modelName string) (*Client, error) {
	if modelName == "" {
		modelName = "gemini-3.8-flash"
	}
	if location == "" || (strings.HasPrefix(modelName, "gemini-3") && location == "us-central1") {
		location = "global"
	}

	if credentialsFile != "" {
		_ = os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credentialsFile)
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex ai genai client: %w", err)
	}

	// Live WebSocket streaming (gemini-live-2.5-flash-native-audio) requires us-central1
	liveLocation := os.Getenv("VERTEX_LIVE_LOCATION")
	if liveLocation == "" {
		liveLocation = "us-central1"
	}

	var liveClient *genai.Client
	if location == liveLocation {
		liveClient = client
	} else {
		lc, lErr := genai.NewClient(ctx, &genai.ClientConfig{
			Project:  projectID,
			Location: liveLocation,
			Backend:  genai.BackendVertexAI,
		})
		if lErr == nil {
			liveClient = lc
		} else {
			liveClient = client
		}
	}

	return &Client{
		client:     client,
		liveClient: liveClient,
		modelName:  modelName,
		projectID:  projectID,
		location:   location,
	}, nil
}

// GenAIClient returns the underlying official Google GenAI client.
func (c *Client) GenAIClient() *genai.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// LiveGenAIClient returns a Vertex AI client configured for live audio streaming (us-central1).
func (c *Client) LiveGenAIClient() *genai.Client {
	if c == nil {
		return nil
	}
	if c.liveClient != nil {
		return c.liveClient
	}
	return c.client
}

// ModelName returns the configured default Gemini model name.
func (c *Client) ModelName() string {
	if c == nil {
		return "gemini-2.5-flash"
	}
	return c.modelName
}

// Close releases Vertex AI client resources.
func (c *Client) Close() error {
	return nil
}
