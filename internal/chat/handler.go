package chat

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler handles HTTP endpoints for AI Chat, Personal Intelligence, and System Prompts.
type Handler struct {
	service *Service
}

// NewHandler creates a new Chat HTTP Handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateSession starts a new chat session.
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	var req domain.CreateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	session, err := h.service.CreateSession(r.Context(), authUser.UID, req.Title)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, session)
}

// ListSessions lists all chat sessions for the authenticated student.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sessions, err := h.service.ListSessions(r.Context(), authUser.UID, limit)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, sessions)
}

// GetSession retrieves session details.
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin

	session, err := h.service.GetSession(r.Context(), authUser.UID, sessionID, isAdmin)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, session)
}

// RenameSession updates a session title.
func (h *Handler) RenameSession(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin

	var req domain.RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	session, err := h.service.RenameSession(r.Context(), authUser.UID, sessionID, isAdmin, req.Title)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, session)
}

// DeleteSession deletes a chat session.
func (h *Handler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin

	if err := h.service.DeleteSession(r.Context(), authUser.UID, sessionID, isAdmin); err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message": "session deleted successfully",
		"id":      sessionID,
	})
}

// DeleteLastMessage rolls back the last conversation turn for rephrase/edit.
func (h *Handler) DeleteLastMessage(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin

	if err := h.service.DeleteLastTurn(r.Context(), authUser.UID, sessionID, isAdmin); err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message":    "last turn deleted successfully",
		"session_id": sessionID,
	})
}

// ListStudentSessions lists chat sessions for a specific student (Admin / Teacher only).
func (h *Handler) ListStudentSessions(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	studentID := chi.URLParam(r, "studentId")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	sessions, err := h.service.ListSessions(r.Context(), studentID, limit)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, sessions)
}

// ListMessages returns message history for a session.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	messages, err := h.service.ListMessages(r.Context(), authUser.UID, sessionID, isAdmin, limit)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, messages)
}

// SendMessage sends a user prompt (with optional multimodal attachments) to Buddy AI.
func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	isAdmin := authUser.Role == domain.RoleAdmin

	var req domain.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	reply, err := h.service.SendMessage(r.Context(), authUser.UID, sessionID, isAdmin, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, reply)
}

// GetPersonalIntelligence returns the authenticated student's personal memory context.
func (h *Handler) GetPersonalIntelligence(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	mem, err := h.service.GetPersonalIntelligence(r.Context(), authUser.UID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, mem)
}

// UpdatePersonalIntelligence updates the authenticated student's personal memory context.
func (h *Handler) UpdatePersonalIntelligence(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	var req domain.UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	mem, err := h.service.UpdatePersonalIntelligence(r.Context(), authUser.UID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, mem)
}

// GetStudentPersonalIntelligence allows Admin/Teachers to inspect a student's personal intelligence.
func (h *Handler) GetStudentPersonalIntelligence(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentId")
	if studentID == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	mem, err := h.service.GetPersonalIntelligence(r.Context(), studentID)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, mem)
}

// UpdateStudentPersonalIntelligence allows Admin/Teachers to update a student's personal intelligence.
func (h *Handler) UpdateStudentPersonalIntelligence(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentId")
	if studentID == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	var req domain.UpdateMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	mem, err := h.service.UpdatePersonalIntelligence(r.Context(), studentID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, mem)
}

// DeleteStudentMemoryFact allows Admin/Teachers to delete an individual memory observation.
func (h *Handler) DeleteStudentMemoryFact(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentId")
	if studentID == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	indexStr := r.URL.Query().Get("index")
	factText := r.URL.Query().Get("fact")
	index := -1
	if indexStr != "" {
		if parsed, err := strconv.Atoi(indexStr); err == nil {
			index = parsed
		}
	}

	// Also check request body if query params are not provided
	if factText == "" && index < 0 && r.Body != nil {
		var req domain.DeleteFactRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			factText = req.Fact
			index = req.Index
		}
	}

	mem, err := h.service.DeletePersonalMemoryFact(r.Context(), studentID, index, factText)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, mem)
}

// GetActivePrompt retrieves the current active system prompt (Admin only).
func (h *Handler) GetActivePrompt(w http.ResponseWriter, r *http.Request) {
	prompt, err := h.service.GetActivePrompt(r.Context())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, prompt)
}

// UpdateSystemPrompt creates and activates a new revision of the system prompt (Admin only).
func (h *Handler) UpdateSystemPrompt(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	var req domain.UpdatePromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	prompt, err := h.service.UpdateSystemPrompt(r.Context(), authUser.UID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, prompt)
}
