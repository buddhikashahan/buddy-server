package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

func TestResponse_JSON(t *testing.T) {
	rec := httptest.NewRecorder()
	response.JSON(rec, http.StatusOK, map[string]string{"message": "success"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res response.Response
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if !res.Success {
		t.Fatal("expected response success to be true")
	}
}

func TestResponse_HandleError(t *testing.T) {
	tests := []struct {
		err          error
		expectedCode int
	}{
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrUserAlreadyExists, http.StatusConflict},
		{domain.ErrInvalidInput, http.StatusBadRequest},
		{domain.ErrUserSuspended, http.StatusForbidden},
	}

	for _, tt := range tests {
		rec := httptest.NewRecorder()
		response.HandleError(rec, tt.err)
		if rec.Code != tt.expectedCode {
			t.Errorf("for error %v expected status %d, got %d", tt.err, tt.expectedCode, rec.Code)
		}
	}
}
