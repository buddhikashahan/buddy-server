package rag

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler provides REST endpoints for RAG document ingestion and vector search.
type Handler struct {
	service *Service
}

// NewHandler initializes a new RAG HTTP Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// UploadDocument accepts multipart file uploads (.pdf, .docx, .txt, .md), extracts text, and indexes into RAG.
func (h *Handler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	// Limit to 32 MB upload
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "UPLOAD_TOO_LARGE", "Uploaded file exceeds 32MB limit", nil)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "MISSING_FILE", "A file part named 'file' is required", nil)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "READ_ERROR", "Failed to read uploaded file", nil)
		return
	}

	text, err := ExtractText(header.Filename, data)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "EXTRACT_ERROR", fmt.Sprintf("Failed to extract text from %s: %v", header.Filename, err), nil)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		ext := filepath.Ext(header.Filename)
		title = strings.TrimSuffix(header.Filename, ext)
	}

	subject := strings.TrimSpace(r.FormValue("subject"))
	if subject == "" {
		subject = "Academic Materials"
	}

	req := domain.CreateDocumentRequest{
		Title:      title,
		Subject:    subject,
		Content:    text,
		AuthorName: authUser.DisplayName,
	}

	doc, err := h.service.IngestDocument(r.Context(), authUser.UID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, doc)
}

// IngestDocument handles uploading and indexing a new knowledge document with Vertex AI vectors.
func (h *Handler) IngestDocument(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	var req domain.CreateDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	if req.AuthorName == "" {
		req.AuthorName = authUser.DisplayName
	}

	doc, err := h.service.IngestDocument(r.Context(), authUser.UID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, doc)
}

// ListDocuments returns a list of ingested knowledge documents.
func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	docs, err := h.service.ListDocuments(r.Context(), limit)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, docs)
}

// GetDocument returns a document's metadata.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	doc, err := h.service.GetDocument(r.Context(), id)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, doc)
}

// DeleteDocument removes a document and its indexed vectors.
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteDocument(r.Context(), id); err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message": "document and indexed chunks deleted successfully",
		"id":      id,
	})
}

// Search performs semantic vector similarity search on ingested knowledge chunks.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	var req domain.RAGSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Query == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Field 'query' is required", nil)
		return
	}

	matches, err := h.service.Search(r.Context(), req.Query, req.TopK)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"query":       req.Query,
		"match_count": len(matches),
		"matches":     matches,
	})
}
