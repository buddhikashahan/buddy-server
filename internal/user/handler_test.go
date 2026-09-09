package user_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"buddy/server/internal/domain"
	"buddy/server/internal/middleware"
	platformAuth "buddy/server/internal/platform/auth"
	"buddy/server/internal/user"
)

func setupTestRouter() http.Handler {
	repo := user.NewMemoryRepository()
	authClient := platformAuth.NewDevAuthClient()
	svc := user.NewService(repo, authClient)
	h := user.NewHandler(svc)

	r := chi.NewRouter()
	r.Use(middleware.Authenticate(authClient, true))

	r.Route("/students", func(r chi.Router) {
		r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Post("/", h.CreateStudent)
		r.With(middleware.RequireRoles(domain.RoleAdmin, domain.RoleTeacher)).Get("/", h.ListStudents)
		r.Get("/{id}", h.GetStudent)
	})

	r.Route("/staff", func(r chi.Router) {
		r.With(middleware.RequireRoles(domain.RoleAdmin)).Post("/", h.CreateStaff)
		r.With(middleware.RequireRoles(domain.RoleAdmin)).Get("/", h.ListStaff)
	})

	return r
}

func TestHandler_CreateStudent_AdminAuthorized(t *testing.T) {
	router := setupTestRouter()

	payload := domain.CreateStudentRequest{
		Email:              "api.student@example.com",
		Password:           "password123",
		DisplayName:        "API Student",
		RegistrationNumber: "STU-API-001",
		Grade:              "Grade 12",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/students", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dev-admin-token")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandler_CreateStudent_StudentForbidden(t *testing.T) {
	router := setupTestRouter()

	payload := domain.CreateStudentRequest{
		Email:              "api.student2@example.com",
		Password:           "password123",
		DisplayName:        "API Student 2",
		RegistrationNumber: "STU-API-002",
		Grade:              "Grade 12",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/students", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Student token attempting to create a student
	req.Header.Set("Authorization", "Bearer dev-student-token")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden for student creating another student, got %d", rec.Code)
	}
}

func TestHandler_CreateStaff_TeacherForbidden(t *testing.T) {
	router := setupTestRouter()

	payload := domain.CreateStaffRequest{
		Email:       "teacher.sub@example.com",
		Password:    "password123",
		DisplayName: "Sub Teacher",
		Role:        domain.RoleTeacher,
		EmployeeID:  "EMP-SUB-001",
		Designation: "Teacher",
		Department:  "Science",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/staff", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Teacher trying to create staff (Only Admin should be allowed)
	req.Header.Set("Authorization", "Bearer dev-teacher-token")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden for teacher creating staff, got %d", rec.Code)
	}
}
