package batch

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler provides HTTP controller actions for batch management.
type Handler struct {
	service *Service
}

// NewHandler initializes a batch HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateBatch handles POST /api/v1/batches.
func (h *Handler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	batch, err := h.service.CreateBatch(r.Context(), req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, batch)
}

// GetBatch handles GET /api/v1/batches/{id}.
func (h *Handler) GetBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, err := h.service.GetBatch(r.Context(), id)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, batch)
}

// ListBatches handles GET /api/v1/batches.
func (h *Handler) ListBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := h.service.ListBatches(r.Context())
	if err != nil {
		response.HandleError(w, err)
		return
	}

	if batches == nil {
		batches = make([]*domain.Batch, 0)
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"data": batches,
	})
}

// UpdateBatch handles PUT /api/v1/batches/{id}.
func (h *Handler) UpdateBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req domain.UpdateBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON payload", nil)
		return
	}

	batch, err := h.service.UpdateBatch(r.Context(), id, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, batch)
}

// DeleteBatch handles DELETE /api/v1/batches/{id}.
func (h *Handler) DeleteBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.service.DeleteBatch(r.Context(), id); err != nil {
		response.HandleError(w, err)
		return
	}

	response.NoContent(w)
}
