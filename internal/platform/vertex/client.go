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
	client         *genai.Client
	liveClient     *genai.Client
	imageClient    *genai.Client
	modelName      string
	imageModelName string
	projectID      string
	location       string
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

	imageModelName := os.Getenv("VERTEX_IMAGE_MODEL")
	if imageModelName == "" {
		imageModelName = "gemini-3.1-flash-image"
	}

	// gemini-3.1-flash-image (Nano Banana 2) is served from the "global" Vertex AI
	// endpoint specifically — confirmed by hitting a real "us-central1" deployment
	// and getting back an explicit 404 "model not found in this region". This is the
	// opposite requirement from the live-audio model (which needs us-central1, see
	// liveLocation above), so the two can't share a default.
	imageLocation := os.Getenv("VERTEX_IMAGE_LOCATION")
	if imageLocation == "" {
		imageLocation = "global"
	}

	var imageClient *genai.Client
	if location == imageLocation {
		imageClient = client
	} else {
		ic, iErr := genai.NewClient(ctx, &genai.ClientConfig{
			Project:  projectID,
			Location: imageLocation,
			Backend:  genai.BackendVertexAI,
		})
		if iErr == nil {
			imageClient = ic
		} else {
			imageClient = client
		}
	}

	return &Client{
		client:         client,
		liveClient:     liveClient,
		imageClient:    imageClient,
		modelName:      modelName,
		imageModelName: imageModelName,
		projectID:      projectID,
		location:       location,
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
