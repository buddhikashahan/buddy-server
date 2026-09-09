package announcement

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler exposes REST endpoints for the platform-wide announcement feed.
type Handler struct {
	service *Service
}

// NewHandler creates a new announcement HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateAnnouncement handles POST /api/v1/announcements.
func (h *Handler) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	a, err := h.service.CreateAnnouncement(r.Context(), req)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

// ListAnnouncements handles GET /api/v1/announcements.
func (h *Handler) ListAnnouncements(w http.ResponseWriter, r *http.Request) {
	announcements, err := h.service.ListAnnouncements(r.Context())
	if err != nil {
		response.HandleError(w, err)
		return
	}
	if announcements == nil {
		announcements = make([]*domain.Announcement, 0)
	}
	response.JSON(w, http.StatusOK, announcements)
}

// UpdateAnnouncement handles PUT /api/v1/announcements/{id}.
func (h *Handler) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req domain.UpdateAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	a, err := h.service.UpdateAnnouncement(r.Context(), id, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, a)
}

// DeleteAnnouncement handles DELETE /api/v1/announcements/{id}.
func (h *Handler) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteAnnouncement(r.Context(), id); err != nil {
		response.HandleError(w, err)
		return
	}
	response.NoContent(w)
}
