package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"buddy/server/internal/domain"
	"buddy/server/internal/middleware"
	platformAuth "buddy/server/internal/platform/auth"
)

func TestAuthenticate_DevMode(t *testing.T) {
	authClient := platformAuth.NewDevAuthClient()
	authMiddleware := middleware.Authenticate(authClient, true)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := domain.AuthUserFromContext(r.Context())
		if !ok || u == nil {
			t.Fatal("expected AuthUser in context")
		}
		if u.Role != domain.RoleAdmin {
			t.Fatalf("expected role admin, got %s", u.Role)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer dev-admin-token")
	rec := httptest.NewRecorder()

	authMiddleware(nextHandler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestRequireRoles(t *testing.T) {
	rbacMiddleware := middleware.RequireRoles(domain.RoleAdmin)

	handler := rbacMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Role is Student (Forbidden)
	studentUser := &domain.AuthUser{UID: "s1", Role: domain.RoleStudent}
	req1 := httptest.NewRequest("GET", "/admin-only", nil)
	req1 = req1.WithContext(domain.ContextWithAuthUser(req1.Context(), studentUser))
	rec1 := httptest.NewRecorder()

	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for student, got %d", rec1.Code)
	}

	// Case 2: Role is Admin (Allowed)
	adminUser := &domain.AuthUser{UID: "a1", Role: domain.RoleAdmin}
	req2 := httptest.NewRequest("GET", "/admin-only", nil)
	req2 = req2.WithContext(domain.ContextWithAuthUser(req2.Context(), adminUser))
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d", rec2.Code)
	}
}
