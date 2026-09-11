package user

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler exposes REST endpoints for managing students and staff.
type Handler struct {
	service *Service
}

// NewHandler creates a new user HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateStudent handles student registration requests.
func (h *Handler) CreateStudent(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateStudentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	res, err := h.service.CreateStudent(r.Context(), req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, res)
}

// ListStudents retrieves a paginated list of students.
func (h *Handler) ListStudents(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	status := domain.UserStatus(r.URL.Query().Get("status"))
	cursor := r.URL.Query().Get("cursor")

	filter := domain.ListUsersFilter{
		Status: status,
		Limit:  limit,
		Cursor: cursor,
	}

	students, nextCursor, err := h.service.ListStudents(r.Context(), filter)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	meta := response.PaginationMeta{
		TotalCount: len(students),
		Limit:      filter.Limit,
		NextCursor: nextCursor,
	}

	response.JSONWithMeta(w, http.StatusOK, students, meta)
}

// GetStudent retrieves student profile by user ID.
func (h *Handler) GetStudent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	student, err := h.service.GetStudent(r.Context(), id)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, student)
}

// UpdateStudent updates student profile fields.
func (h *Handler) UpdateStudent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	var req domain.UpdateStudentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	updated, err := h.service.UpdateStudent(r.Context(), id, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, updated)
}

// CreateStaff handles teacher or admin registration requests.
func (h *Handler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateStaffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	res, err := h.service.CreateStaff(r.Context(), req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusCreated, res)
}

// ListStaff retrieves a paginated list of staff members.
func (h *Handler) ListStaff(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	role := domain.Role(r.URL.Query().Get("role"))
	status := domain.UserStatus(r.URL.Query().Get("status"))
	cursor := r.URL.Query().Get("cursor")

	filter := domain.ListUsersFilter{
		Role:   role,
		Status: status,
		Limit:  limit,
		Cursor: cursor,
	}

	staff, nextCursor, err := h.service.ListStaff(r.Context(), filter)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	meta := response.PaginationMeta{
		TotalCount: len(staff),
		Limit:      filter.Limit,
		NextCursor: nextCursor,
	}

	response.JSONWithMeta(w, http.StatusOK, staff, meta)
}

// GetStaff retrieves staff profile by user ID.
func (h *Handler) GetStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	staff, err := h.service.GetStaff(r.Context(), id)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, staff)
}

// UpdateStaff updates staff employment profile.
func (h *Handler) UpdateStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	var req domain.UpdateStaffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	updated, err := h.service.UpdateStaff(r.Context(), id, req)
	if err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, updated)
}

// UpdateStatus modifies user account lifecycle status (active/suspended).
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	var req domain.UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Failed to parse JSON body", nil)
		return
	}

	// A role is only ever included when approving a pending account that needs
	// reassigning away from its auto-provisioned student default (see
	// user.Service.ChangeRole) — apply it before the status change so a freshly
	// (re)approved account is already under its correct role.
	if req.Role != "" {
		if err := h.service.ChangeRole(r.Context(), id, req.Role); err != nil {
			response.HandleError(w, err)
			return
		}
	}

	if err := h.service.UpdateStatus(r.Context(), id, req.Status); err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message": "user status updated successfully",
		"id":      id,
		"status":  string(req.Status),
	})
}

// DeleteUser removes a user account permanently (Admin only).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.HandleError(w, domain.ErrInvalidInput)
		return
	}

	if err := h.service.DeleteUser(r.Context(), id); err != nil {
		response.HandleError(w, err)
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{
		"message": "user deleted successfully",
		"id":      id,
	})
}

// GetCurrentUserProfile returns the profile of the currently authenticated user.
func (h *Handler) GetCurrentUserProfile(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	profile, err := h.service.GetUserProfileComposite(r.Context(), authUser.UID)
	if err != nil {
		// If user record in DB isn't yet created (e.g. fresh token login), return basic auth info
		response.JSON(w, http.StatusOK, authUser)
		return
	}

	response.JSON(w, http.StatusOK, profile)
}
