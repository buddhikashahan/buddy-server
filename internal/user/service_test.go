package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	chatModule "buddy/server/internal/chat"
	"buddy/server/internal/domain"
	platformAuth "buddy/server/internal/platform/auth"
	"buddy/server/internal/user"
)

// failingAuthClient wraps DevAuthClient but fails DeleteUser, to test that a Firebase
// Auth deletion failure aborts user.Service.DeleteUser before any data is touched.
type failingAuthClient struct {
	*platformAuth.DevAuthClient
}

func (f *failingAuthClient) DeleteUser(ctx context.Context, uid string) error {
	return errors.New("simulated firebase auth outage")
}

func setupTestService() *user.Service {
	repo := user.NewMemoryRepository()
	authClient := platformAuth.NewDevAuthClient()
	return user.NewService(repo, authClient, chatModule.NewMemoryChatRepository())
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

func TestService_DeleteUser_AbortsIfAuthDeletionFails(t *testing.T) {
	repo := user.NewMemoryRepository()
	authClient := &failingAuthClient{DevAuthClient: &platformAuth.DevAuthClient{}}
	chatRepo := chatModule.NewMemoryChatRepository()
	svc := user.NewService(repo, authClient, chatRepo)
	ctx := context.Background()

	created, err := svc.CreateStudent(ctx, domain.CreateStudentRequest{
		Email:              "auth.failure@example.com",
		Password:           "password123",
		DisplayName:        "Auth Failure",
		RegistrationNumber: "STU-004",
		Grade:              "10th Grade",
	})
	if err != nil {
		t.Fatalf("failed to create student: %v", err)
	}

	if err := svc.DeleteUser(ctx, created.ID); err == nil {
		t.Fatal("expected DeleteUser to return an error when Firebase Auth deletion fails")
	}

	// The deletion must abort before touching any data, not report success while the
	// Firebase Auth identity silently lingers — so the Firestore record must survive
	// and remain retryable.
	if _, err := svc.GetStudent(ctx, created.ID); err != nil {
		t.Errorf("expected student record to survive an aborted deletion, got error: %v", err)
	}
}

func TestService_DeleteUser_CascadesChatData(t *testing.T) {
	// This test wires its own chatRepo (rather than using setupTestService) so it can
	// inspect the chat repository directly after deletion.
	repo := user.NewMemoryRepository()
	authClient := platformAuth.NewDevAuthClient()
	chatRepo := chatModule.NewMemoryChatRepository()
	svc := user.NewService(repo, authClient, chatRepo)
	ctx := context.Background()

	created, err := svc.CreateStudent(ctx, domain.CreateStudentRequest{
		Email:              "delete.cascade@example.com",
		Password:           "password123",
		DisplayName:        "Delete Cascade",
		RegistrationNumber: "STU-003",
		Grade:              "9th Grade",
	})
	if err != nil {
		t.Fatalf("failed to create student: %v", err)
	}

	session := &domain.ChatSession{
		ID:        "session-1",
		StudentID: created.ID,
		Title:     "Test Session",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := chatRepo.CreateSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	message := &domain.ChatMessage{
		ID:        "message-1",
		SessionID: session.ID,
		Sender:    domain.SenderUser,
		Content:   "hello",
		CreatedAt: time.Now(),
	}
	if err := chatRepo.SaveMessage(ctx, message); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	if err := chatRepo.SavePersonalIntelligence(ctx, &domain.StudentPersonalIntelligence{StudentID: created.ID}); err != nil {
		t.Fatalf("failed to save personal intelligence: %v", err)
	}

	if err := svc.DeleteUser(ctx, created.ID); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	if _, err := svc.GetStudent(ctx, created.ID); err == nil {
		t.Error("expected student record to be deleted")
	}

	sessions, err := chatRepo.ListSessions(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected no chat sessions to remain after user deletion, got %d", len(sessions))
	}

	messages, err := chatRepo.ListMessages(ctx, session.ID, 20)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected no chat messages to remain after user deletion, got %d", len(messages))
	}

	if _, err := chatRepo.GetPersonalIntelligence(ctx, created.ID); err == nil {
		t.Error("expected personal intelligence memory to be deleted")
	}
}
