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

// TestMemoryChatRepository_SaveMessage_UpsertsByID guards the exact bug class behind
// background image generation: a message is first saved with ImageGenerating=true and
// no attachments, then re-saved later (same ID) once the image is ready. If SaveMessage
// only ever appended, that second save would leave two copies of the same message
// instead of updating it in place — silently hiding the generated image, since
// whichever copy the frontend happened to render might not be the updated one.
func TestMemoryChatRepository_SaveMessage_UpsertsByID(t *testing.T) {
	repo := chat.NewMemoryChatRepository()
	ctx := context.Background()

	original := &domain.ChatMessage{
		ID:              "msg-1",
		SessionID:       "session-1",
		Sender:          domain.SenderModel,
		Content:         "Here is an illustration for you.",
		ImageGenerating: true,
		CreatedAt:       time.Now(),
	}
	if err := repo.SaveMessage(ctx, original); err != nil {
		t.Fatalf("failed to save initial message: %v", err)
	}

	updated := &domain.ChatMessage{
		ID:              "msg-1",
		SessionID:       "session-1",
		Sender:          domain.SenderModel,
		Content:         original.Content,
		ImageGenerating: false,
		Attachments:     []domain.Attachment{{Name: "generated-illustration.png", MimeType: "image/png", DataBase64: "abc123"}},
		CreatedAt:       original.CreatedAt,
	}
	if err := repo.SaveMessage(ctx, updated); err != nil {
		t.Fatalf("failed to re-save message with generated image: %v", err)
	}

	msgs, err := repo.ListMessages(ctx, "session-1", 20)
	if err != nil {
		t.Fatalf("ListMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message after upsert, got %d", len(msgs))
	}
	if msgs[0].ImageGenerating {
		t.Error("expected ImageGenerating to be cleared after the upsert")
	}
	if len(msgs[0].Attachments) != 1 {
		t.Errorf("expected the generated image attachment to be present, got %d attachments", len(msgs[0].Attachments))
	}

	fetched, err := repo.GetMessage(ctx, "msg-1")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}
	if len(fetched.Attachments) != 1 {
		t.Errorf("expected GetMessage to return the updated message with its attachment, got %d attachments", len(fetched.Attachments))
	}
}

func TestMemoryChatRepository_GetMessage_NotFound(t *testing.T) {
	repo := chat.NewMemoryChatRepository()
	if _, err := repo.GetMessage(context.Background(), "does-not-exist"); err == nil {
		t.Error("expected an error looking up a message that doesn't exist")
	}
}
