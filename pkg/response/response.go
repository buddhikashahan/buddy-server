package response

import (
	"encoding/json"
	"errors"
	"net/http"

	"buddy/server/internal/domain"
)

// Response represents the standard API response structure.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

// APIError represents structured error details.
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// PaginationMeta contains page metadata for list responses.
type PaginationMeta struct {
	TotalCount int    `json:"total_count"`
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// JSON sends a standard JSON response with HTTP status code.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{
		Success: status >= 200 && status < 300,
		Data:    data,
	})
}

// JSONWithMeta sends a standard JSON response with metadata (e.g. pagination).
func JSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{
		Success: status >= 200 && status < 300,
		Data:    data,
		Meta:    meta,
	})
}

// NoContent sends a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Error sends a standardized JSON error response.
func Error(w http.ResponseWriter, status int, code, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// HandleError maps domain errors to appropriate HTTP status and standard response.
func HandleError(w http.ResponseWriter, err error) {
	var valErr *domain.ValidationError
	if errors.As(err, &valErr) {
		Error(w, http.StatusBadRequest, "VALIDATION_FAILED", "Invalid request parameters", valErr.Fields)
		return
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		Error(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	case errors.Is(err, domain.ErrUserAlreadyExists):
		Error(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, domain.ErrUnauthorized):
		Error(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error(), nil)
	case errors.Is(err, domain.ErrForbidden):
		Error(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
	case errors.Is(err, domain.ErrInvalidInput):
		Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
	case errors.Is(err, domain.ErrUserSuspended):
		Error(w, http.StatusForbidden, "USER_SUSPENDED", err.Error(), nil)
	default:
		Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
