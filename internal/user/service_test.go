package user_test

import (
	"context"
	"testing"

	"buddy/server/internal/domain"
	platformAuth "buddy/server/internal/platform/auth"
	"buddy/server/internal/user"
)

func setupTestService() *user.Service {
	repo := user.NewMemoryRepository()
	authClient := platformAuth.NewDevAuthClient()
	return user.NewService(repo, authClient)
}

func TestService_CreateStudent(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := domain.CreateStudentRequest{
		Email:              "john.doe@example.com",
		Password:           "securePass123",
		DisplayName:        "John Doe",
		RegistrationNumber: "STU-2026-001",
		Grade:              "10th Grade",
		Section:            "A",
	}

	res, err := svc.CreateStudent(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error creating student: %v", err)
	}

	if res.Email != req.Email {
		t.Errorf("expected email %s, got %s", req.Email, res.Email)
	}
	if res.Role != domain.RoleStudent {
		t.Errorf("expected role %s, got %s", domain.RoleStudent, res.Role)
	}
	if res.Profile.RegistrationNumber != req.RegistrationNumber {
		t.Errorf("expected registration number %s, got %s", req.RegistrationNumber, res.Profile.RegistrationNumber)
	}

	// Test duplicate email
	_, err = svc.CreateStudent(ctx, req)
	if err == nil {
		t.Fatal("expected duplicate email error")
	}
}

func TestService_CreateStaff(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := domain.CreateStaffRequest{
		Email:       "teacher.smith@example.com",
		Password:    "password123",
		DisplayName: "Sarah Smith",
		Role:        domain.RoleTeacher,
		EmployeeID:  "EMP-001",
		Designation: "Senior Mathematics Lecturer",
		Department:  "Mathematics",
		Subjects:    []string{"Calculus", "Algebra"},
	}

	res, err := svc.CreateStaff(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error creating staff: %v", err)
	}

	if res.Email != req.Email {
		t.Errorf("expected email %s, got %s", req.Email, res.Email)
	}
	if res.Role != domain.RoleTeacher {
		t.Errorf("expected role %s, got %s", domain.RoleTeacher, res.Role)
	}
	if res.Profile.EmployeeID != req.EmployeeID {
		t.Errorf("expected employee id %s, got %s", req.EmployeeID, res.Profile.EmployeeID)
	}
}

func TestService_UpdateStatus(t *testing.T) {
	svc := setupTestService()
	ctx := context.Background()

	req := domain.CreateStudentRequest{
		Email:              "suspend.test@example.com",
		Password:           "password123",
		DisplayName:        "Suspend Test",
		RegistrationNumber: "STU-002",
		Grade:              "11th Grade",
	}

	res, err := svc.CreateStudent(ctx, req)
	if err != nil {
		t.Fatalf("failed to create student: %v", err)
	}

	// Suspend user
	err = svc.UpdateStatus(ctx, res.ID, domain.StatusSuspended)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	student, err := svc.GetStudent(ctx, res.ID)
	if err != nil {
		t.Fatalf("failed to fetch student: %v", err)
	}

	if student.Status != domain.StatusSuspended {
		t.Errorf("expected status suspended, got %s", student.Status)
	}
}
