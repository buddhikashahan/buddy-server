package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config encapsulates runtime configuration for the Buddy AI LMS server.
type Config struct {
	Port               string
	Env                string
	GCPProjectID       string
	FirebaseKeyPath    string
	FirestoreDBID      string
	CORSAllowedOrigins []string
	RateLimitRPS       int
	RateLimitBurst     int
	AuthDevMode        bool
	VertexLocation     string
	GeminiModel        string
	VertexRAGCorpusID  string
}

// Load reads configuration from environment variables with fallback defaults.
// Automatically discovers and loads .env file if present.
func Load() *Config {
	envDir := loadEnvFile()

	port := getEnv("PORT", "8080")
	env := getEnv("ENV", "development")
	gcpProjectID := getEnv("GCP_PROJECT_ID", "buddy-ai-lms")
	firebaseKeyPath := getEnv("FIREBASE_CREDENTIALS_FILE", "")
	firestoreDBID := getEnv("FIRESTORE_DATABASE_ID", "(default)")

	// If firebaseKeyPath is relative, resolve it relative to envDir or CWD
	if firebaseKeyPath != "" && !filepath.IsAbs(firebaseKeyPath) {
		firebaseKeyPath = resolveFilePath(firebaseKeyPath, envDir)
	}

	originsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:8080")
	origins := strings.Split(originsStr, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	rateRPS, err := strconv.Atoi(getEnv("RATE_LIMIT_RPS", "100"))
	if err != nil || rateRPS <= 0 {
		rateRPS = 100
	}

	rateBurst, err := strconv.Atoi(getEnv("RATE_LIMIT_BURST", "200"))
	if err != nil || rateBurst <= 0 {
		rateBurst = 200
	}

	authDevMode := getEnv("AUTH_DEV_MODE", "true") == "true"
	if strings.ToLower(env) == "production" || strings.ToLower(env) == "prod" {
		authDevMode = false // Strictly forbidden in production
	}
	vertexLocation := getEnv("VERTEX_LOCATION", "global")
	geminiModel := getEnv("GEMINI_MODEL", "gemini-3.8-flash")
	vertexRAGCorpusID := getEnv("VERTEX_RAG_CORPUS_ID", "")

	return &Config{
		Port:               port,
		Env:                env,
		GCPProjectID:       gcpProjectID,
		FirebaseKeyPath:    firebaseKeyPath,
		FirestoreDBID:      firestoreDBID,
		CORSAllowedOrigins: origins,
		RateLimitRPS:       rateRPS,
		RateLimitBurst:     rateBurst,
		AuthDevMode:        authDevMode,
		VertexLocation:     vertexLocation,
		GeminiModel:        geminiModel,
		VertexRAGCorpusID:  vertexRAGCorpusID,
	}
}

// loadEnvFile searches for .env in current and parent directories.
func loadEnvFile() string {
	dir, err := os.Getwd()
	if err != nil {
		_ = godotenv.Load(".env")
		return "."
	}

	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, ".env")
		if _, err := os.Stat(candidate); err == nil {
			_ = godotenv.Load(candidate)
			return dir
		}

		serverCandidate := filepath.Join(dir, "apps", "server", ".env")
		if _, err := os.Stat(serverCandidate); err == nil {
			_ = godotenv.Load(serverCandidate)
			return filepath.Join(dir, "apps", "server")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	_ = godotenv.Load(".env")
	return "."
}

func resolveFilePath(relPath, baseDir string) string {
	// First check directly
	if _, err := os.Stat(relPath); err == nil {
		return relPath
	}

	// Check relative to base directory
	candidate := filepath.Join(baseDir, relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Check apps/server
	candidate = filepath.Join("apps", "server", relPath)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	return relPath
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
