package chat_test

import (
	"context"
	"testing"
	"time"

	"buddy/server/internal/chat"
	"buddy/server/internal/domain"
)

// TestMemoryChatRepository_DeleteSessionsByStudent verifies that deleting a student's
// sessions in bulk also removes their messages, and leaves other students' data
// untouched.
func TestMemoryChatRepository_DeleteSessionsByStudent(t *testing.T) {
	repo := chat.NewMemoryChatRepository()
	ctx := context.Background()

	seedSession := func(id, studentID string) {
		if err := repo.CreateSession(ctx, &domain.ChatSession{
			ID:        id,
			StudentID: studentID,
			Title:     "Session " + id,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("failed to create session %s: %v", id, err)
		}
		if err := repo.SaveMessage(ctx, &domain.ChatMessage{
			ID:        "msg-" + id,
			SessionID: id,
			Sender:    domain.SenderUser,
			Content:   "hello from " + id,
			CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("failed to save message for session %s: %v", id, err)
		}
	}

	seedSession("s1", "student-a")
	seedSession("s2", "student-a")
	seedSession("s3", "student-b")

	if err := repo.DeleteSessionsByStudent(ctx, "student-a"); err != nil {
		t.Fatalf("DeleteSessionsByStudent failed: %v", err)
	}

	remaining, err := repo.ListSessions(ctx, "student-a", 20)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected student-a to have no sessions left, got %d", len(remaining))
	}

	for _, sid := range []string{"s1", "s2"} {
		msgs, err := repo.ListMessages(ctx, sid, 20)
		if err != nil {
			t.Fatalf("ListMessages(%s) failed: %v", sid, err)
		}
		if len(msgs) != 0 {
			t.Errorf("expected session %s to have no messages left, got %d", sid, len(msgs))
		}
	}

	// student-b's session and message must survive untouched.
	otherSessions, err := repo.ListSessions(ctx, "student-b", 20)
	if err != nil {
		t.Fatalf("ListSessions(student-b) failed: %v", err)
	}
	if len(otherSessions) != 1 {
		t.Errorf("expected student-b to keep 1 session, got %d", len(otherSessions))
	}
	otherMsgs, err := repo.ListMessages(ctx, "s3", 20)
	if err != nil {
		t.Fatalf("ListMessages(s3) failed: %v", err)
	}
	if len(otherMsgs) != 1 {
		t.Errorf("expected session s3 to keep 1 message, got %d", len(otherMsgs))
	}
}

func TestMemoryChatRepository_DeletePersonalIntelligence(t *testing.T) {
	repo := chat.NewMemoryChatRepository()
	ctx := context.Background()

	if err := repo.SavePersonalIntelligence(ctx, &domain.StudentPersonalIntelligence{StudentID: "student-a"}); err != nil {
		t.Fatalf("SavePersonalIntelligence failed: %v", err)
	}

	if err := repo.DeletePersonalIntelligence(ctx, "student-a"); err != nil {
		t.Fatalf("DeletePersonalIntelligence failed: %v", err)
	}

	if _, err := repo.GetPersonalIntelligence(ctx, "student-a"); err == nil {
		t.Error("expected personal intelligence to be deleted")
	}
}
