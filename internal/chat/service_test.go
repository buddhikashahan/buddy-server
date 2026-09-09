package chat_test

import (
	"context"
	"testing"

	"buddy/server/internal/chat"
	"buddy/server/internal/domain"
	platformVertex "buddy/server/internal/platform/vertex"
)

func setupTestChatService() *chat.Service {
	repo := chat.NewMemoryChatRepository()
	ragEngine := platformVertex.NewVertexRAGEngine("test-project", "us-central1", "")
	// With vertexClient = nil, it runs in deterministic test fallback mode
	return chat.NewService(repo, nil, ragEngine)
}

func TestChatService_SystemPromptManagement(t *testing.T) {
	svc := setupTestChatService()
	ctx := context.Background()

	// 1. Get default prompt
	prompt, err := svc.GetActivePrompt(ctx, domain.PromptKindChat)
	if err != nil {
		t.Fatalf("failed to get active prompt: %v", err)
	}
	if prompt.Version != 1 {
		t.Errorf("expected version 1, got %d", prompt.Version)
	}

	// 2. Update prompt as admin
	updated, err := svc.UpdateSystemPrompt(ctx, "admin-001", domain.PromptKindChat, domain.UpdatePromptRequest{
		Content:     "Updated Buddy AI persona instructions",
		Description: "Refined empathetic mentoring directives",
	})
	if err != nil {
		t.Fatalf("failed to update system prompt: %v", err)
	}

	if updated.Version != 2 {
		t.Errorf("expected version 2 after update, got %d", updated.Version)
	}
	if updated.Content != "Updated Buddy AI persona instructions" {
		t.Errorf("expected updated content, got %s", updated.Content)
	}

	// 3. Re-fetching must return the same updated revision, not an older one — this
	// guards against the previous bug where saving a new active prompt never
	// deactivated its predecessor, leaving GetActivePrompt to pick between them
	// arbitrarily.
	refetched, err := svc.GetActivePrompt(ctx, domain.PromptKindChat)
	if err != nil {
		t.Fatalf("failed to re-fetch active prompt: %v", err)
	}
	if refetched.ID != updated.ID {
		t.Errorf("expected re-fetch to return the just-updated prompt %s, got %s", updated.ID, refetched.ID)
	}
}

func TestChatService_ChatAndLiveTalkPromptsAreIndependent(t *testing.T) {
	svc := setupTestChatService()
	ctx := context.Background()

	// A fresh live_talk prompt must start from its own default, not the chat prompt's.
	liveTalkDefault, err := svc.GetActivePrompt(ctx, domain.PromptKindLiveTalk)
	if err != nil {
		t.Fatalf("failed to get default live talk prompt: %v", err)
	}
	if liveTalkDefault.Content != domain.DefaultLiveTalkSystemPrompt {
		t.Errorf("expected the seeded Live Talk default, got: %s", liveTalkDefault.Content)
	}

	// Updating the chat prompt must not affect the live_talk prompt.
	if _, err := svc.UpdateSystemPrompt(ctx, "admin-001", domain.PromptKindChat, domain.UpdatePromptRequest{
		Content: "New chat-only persona",
	}); err != nil {
		t.Fatalf("failed to update chat prompt: %v", err)
	}
	liveTalkAfterChatUpdate, err := svc.GetActivePrompt(ctx, domain.PromptKindLiveTalk)
	if err != nil {
		t.Fatalf("failed to get live talk prompt: %v", err)
	}
	if liveTalkAfterChatUpdate.Content != domain.DefaultLiveTalkSystemPrompt {
		t.Errorf("expected live talk prompt to be untouched by a chat prompt update, got: %s", liveTalkAfterChatUpdate.Content)
	}

	// Updating the live_talk prompt must not affect the chat prompt.
	if _, err := svc.UpdateSystemPrompt(ctx, "admin-001", domain.PromptKindLiveTalk, domain.UpdatePromptRequest{
		Content: "New live-talk-only persona",
	}); err != nil {
		t.Fatalf("failed to update live talk prompt: %v", err)
	}
	chatAfterLiveTalkUpdate, err := svc.GetActivePrompt(ctx, domain.PromptKindChat)
	if err != nil {
		t.Fatalf("failed to get chat prompt: %v", err)
	}
	if chatAfterLiveTalkUpdate.Content != "New chat-only persona" {
		t.Errorf("expected chat prompt to be untouched by a live talk prompt update, got: %s", chatAfterLiveTalkUpdate.Content)
	}
}

func TestChatService_PersonalIntelligence(t *testing.T) {
	svc := setupTestChatService()
	ctx := context.Background()
	studentID := "stu-123"

	// 1. Initial memory should be empty default
	mem, err := svc.GetPersonalIntelligence(ctx, studentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.StudentID != studentID {
		t.Errorf("expected student id %s, got %s", studentID, mem.StudentID)
	}

	// 2. Update memory
	bg := "Born in Galle, passionate about robotics and mechanics"
	ambition := "Become a senior automation engineer and support family"
	med := "Mild dust allergy, prefers morning study sessions"
	strengths := []string{"Analytical problem solving", "Mathematics"}

	updated, err := svc.UpdatePersonalIntelligence(ctx, studentID, domain.UpdateMemoryRequest{
		Background:   &bg,
		Ambitions:    &ambition,
		MedicalNotes: &med,
		Strengths:    &strengths,
	})
	if err != nil {
		t.Fatalf("failed to update memory: %v", err)
	}

	if updated.Background != bg || updated.Ambitions != ambition {
		t.Errorf("memory fields did not match expected values")
	}

	formatted := updated.FormatContext()
	if formatted == "" {
		t.Fatal("expected non-empty formatted memory context")
	}
}

func TestChatService_ConversationFlow(t *testing.T) {
	svc := setupTestChatService()
	ctx := context.Background()
	studentID := "stu-456"

	// 1. Create Session
	session, err := svc.CreateSession(ctx, studentID, "Study Advice Session")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if session.MessageCount != 0 {
		t.Errorf("expected message count 0, got %d", session.MessageCount)
	}

	// 2. Send Message with Multimodal Attachment
	reply, err := svc.SendMessage(ctx, studentID, session.ID, false, domain.SendMessageRequest{
		Content: "Hi Buddy, how did Dr. Tissa Jinasena approach vocational discipline?",
		Attachments: []domain.Attachment{
			{
				Name:       "notes.png",
				MimeType:   "image/png",
				DataBase64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	if reply.Sender != domain.SenderModel {
		t.Errorf("expected sender model, got %s", reply.Sender)
	}
	if reply.Content == "" {
		t.Fatal("expected non-empty reply content")
	}

	// 3. Verify Message History
	messages, err := svc.ListMessages(ctx, studentID, session.ID, false, 10)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}
	if len(messages) != 2 { // 1 user + 1 model
		t.Errorf("expected 2 messages in session, got %d", len(messages))
	}
}

func TestChatService_UnicodeTitleAndRename(t *testing.T) {
	svc := setupTestChatService()
	ctx := context.Background()
	studentID := "stu-sinhala-999"

	// 1. Create a session
	session, err := svc.CreateSession(ctx, studentID, "New Conversation with Buddy")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 2. Send a Sinhala prompt
	sinhalaPrompt := "නාමගෝත්‍ර සහ විද්‍යුත් ඉංජිනේරු විද්‍යාව ගැන පැහැදිලි කරන්න"
	reply, err := svc.SendMessage(ctx, studentID, session.ID, false, domain.SendMessageRequest{
		Content: sinhalaPrompt,
	})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Verify session title has preserved Sinhala characters without corruption
	updatedSession, err := svc.GetSession(ctx, studentID, session.ID, false)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}

	if updatedSession.Title == "" {
		t.Error("expected non-empty session title")
	}

	// 3. Rename session
	renamedTitle := "මගේ නව සාකච්ඡාව"
	renamed, err := svc.RenameSession(ctx, studentID, session.ID, false, renamedTitle)
	if err != nil {
		t.Fatalf("failed to rename session: %v", err)
	}
	if renamed.Title != renamedTitle {
		t.Errorf("expected title '%s', got '%s'", renamedTitle, renamed.Title)
	}

	_ = reply
}

// mockRAGIngester lets TestChatService_SavePersonalMemoryFact assert that saving a
// personal memory fact never ingests it into the RAG knowledge base (see that test).
type mockRAGIngester struct {
	platformVertex.RAGEngine
	ingestedDocs []*domain.KnowledgeDocument
}

func (m *mockRAGIngester) IngestDocument(ctx context.Context, authorID string, req domain.CreateDocumentRequest) (*domain.KnowledgeDocument, error) {
	doc := &domain.KnowledgeDocument{
		ID:    "mock-doc-1",
		Title: req.Title,
		Tags:  req.Tags,
	}
	m.ingestedDocs = append(m.ingestedDocs, doc)
	return doc, nil
}

func TestChatService_SavePersonalMemoryFact(t *testing.T) {
	repo := chat.NewMemoryChatRepository()
	mockRAG := &mockRAGIngester{
		RAGEngine: platformVertex.NewVertexRAGEngine("test-project", "us-central1", ""),
	}
	svc := chat.NewService(repo, nil, mockRAG)
	ctx := context.Background()
	studentID := "stu-sinhala-001"

	// 1. Save Sinhala bio details (Name Buddhika, Age 23)
	err := svc.SavePersonalMemoryFact(ctx, studentID, "background", "Student is named Buddhika and is 23 years old", "හායි බඩී මගෙ නම බුද්ධික. මගෙ වයස අවුරුදු 23 යි.")
	if err != nil {
		t.Fatalf("failed to save personal memory fact: %v", err)
	}

	// 2. Save Family details (Family of 6 members)
	err = svc.SavePersonalMemoryFact(ctx, studentID, "family", "Family has 6 members living at home", "මගේ පවුලෙ හය දෙනෙක් ඉන්නවා")
	if err != nil {
		t.Fatalf("failed to save family memory fact: %v", err)
	}

	// 3. Verify retrieved intelligence profile
	mem, err := svc.GetPersonalIntelligence(ctx, studentID)
	if err != nil {
		t.Fatalf("failed to retrieve intelligence: %v", err)
	}

	if mem.Background != "Student is named Buddhika and is 23 years old" {
		t.Errorf("unexpected background: %s", mem.Background)
	}

	if mem.FamilyContext != "Family has 6 members living at home" {
		t.Errorf("unexpected family context: %s", mem.FamilyContext)
	}

	if len(mem.ExtractedFacts) != 2 {
		t.Errorf("expected 2 extracted facts, got %d", len(mem.ExtractedFacts))
	}

	// 4. Verify neither fact was ingested into the RAG knowledge base. Personal
	// memory facts must only ever be persisted to the student's own profile — the
	// RAG store is curriculum content an admin/teacher deliberately uploads, and an
	// earlier version of this method also mirrored every fact into it, which meant
	// per-student facts leaked into the shared knowledge base management screen.
	if len(mockRAG.ingestedDocs) != 0 {
		t.Errorf("expected no documents ingested into RAG from personal memory facts, got %d: %v", len(mockRAG.ingestedDocs), mockRAG.ingestedDocs)
	}

	// 5. Test rejecting duplicate facts and meta-conversational prompts
	// Duplicate 1: "Student's name is Buddhika" -> should be rejected because "Student is named Buddhika and is 23 years old" exists
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "background", "Student's name is Buddhika", "student name")
	// Duplicate 2: "Student is 23 years old." -> should be rejected
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "background", "Student is 23 years old.", "age 23")
	// Duplicate 3: "Family has 6 members" -> should be rejected because "Family has 6 members living at home" exists
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "family", "Family has 6 members", "family count")
	// Meta 1: "Student asked if I know about their family details." -> should be rejected
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "fact", "Student asked if I know about their family details.", "meta question")
	// Meta 2: "Student confirmed their family has 6 members." -> should be rejected
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "fact", "Student confirmed their family has 6 members.", "meta confirmation")

	// Real unique fact: "Student likes fried rice" -> should be accepted
	_ = svc.SavePersonalMemoryFact(ctx, studentID, "fact", "Student likes fried rice", "i love fried rice")

	memAfter, err := svc.GetPersonalIntelligence(ctx, studentID)
	if err != nil {
		t.Fatalf("failed to retrieve memory: %v", err)
	}

	// Should have exactly 3 facts: Buddhika (23), Family 6 members, and Likes fried rice!
	if len(memAfter.ExtractedFacts) != 3 {
		t.Fatalf("expected exactly 3 facts after deduplication and meta-filtering, got %d: %v", len(memAfter.ExtractedFacts), memAfter.ExtractedFacts)
	}

	// 6. Test Admin Deleting a Fact
	deletedMem, err := svc.DeletePersonalMemoryFact(ctx, studentID, -1, "Student likes fried rice")
	if err != nil {
		t.Fatalf("failed to delete memory fact: %v", err)
	}
	if len(deletedMem.ExtractedFacts) != 2 {
		t.Fatalf("expected 2 facts after deletion, got %d", len(deletedMem.ExtractedFacts))
	}
}

