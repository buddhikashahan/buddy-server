package subject

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler exposes REST endpoints for subjects and the materials inside them.
type Handler struct {
	service *Service
}

// NewHandler creates a new subject HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateSubject handles POST /api/v1/subjects.
func (h *Handler) CreateSubject(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	s, err := h.service.CreateSubject(r.Context(), req)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, s)
}

// ListSubjects handles GET /api/v1/subjects.
func (h *Handler) ListSubjects(w http.ResponseWriter, r *http.Request) {
	subjects, err := h.service.ListSubjects(r.Context())
	if err != nil {
		response.HandleError(w, err)
		return
	}
	if subjects == nil {
		subjects = make([]*domain.Subject, 0)
	}
	response.JSON(w, http.StatusOK, subjects)
}

// GetSubject handles GET /api/v1/subjects/{id}.
func (h *Handler) GetSubject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s, err := h.service.GetSubject(r.Context(), id)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// UpdateSubject handles PUT /api/v1/subjects/{id}.
func (h *Handler) UpdateSubject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req domain.UpdateSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	s, err := h.service.UpdateSubject(r.Context(), id, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusOK, s)
}

// DeleteSubject handles DELETE /api/v1/subjects/{id}.
func (h *Handler) DeleteSubject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteSubject(r.Context(), id); err != nil {
		response.HandleError(w, err)
		return
	}
	response.NoContent(w)
}

// ListMaterials handles GET /api/v1/subjects/{id}/materials.
func (h *Handler) ListMaterials(w http.ResponseWriter, r *http.Request) {
	subjectID := chi.URLParam(r, "id")
	materials, err := h.service.ListMaterials(r.Context(), subjectID)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	if materials == nil {
		materials = make([]*domain.SubjectMaterial, 0)
	}
	response.JSON(w, http.StatusOK, materials)
}

// CreateMaterial handles POST /api/v1/subjects/{id}/materials.
func (h *Handler) CreateMaterial(w http.ResponseWriter, r *http.Request) {
	subjectID := chi.URLParam(r, "id")
	var req domain.CreateMaterialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	m, err := h.service.CreateMaterial(r.Context(), subjectID, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.JSON(w, http.StatusCreated, m)
}

// DeleteMaterial handles DELETE /api/v1/subjects/{id}/materials/{materialId}.
func (h *Handler) DeleteMaterial(w http.ResponseWriter, r *http.Request) {
	materialID := chi.URLParam(r, "materialId")
	if err := h.service.DeleteMaterial(r.Context(), materialID); err != nil {
		response.HandleError(w, err)
		return
	}
	response.NoContent(w)
}
