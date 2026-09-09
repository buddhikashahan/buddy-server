package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	authHandler "buddy/server/internal/auth"
	batchModule "buddy/server/internal/batch"
	chatHandler "buddy/server/internal/chat"
	"buddy/server/internal/config"
	"buddy/server/internal/domain"
	"buddy/server/internal/middleware"
	platformAuth "buddy/server/internal/platform/auth"
	platformFirestore "buddy/server/internal/platform/firestore"
	platformVertex "buddy/server/internal/platform/vertex"
	ragModule "buddy/server/internal/rag"
	"buddy/server/internal/streaming"
	"buddy/server/internal/user"
	"buddy/server/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.Env)

	log.Info("Initializing Buddy AI LMS server",
		slog.String("env", cfg.Env),
		slog.String("port", cfg.Port),
		slog.String("gemini_model", cfg.GeminiModel),
		slog.Bool("auth_dev_mode", cfg.AuthDevMode),
	)

	ctx := context.Background()

	// 1. Initialize Auth Provider
	var authClient domain.AuthClient
	var err error

	if cfg.FirebaseKeyPath != "" || (!cfg.AuthDevMode && cfg.GCPProjectID != "") {
		authClient, err = platformAuth.NewFirebaseAuthClient(ctx, cfg.GCPProjectID, cfg.FirebaseKeyPath)
		if err != nil {
			log.Warn("Failed to initialize Firebase Auth SDK, falling back to DevAuthClient", slog.String("error", err.Error()))
			authClient = platformAuth.NewDevAuthClient()
		} else {
			log.Info("Firebase Auth SDK initialized successfully")
		}
	} else {
		log.Info("Running with DevAuthClient (development / offline mode)")
		authClient = platformAuth.NewDevAuthClient()
	}

	// 2. Initialize Firestore Client
	var fsClient *firestore.Client
	var userRepo domain.UserRepository
	var chatRepo domain.ChatRepository
	var batchRepo domain.BatchRepository

	if cfg.FirebaseKeyPath != "" || (!cfg.AuthDevMode && cfg.GCPProjectID != "") {
		fsClient, err = platformFirestore.NewClient(ctx, cfg.GCPProjectID, cfg.FirestoreDBID, cfg.FirebaseKeyPath)
		if err != nil {
			log.Warn("Failed to connect to Google Cloud Firestore, falling back to MemoryRepository", slog.String("error", err.Error()))
			userRepo = user.NewMemoryRepository()
			chatRepo = chatHandler.NewMemoryChatRepository()
			batchRepo = batchModule.NewMemoryBatchRepository()
		} else {
			defer fsClient.Close()
			log.Info("Firestore client initialized successfully")
			userRepo = user.NewFirestoreRepository(fsClient)
			chatRepo = chatHandler.NewFirestoreChatRepository(fsClient)
			batchRepo = batchModule.NewFirestoreBatchRepository(fsClient)
		}
	} else {
		log.Info("Running with in-memory repositories for local development")
		userRepo = user.NewMemoryRepository()
		chatRepo = chatHandler.NewMemoryChatRepository()
		batchRepo = batchModule.NewMemoryBatchRepository()
	}

	// 3. Initialize Vertex AI Client (Gemini 2.5 Flash & Text Embedding 004)
	var vertexClient *platformVertex.Client
	if cfg.FirebaseKeyPath != "" || (!cfg.AuthDevMode && cfg.GCPProjectID != "") {
		vertexClient, err = platformVertex.NewClient(ctx, cfg.GCPProjectID, cfg.VertexLocation, cfg.FirebaseKeyPath, cfg.GeminiModel)
		if err != nil {
			log.Warn("Failed to initialize Vertex AI client, fallback mode active", slog.String("error", err.Error()))
		} else {
			defer vertexClient.Close()
			log.Info("Vertex AI client initialized successfully",
				slog.String("model", cfg.GeminiModel),
				slog.String("location", cfg.VertexLocation),
			)
		}
	}

	// 4. Initialize Real RAG Engine & Vector Repository
	var ragRepo domain.RAGRepository
	var chunkProvider ragModule.ChunkProvider

	if fsClient != nil {
		fRepo, err := ragModule.NewFirestoreRAGRepository(ctx, fsClient)
		if err == nil {
			ragRepo = fRepo
			chunkProvider = fRepo
		} else {
			mRepo := ragModule.NewMemoryRAGRepository()
			ragRepo = mRepo
			chunkProvider = mRepo
		}
	} else {
		mRepo := ragModule.NewMemoryRAGRepository()
		ragRepo = mRepo
		chunkProvider = mRepo
	}

	ragService := ragModule.NewService(ragRepo, chunkProvider, vertexClient)
	ragHandler := ragModule.NewHandler(ragService)

	// 5. Initialize Services and Handlers
	userService := user.NewService(userRepo, authClient)
	batchService := batchModule.NewService(batchRepo)
	chatService := chatHandler.NewService(chatRepo, vertexClient, ragService)

	uHandler := user.NewHandler(userService)
	bHandler := batchModule.NewHandler(batchService)
	aHandler := authHandler.NewHandler(authClient, userRepo)
	cHandler := chatHandler.NewHandler(chatService)
	liveHandler := streaming.NewHandler(authClient, vertexClient, chatService, ragService, cfg.AuthDevMode)

	// 6. Router & Middleware Stack
	r := chi.NewRouter()

	// Essential Chi middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)

	// Enforce 10MB request body limit to prevent DoS memory exhaustion
	r.Use(middleware.BodyLimit(10 << 20))

	// OWASP Security Headers
	r.Use(middleware.SecurityHeaders)

	// Structured Request Logging
	r.Use(middleware.RequestLogger(log))

	// Token Bucket Rate Limiting
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	r.Use(rateLimiter.Handler)

	// CORS Setup
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Dev-Role", "X-Dev-User-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 7. Routes
	// Health check endpoint
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`))
	})

	// API v1 Routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes & WebSocket Live Talk
		r.Post("/auth/verify", aHandler.VerifyToken)
		r.Get("/live/ws", liveHandler.HandleLiveTalk)

		// ==========================================
		// Batches & Cohorts Module (Public Read, Admin Write)
		// ==========================================
		r.Route("/batches", func(r chi.Router) {
			// Public read-only endpoints (registration dropdown & batch info)
			r.Get("/", bHandler.ListBatches)
			r.Get("/{id}", bHandler.GetBatch)

			// Admin-only mutation endpoints
			r.Group(func(r chi.Router) {
				r.Use(middleware.Authenticate(authClient, cfg.AuthDevMode))
				r.Use(middleware.RequireRoles(domain.RoleAdmin))

				r.Post("/", bHandler.CreateBatch)
				r.Put("/{id}", bHandler.UpdateBatch)
				r.Delete("/{id}", bHandler.DeleteBatch)
			})
		})

		// Protected routes requiring authentication
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(authClient, cfg.AuthDevMode))

			// Auth identity inspection
			r.Get("/auth/me", aHandler.Me)

			// Current user profile
			r.Get("/users/me", uHandler.GetCurrentUserProfile)

			// Students management
			r.Route("/users/students", func(r chi.Router) {
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/", uHandler.ListStudents)
				r.Post("/", uHandler.CreateStudent)
				// Reading a single student's full profile (phone, guardian, DOB, etc.) is staff-only;
				// students use GET /users/me for their own profile.
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/{id}", uHandler.GetStudent)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Put("/{id}", uHandler.UpdateStudent)
			})

			// Staff management (Teachers & Admins)
			r.Route("/users/staff", func(r chi.Router) {
				r.With(middleware.RequireRoles(domain.RoleAdmin)).Get("/", uHandler.ListStaff)
				r.With(middleware.RequireRoles(domain.RoleAdmin)).Post("/", uHandler.CreateStaff)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/{id}", uHandler.GetStaff)
				r.With(middleware.RequireRoles(domain.RoleAdmin)).Put("/{id}", uHandler.UpdateStaff)
			})

			// Admin lifecycle controls
			r.With(middleware.RequireRoles(domain.RoleAdmin)).Patch("/users/{id}/status", uHandler.UpdateStatus)
			r.With(middleware.RequireRoles(domain.RoleAdmin)).Delete("/users/{id}", uHandler.DeleteUser)

			// ==========================================
			// AI Chat & Mentorship Module (Buddy AI)
			// ==========================================
			r.Route("/chat", func(r chi.Router) {
				// System Prompt Management (Admin Only)
				r.With(middleware.RequireRoles(domain.RoleAdmin)).Get("/prompt", cHandler.GetActivePrompt)
				r.With(middleware.RequireRoles(domain.RoleAdmin)).Put("/prompt", cHandler.UpdateSystemPrompt)

				// Student Personal Intelligence (Self)
				r.Get("/memory", cHandler.GetPersonalIntelligence)
				r.Put("/memory", cHandler.UpdatePersonalIntelligence)

				// Student Personal Intelligence (Admin/Teacher view & update)
				// Student Personal Intelligence (Admin/Teacher view, update, & delete facts)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/memory/{studentId}", cHandler.GetStudentPersonalIntelligence)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Put("/memory/{studentId}", cHandler.UpdateStudentPersonalIntelligence)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Delete("/memory/{studentId}/fact", cHandler.DeleteStudentMemoryFact)

				// Student Chat Sessions inspection (Admin/Teacher)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/students/{studentId}/sessions", cHandler.ListStudentSessions)

				// Conversations & Sessions
				r.Post("/sessions", cHandler.CreateSession)
				r.Get("/sessions", cHandler.ListSessions)
				r.Get("/sessions/{id}", cHandler.GetSession)
				r.Patch("/sessions/{id}", cHandler.RenameSession)
				r.Delete("/sessions/{id}", cHandler.DeleteSession)
				r.Get("/sessions/{id}/messages", cHandler.ListMessages)
				r.Post("/sessions/{id}/messages", cHandler.SendMessage)
				r.Delete("/sessions/{id}/messages/last", cHandler.DeleteLastMessage)
			})

			// ==========================================
			// RAG Knowledge Base & Notes Module
			// ==========================================
			r.Route("/rag", func(r chi.Router) {
				// Document Ingestion & Management (Admin & Teachers)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Post("/documents", ragHandler.IngestDocument)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Post("/upload", ragHandler.UploadDocument)
				r.Get("/documents", ragHandler.ListDocuments)
				r.Get("/documents/{id}", ragHandler.GetDocument)
				r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Delete("/documents/{id}", ragHandler.DeleteDocument)

				// Semantic Vector Search (Direct Testing)
				r.Post("/search", ragHandler.Search)
			})
		})
	})

	// 8. Graceful Server Startup & Shutdown
	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second, // Prevent Slowloris attacks
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("Server listening for connections", slog.String("addr", addr))
		serverErrors <- srv.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server encountered fatal error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case sig := <-shutdown:
		log.Info("Shutdown signal received, draining connections", slog.String("signal", sig.String()))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Error("Graceful shutdown failed, forcing close", slog.String("error", err.Error()))
			_ = srv.Close()
		}
		log.Info("Server gracefully stopped")
	}
}
