package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"buddy/server/internal/middleware"
)

func TestBodyLimit_Allowed(t *testing.T) {
	limiter := middleware.BodyLimit(1024) // 1KB limit

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("unexpected error reading body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))

	smallPayload := bytes.Repeat([]byte("a"), 500)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(smallPayload))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestBodyLimit_Exceeded(t *testing.T) {
	limiter := middleware.BodyLimit(100) // 100 bytes limit

	handler := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			// Expected error from http.MaxBytesReader
			http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	largePayload := bytes.Repeat([]byte("a"), 500) // 500 bytes
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(largePayload))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status 413, got %d", rec.Code)
	}
}
