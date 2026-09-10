package analytics

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler exposes REST endpoints for student engagement analytics (Admin/Teacher only).
type Handler struct {
	service *Service
}

// NewHandler creates a new analytics HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetStudentProgress handles GET /api/v1/analytics/students/{studentId}.
func (h *Handler) GetStudentProgress(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "studentId")
	stats, err := h.service.GetStudentProgress(r.Context(), studentID)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, stats)
}

// GetLeaderboard handles GET /api/v1/analytics/leaderboard.
func (h *Handler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.GetLeaderboard(r.Context())
	if err != nil {
		response.HandleError(w, err)
		return
	}
	if entries == nil {
		entries = make([]*domain.LeaderboardEntry, 0)
	}
	response.JSON(w, http.StatusOK, entries)
}
