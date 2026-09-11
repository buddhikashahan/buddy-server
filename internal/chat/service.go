package chat

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"buddy/server/internal/domain"
	"buddy/server/internal/platform/vertex"
	"buddy/server/pkg/validator"
)

// Service coordinates AI Chat, Personal Intelligence, RAG, and System Prompts.
type Service struct {
	repo         domain.ChatRepository
	vertexClient *vertex.Client
	ragEngine    vertex.RAGEngine
}

// NewService instantiates a new Chat Service.
func NewService(repo domain.ChatRepository, vertexClient *vertex.Client, ragEngine vertex.RAGEngine) *Service {
	return &Service{
		repo:         repo,
		vertexClient: vertexClient,
		ragEngine:    ragEngine,
	}
}

// CreateSession initializes a new chat conversation for a student.
func (s *Service) CreateSession(ctx context.Context, studentID, title string) (*domain.ChatSession, error) {
	// Prevent duplicate empty sessions: return existing empty chat if available
	existingSessions, err := s.repo.ListSessions(ctx, studentID, 15)
	if err == nil {
		for _, sess := range existingSessions {
			if sess.MessageCount == 0 && !strings.HasPrefix(sess.Title, domain.LiveTalkSessionTitlePrefix) {
				return sess, nil
			}
		}
	}

	if title == "" {
		title = fmt.Sprintf("Chat with Buddy - %s", time.Now().Format("Jan 02, 15:04"))
	}

	session := &domain.ChatSession{
		ID:           uuid.New().String(),
		StudentID:    studentID,
		Title:        title,
		MessageCount: 0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a chat session and verifies ownership.
func (s *Service) GetSession(ctx context.Context, studentID, sessionID string, isAdmin bool) (*domain.ChatSession, error) {
	session, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if !isAdmin && session.StudentID != studentID {
		return nil, domain.ErrForbidden
	}

	return session, nil
}

// ListSessions retrieves chat sessions for a student.
func (s *Service) ListSessions(ctx context.Context, studentID string, limit int) ([]*domain.ChatSession, error) {
	return s.repo.ListSessions(ctx, studentID, limit)
}

// DeleteSession verifies session ownership and deletes the session. Deleting chat
// history is admin-only — a student can no longer delete their own sessions (an
// earlier version allowed any owner to delete their own session; that let students
// erase conversations a teacher/admin might later need to review).
func (s *Service) DeleteSession(ctx context.Context, studentID, sessionID string, isAdmin bool) error {
	if !isAdmin {
		return domain.ErrForbidden
	}

	session, err := s.GetSession(ctx, studentID, sessionID, isAdmin)
	if err != nil {
		return err
	}

	return s.repo.DeleteSession(ctx, session.ID)
}

// ListMessages retrieves chronologically sorted message history for a session.
func (s *Service) ListMessages(ctx context.Context, studentID, sessionID string, isAdmin bool, limit int) ([]*domain.ChatMessage, error) {
	session, err := s.GetSession(ctx, studentID, sessionID, isAdmin)
	if err != nil {
		return nil, err
	}

	return s.repo.ListMessages(ctx, session.ID, limit)
}

// SendMessage processes an incoming user message, grounds it with RAG and Personal Intelligence, and returns Buddy AI's response.
func (s *Service) SendMessage(ctx context.Context, studentID, sessionID string, isAdmin bool, req domain.SendMessageRequest) (*domain.ChatMessage, error) {
	v := validator.New()
	// Content is required UNLESS the message carries at least one attachment (e.g. a
	// voice recording or an image sent with no caption) — those are a complete message
	// on their own, so requiring text alongside them would reject a perfectly valid
	// voice-only send.
	if len(req.Attachments) == 0 {
		v.Required("content", req.Content)
	}
	if v.HasErrors() {
		return nil, v.Error()
	}

	session, err := s.GetSession(ctx, studentID, sessionID, isAdmin)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// 1. Save the user's message, and gather everything needed to generate a reply
	// (system prompt, personal memory, RAG grounding, recent history) concurrently.
	// None of these five operations depends on another's result, but run sequentially
	// they used to stack their latencies one after another — including a real network
	// round trip to Vertex AI to embed the RAG query — adding up to a second or more
	// of pure waiting before generation even starts. Running them in parallel cuts
	// that down to the slowest single one of them.
	userMsg := &domain.ChatMessage{
		ID:          uuid.New().String(),
		SessionID:   session.ID,
		Sender:      domain.SenderUser,
		Content:     req.Content,
		Attachments: req.Attachments,
		CreatedAt:   now,
	}

	var (
		wg               sync.WaitGroup
		saveUserMsgErr   error
		systemPromptText = domain.DefaultBuddySystemPrompt
		studentMemory    *domain.StudentPersonalIntelligence
		ragSources       []domain.RAGSource
		history          []*domain.ChatMessage
	)

	wg.Add(5)
	go func() {
		defer wg.Done()
		saveUserMsgErr = s.repo.SaveMessage(ctx, userMsg)
	}()
	go func() {
		defer wg.Done()
		if prompt, promptErr := s.repo.GetActivePrompt(ctx, domain.PromptKindChat); promptErr == nil && prompt != nil && prompt.Content != "" {
			systemPromptText = prompt.Content
		}
	}()
	go func() {
		defer wg.Done()
		studentMemory, _ = s.repo.GetPersonalIntelligence(ctx, session.StudentID)
	}()
	go func() {
		defer wg.Done()
		if s.ragEngine != nil {
			ragSources, _ = s.ragEngine.RetrieveContext(ctx, req.Content, 2)
		}
	}()
	go func() {
		defer wg.Done()
		history, _ = s.repo.ListMessages(ctx, session.ID, 10)
	}()
	wg.Wait()

	if saveUserMsgErr != nil {
		return nil, fmt.Errorf("failed to save user message: %w", saveUserMsgErr)
	}

	// 2. Generate AI Response via Vertex AI
	var aiResponse *domain.StructuredAIResponse

	if s.vertexClient != nil {
		memoryCallback := func(category, fact, details string) error {
			return s.SavePersonalMemoryFact(context.Background(), session.StudentID, category, fact, details)
		}

		aiResponse, err = s.vertexClient.GenerateChatResponse(
			ctx,
			systemPromptText,
			studentMemory,
			ragSources,
			history,
			req.Content,
			req.Attachments,
			memoryCallback,
			true, // enableImageGeneration — always on in text chat when Vertex AI is available
		)
		if err != nil {
			// Fallback with rich Markdown format
			aiResponse = &domain.StructuredAIResponse{
				ReplyText:     fmt.Sprintf("### 🌟 A Mindful Moment\n\nI hear you clearly, and I am right here beside you.\n\n### 💡 Guidance from Dr. Tissa Jinasena\nDr. Tissa Jinasena taught that **genuine self-reliance is built through steady, calm effort**—never through panic or stress. Every challenge you face is an opportunity to cultivate both practical mastery and inner peace.\n\n### 🎯 Action Steps for Right Now\n1. **Mindful Pause**: Take three slow, deep breaths to clear mental fog.\n2. **Break Down the Material**: Pick just *one* specific chapter or problem rather than the whole syllabus.\n3. **Active Practice**: Work through one example problem calmly step by step.\n\nTell me which specific concept or problem you'd like to work on together right now!"),
				EmotionalTone: "Empathetic, Caring & Grounding",
				Sources:       ragSources,
			}
		}
	} else {
		// Development mode simulated Markdown response
		aiResponse = &domain.StructuredAIResponse{
			ReplyText:     fmt.Sprintf("### 🌟 Hello! I am Buddy\n\nYour empathetic tutor, mentor, and friend inspired by **Dr. Tissa Jinasena** and the **Jinasena Training Foundation**.\n\n### 💡 Wisdom & Guidance\nRegarding what you shared: *\"%s\"*\n\n- **Tutor Advice**: Let us approach this with methodical curiosity.\n- **Mentorship**: Build your discipline one session at a time.\n- **Mindfulness**: Keep your focus in the present moment.\n\n### 🎯 Next Step\nWhat would you like to explore next?", req.Content),
			EmotionalTone: "Supportive & Encouraging",
			Sources:       ragSources,
		}
	}

	// 3. Save Model Message
	modelMsg := &domain.ChatMessage{
		ID:              uuid.New().String(),
		SessionID:       session.ID,
		Sender:          domain.SenderModel,
		Content:         aiResponse.ReplyText,
		ImageGenerating: aiResponse.PendingImagePrompt != "",
		CreatedAt:       time.Now(),
	}
	if err := s.repo.SaveMessage(ctx, modelMsg); err != nil {
		return nil, fmt.Errorf("failed to save model message: %w", err)
	}

	// 4. Update Session Metadata & Title (ONLY on the very first message turn of the session!)
	isFirstTurn := len(history) == 0 || session.MessageCount == 0

	session.MessageCount += 2
	session.UpdatedAt = time.Now()

	if isFirstTurn {
		newTitle := deriveSessionTitle(req.Content, aiResponse.ReplyText)
		if newTitle != "" {
			session.Title = newTitle
			modelMsg.SessionTitle = newTitle
		}

		// Asynchronously refine title with Gemini if available without stalling user response
		if s.vertexClient != nil {
			go func(sess domain.ChatSession, userContent string) {
				tCtx, tCancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer tCancel()
				t, tErr := s.vertexClient.GenerateSessionTitle(tCtx, userContent)
				if tErr == nil && strings.TrimSpace(t) != "" {
					sess.Title = strings.TrimSpace(t)
					_ = s.repo.UpdateSession(context.Background(), &sess)
				}
			}(*session, req.Content)
		}
	}

	// Session metadata (message count, title, timestamps) isn't part of what the caller
	// is waiting on — modelMsg already carries the reply and derived title — so persist
	// it in the background instead of adding one more write to the response latency.
	go func(sess domain.ChatSession) {
		_ = s.repo.UpdateSession(context.Background(), &sess)
	}(*session)

	// Image generation is slow (several seconds) and modelMsg has already been saved
	// and is about to be returned to the student with its text reply — generating the
	// image now, in the background, is what keeps that reply from being held up. The
	// frontend polls while ImageGenerating is true and picks up the attachment once
	// completeImageGeneration re-saves this same message with it (or gives up
	// gracefully if generation fails).
	if aiResponse.PendingImagePrompt != "" {
		go s.completeImageGeneration(modelMsg.ID, aiResponse.PendingImagePrompt)
	}

	return modelMsg, nil
}

// DeleteLastTurn removes the last user message and any subsequent model response from the session.
// This allows students to rephrase and edit their last submitted prompt.
func (s *Service) DeleteLastTurn(ctx context.Context, studentID, sessionID string, isAdmin bool) error {
	session, err := s.GetSession(ctx, studentID, sessionID, isAdmin)
	if err != nil {
		return err
	}

	messages, err := s.repo.ListMessages(ctx, session.ID, 100)
	if err != nil {
		return err
	}

	// Find index of last user message
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Sender == domain.SenderUser {
			lastUserIdx = i
			break
		}
	}

	if lastUserIdx == -1 {
		return fmt.Errorf("no user messages to delete in this session")
	}

	// Delete all messages from lastUserIdx onwards
	deletedCount := 0
	for i := lastUserIdx; i < len(messages); i++ {
		if err := s.repo.DeleteMessage(ctx, messages[i].ID); err == nil {
			deletedCount++
		}
	}

	session.MessageCount -= deletedCount
	if session.MessageCount < 0 {
		session.MessageCount = 0
	}
	session.UpdatedAt = time.Now()
	return s.repo.UpdateSession(ctx, session)
}

// SaveLiveMessage records a message turn generated during real-time Live Talk.
func (s *Service) SaveLiveMessage(ctx context.Context, sessionID string, sender domain.MessageSender, content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	msg := &domain.ChatMessage{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Sender:    sender,
		Content:   trimmed,
		CreatedAt: time.Now(),
	}
	if err := s.repo.SaveMessage(ctx, msg); err != nil {
		return err
	}

	session, err := s.repo.GetSession(ctx, sessionID)
	if err == nil && session != nil {
		session.MessageCount++
		session.UpdatedAt = time.Now()
		_ = s.repo.UpdateSession(ctx, session)
	}
	return nil
}

// GetPersonalIntelligence retrieves the longitudinal memory context of a student.
func (s *Service) GetPersonalIntelligence(ctx context.Context, studentID string) (*domain.StudentPersonalIntelligence, error) {
	mem, err := s.repo.GetPersonalIntelligence(ctx, studentID)
	if err != nil {
		// Return empty default profile if not yet created
		return &domain.StudentPersonalIntelligence{
			StudentID:      studentID,
			Strengths:      make([]string, 0),
			Weaknesses:     make([]string, 0),
			ExtractedFacts: make([]string, 0),
			UpdatedAt:      time.Now(),
		}, nil
	}
	// Consolidate and clean up any historical duplicate or meta facts. The caller only
	// needs the cleaned-up value returned, not the write acknowledged, so persist the
	// dedup in the background instead of adding a write to this (often latency-critical,
	// called on every chat turn) read path.
	if len(mem.ExtractedFacts) > 0 {
		cleaned := DeduplicateFacts(mem.ExtractedFacts)
		if len(cleaned) != len(mem.ExtractedFacts) {
			mem.ExtractedFacts = cleaned
			memCopy := *mem
			go func() {
				_ = s.repo.SavePersonalIntelligence(context.Background(), &memCopy)
			}()
		}
	}
	return mem, nil
}

// UpdatePersonalIntelligence allows student or mentor/admin to update personal memory fields.
func (s *Service) UpdatePersonalIntelligence(ctx context.Context, studentID string, req domain.UpdateMemoryRequest) (*domain.StudentPersonalIntelligence, error) {
	mem, err := s.GetPersonalIntelligence(ctx, studentID)
	if err != nil {
		mem = &domain.StudentPersonalIntelligence{StudentID: studentID}
	}

	if req.Background != nil {
		mem.Background = *req.Background
	}
	if req.FamilyContext != nil {
		mem.FamilyContext = *req.FamilyContext
	}
	if req.Ambitions != nil {
		mem.Ambitions = *req.Ambitions
	}
	if req.Strengths != nil {
		mem.Strengths = *req.Strengths
	}
	if req.Weaknesses != nil {
		mem.Weaknesses = *req.Weaknesses
	}
	if req.MedicalNotes != nil {
		mem.MedicalNotes = *req.MedicalNotes
	}
	if req.SpiritualBeliefs != nil {
		mem.SpiritualBeliefs = *req.SpiritualBeliefs
	}
	if req.ExtractedFacts != nil {
		mem.ExtractedFacts = DeduplicateFacts(*req.ExtractedFacts)
	}

	if err := s.repo.SavePersonalIntelligence(ctx, mem); err != nil {
		return nil, fmt.Errorf("failed to save personal intelligence: %w", err)
	}

	return mem, nil
}

// DeletePersonalMemoryFact removes a memory fact from student personal intelligence by index or fact text match.
func (s *Service) DeletePersonalMemoryFact(ctx context.Context, studentID string, index int, factText string) (*domain.StudentPersonalIntelligence, error) {
	mem, err := s.GetPersonalIntelligence(ctx, studentID)
	if err != nil || mem == nil {
		return nil, fmt.Errorf("student memory profile not found")
	}

	factText = strings.TrimSpace(factText)
	var updatedFacts []string

	if index >= 0 && index < len(mem.ExtractedFacts) && (factText == "" || mem.ExtractedFacts[index] == factText) {
		for i, f := range mem.ExtractedFacts {
			if i != index {
				updatedFacts = append(updatedFacts, f)
			}
		}
	} else if factText != "" {
		for _, f := range mem.ExtractedFacts {
			if strings.TrimSpace(f) != factText {
				updatedFacts = append(updatedFacts, f)
			}
		}
	} else {
		updatedFacts = mem.ExtractedFacts
	}

	mem.ExtractedFacts = updatedFacts
	mem.UpdatedAt = time.Now()

	if err := s.repo.SavePersonalIntelligence(ctx, mem); err != nil {
		return nil, fmt.Errorf("failed to update memory after fact deletion: %w", err)
	}

	slog.Info("[Memory Management] Deleted memory fact", slog.String("student_id", studentID), slog.String("fact", factText))
	return mem, nil
}

// generateEducationalImage runs Gemini image generation for the
// "generate_educational_image" chat tool. Called from a background goroutine kicked
// off by SendMessage after the text reply is already saved and returned — see
// completeImageGeneration. Unlike a search result, a generated image only exists as
// raw bytes with nowhere already hosting it, so — matching how a directly-uploaded
// chat attachment already works (domain.Attachment.DataBase64) — it's embedded inline
// as a base64 data URI rather than uploaded to Cloud Storage first.
func (s *Service) generateEducationalImage(ctx context.Context, prompt string) ([]domain.Attachment, error) {
	img, err := s.vertexClient.GenerateEducationalImage(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("image generation returned no image")
	}

	// Raw base64 only, no "data:...;base64," prefix — the frontend builds that prefix
	// itself from mime_type + data_base64 when rendering (see app/student/page.tsx),
	// so a stored data URI here would end up double-prefixed and fail to render.
	attachment := domain.Attachment{
		Name:       "generated-illustration." + extensionForImageMime(img.MimeType),
		MimeType:   img.MimeType,
		DataBase64: base64.StdEncoding.EncodeToString(img.Data),
		SizeBytes:  int64(len(img.Data)),
	}
	return []domain.Attachment{attachment}, nil
}

// completeImageGeneration runs in the background after SendMessage has already saved
// and returned modelMsg with ImageGenerating set. It generates the requested image and
// re-saves the same message with Attachments populated (and ImageGenerating cleared) —
// or, on failure, just clears ImageGenerating with no attachments, so the frontend's
// polling (see app/student/page.tsx) stops waiting instead of polling forever. Takes
// its own context rather than the original request's, since that one is canceled the
// moment the HTTP handler that started this goroutine returns.
func (s *Service) completeImageGeneration(messageID, prompt string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	atts, err := s.generateEducationalImage(ctx, prompt)
	if err != nil {
		slog.Warn("[Educational Images] generation failed", slog.String("prompt", prompt), slog.String("error", err.Error()))
	}

	msg, getErr := s.repo.GetMessage(ctx, messageID)
	if getErr != nil || msg == nil {
		slog.Error("[Educational Images] could not re-fetch message to attach generated image", slog.String("message_id", messageID))
		return
	}
	msg.ImageGenerating = false
	msg.Attachments = atts
	if saveErr := s.repo.SaveMessage(ctx, msg); saveErr != nil {
		slog.Error("[Educational Images] failed to save generated image onto message", slog.String("message_id", messageID), slog.String("error", saveErr.Error()))
	}
}

// extensionForImageMime maps a generated image's MIME type to a sensible file
// extension for its display name; unrecognized types fall back to .png since that's
// what Gemini image generation returns by default.
func extensionForImageMime(mimeType string) string {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}

// SavePersonalMemoryFact records a specific personal fact for a student into both Firestore Personal Intelligence
// (this is the only place a memory fact is persisted — see the comment above the
// SavePersonalIntelligence call below for why it must never also reach the RAG
// knowledge base). details is currently unused by this method itself but is kept in
// the signature since callers (chat.Service's Gemini function-calling tool, and
// streaming's Live Talk equivalent) already pass it through as a tool argument.
func (s *Service) SavePersonalMemoryFact(ctx context.Context, studentID, category, fact, details string) error {
	if strings.TrimSpace(studentID) == "" || strings.TrimSpace(fact) == "" {
		return nil
	}

	category = strings.TrimSpace(strings.ToLower(category))
	fact = strings.TrimSpace(fact)

	// Filter out non-factual, conversational, or meta statements
	if isInvalidOrMetaFact(fact) {
		slog.Debug("[Memory Persistence] Dropped conversational/meta statement", slog.String("fact", fact))
		return nil
	}

	// 1. Fetch existing student personal intelligence
	mem, err := s.GetPersonalIntelligence(ctx, studentID)
	if err != nil || mem == nil {
		mem = &domain.StudentPersonalIntelligence{
			StudentID:      studentID,
			Strengths:      make([]string, 0),
			Weaknesses:     make([]string, 0),
			ExtractedFacts: make([]string, 0),
		}
	}

	// Prevent duplicate or redundant facts
	if isDuplicateOrRedundantFact(mem.ExtractedFacts, fact) {
		slog.Debug("[Memory Persistence] Skipped duplicate/redundant fact", slog.String("fact", fact))
		return nil
	}

	// 2. Merge into categorical fields (fixed: proper single else-if logic)
	switch category {
	case "background":
		if mem.Background == "" {
			mem.Background = fact
		} else if !strings.Contains(mem.Background, fact) && !isDuplicateOrRedundantFact([]string{mem.Background}, fact) {
			mem.Background = mem.Background + "; " + fact
		}
	case "family":
		if mem.FamilyContext == "" {
			mem.FamilyContext = fact
		} else if !strings.Contains(mem.FamilyContext, fact) && !isDuplicateOrRedundantFact([]string{mem.FamilyContext}, fact) {
			mem.FamilyContext = mem.FamilyContext + "; " + fact
		}
	case "ambitions":
		if mem.Ambitions == "" {
			mem.Ambitions = fact
		} else if !strings.Contains(mem.Ambitions, fact) && !isDuplicateOrRedundantFact([]string{mem.Ambitions}, fact) {
			mem.Ambitions = mem.Ambitions + "; " + fact
		}
	case "medical":
		if mem.MedicalNotes == "" {
			mem.MedicalNotes = fact
		} else if !strings.Contains(mem.MedicalNotes, fact) && !isDuplicateOrRedundantFact([]string{mem.MedicalNotes}, fact) {
			mem.MedicalNotes = mem.MedicalNotes + "; " + fact
		}
	case "spiritual":
		if mem.SpiritualBeliefs == "" {
			mem.SpiritualBeliefs = fact
		} else if !strings.Contains(mem.SpiritualBeliefs, fact) && !isDuplicateOrRedundantFact([]string{mem.SpiritualBeliefs}, fact) {
			mem.SpiritualBeliefs = mem.SpiritualBeliefs + "; " + fact
		}
	case "strengths":
		mem.Strengths = mergeUniqueStrings(mem.Strengths, []string{fact})
	case "weaknesses":
		mem.Weaknesses = mergeUniqueStrings(mem.Weaknesses, []string{fact})
	}

	// Append to ExtractedFacts once, then deduplicate (fixed: was double-appending before)
	mem.ExtractedFacts = DeduplicateFacts(append(mem.ExtractedFacts, fact))
	mem.UpdatedAt = time.Now()

	// 3. Save updated intelligence to Firestore. This is the only place a memory fact
	// is persisted — it must never also be ingested into the RAG knowledge base (an
	// earlier version of this method did that as a step 4 here). The RAG store is
	// curriculum content an admin or teacher deliberately uploads to ground Buddy's
	// answers; it isn't a place for per-student facts extracted during a
	// conversation to leak into, where they'd show up mixed in among real course
	// materials on the knowledge base management screen for every admin to see.
	if err := s.repo.SavePersonalIntelligence(ctx, mem); err != nil {
		slog.Error("[Memory Persistence] Failed to save personal intelligence", slog.String("student_id", studentID), slog.String("error", err.Error()))
	} else {
		slog.Info(fmt.Sprintf("[Memory Persistence] Recorded memory: [%s] %s", category, fact), slog.String("student_id", studentID))
	}

	return nil
}

// isInvalidOrMetaFact checks if a fact is trivial, conversational, or not a genuine personal attribute.
// Returns true (drop it) for any statement that describes a transient state, dialogue exchange,
// mood update, greeting, or generic conversational observation instead of a durable personal fact.
func isInvalidOrMetaFact(fact string) bool {
	lower := strings.ToLower(strings.TrimSpace(fact))
	if len(lower) < 6 {
		return true
	}

	// Meta/conversational prefixes — statements about the student's dialogue, not their identity
	metaPrefixes := []string{
		"student asked", "student confirmed", "student inquired",
		"student requested", "student wondered", "student tested",
		"student greeted", "student said", "student noted",
		"student mentioned that they", "student indicated",
		"student reported", "student expressed", "student shared that",
		"student is engaging", "student was engaging",
		"student responded", "student replied", "student acknowledged",
		"user asked", "user confirmed", "user said",
		"asked if", "confirmed that", "inquired about",
		"student wants to know", "student checked",
	}
	for _, p := range metaPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}

	// Transient daily/mood state patterns — not durable personal facts
	trivialPatterns := []string{
		"is engaging in conversation",
		"is having a conversation",
		"had a good day",
		"having a good day",
		"reported having a good day",
		"reported a good day",
		"having a bad day",
		"had a bad day",
		"nothing significant happened",
		"indicated nothing significant",
		"nothing happened today",
		"is doing well",
		"is feeling well",
		"is feeling good",
		"is feeling okay",
		"is in a good mood",
		"is in a bad mood",
		"was in a good mood",
		"greeted the ai",
		"greeted buddy",
		"said hello",
		"said hi",
		"said ayubowan",
		"opened a conversation",
		"started a conversation",
		"is chatting",
		"engaged in small talk",
		"engaged in a conversation",
	}
	for _, p := range trivialPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// Too short or vague to be meaningful
	words := strings.Fields(lower)
	if len(words) < 3 {
		return true
	}

	return false
}

// normalizeFact cleans and normalizes a fact string for duplicate comparison.
func normalizeFact(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	lower = strings.ReplaceAll(lower, "'s", "")
	lower = strings.ReplaceAll(lower, "’s", "")
	var sb strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			sb.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

func stemFactWord(w string) string {
	w = strings.TrimSuffix(w, "ed")
	w = strings.TrimSuffix(w, "ing")
	w = strings.TrimSuffix(w, "es")
	w = strings.TrimSuffix(w, "s")
	w = strings.TrimSuffix(w, "d")
	return w
}

// isDuplicateOrRedundantFact checks if newFact is already represented in existing facts.
func isDuplicateOrRedundantFact(existing []string, newFact string) bool {
	normNew := normalizeFact(newFact)
	if normNew == "" {
		return true
	}
	newWords := strings.Fields(normNew)

	for _, ext := range existing {
		normExt := normalizeFact(ext)
		if normExt == "" {
			continue
		}
		if normExt == normNew {
			return true
		}
		// Containment check
		if strings.Contains(normExt, normNew) || strings.Contains(normNew, normExt) {
			return true
		}

		// Stemmed word overlap: if >= 65% of words in newFact are in existing fact
		extWords := strings.Fields(normExt)
		if len(newWords) > 0 && len(extWords) > 0 {
			matches := 0
			extWordMap := make(map[string]bool, len(extWords))
			for _, w := range extWords {
				extWordMap[stemFactWord(w)] = true
			}
			for _, w := range newWords {
				if extWordMap[stemFactWord(w)] {
					matches++
				}
			}
			overlap := float64(matches) / float64(len(newWords))
			if overlap >= 0.65 {
				return true
			}
		}
	}
	return false
}

// DeduplicateFacts filters out meta facts and deduplicates a list of facts.
func DeduplicateFacts(facts []string) []string {
	var result []string
	for _, f := range facts {
		clean := strings.TrimSpace(f)
		if clean == "" || isInvalidOrMetaFact(clean) {
			continue
		}
		if !isDuplicateOrRedundantFact(result, clean) {
			result = append(result, clean)
		}
	}
	return result
}

// GetActivePrompt retrieves the currently active system prompt of the given kind
// (chat or Live Talk — see domain.PromptKind).
func (s *Service) GetActivePrompt(ctx context.Context, kind domain.PromptKind) (*domain.SystemPrompt, error) {
	return s.repo.GetActivePrompt(ctx, kind)
}

// UpdateSystemPrompt creates and activates a new revision of the system prompt of the
// given kind (Admin only). Chat and Live Talk are edited completely independently —
// updating one never touches the other's active prompt or version history.
func (s *Service) UpdateSystemPrompt(ctx context.Context, adminID string, kind domain.PromptKind, req domain.UpdatePromptRequest) (*domain.SystemPrompt, error) {
	v := validator.New()
	v.Required("content", req.Content)
	if !kind.IsValid() {
		v.AddError("kind", "must be 'chat' or 'live_talk'")
	}
	if v.HasErrors() {
		return nil, v.Error()
	}

	current, _ := s.repo.GetActivePrompt(ctx, kind)
	newVersion := 1
	if current != nil {
		newVersion = current.Version + 1
	}

	name := "Buddy AI Chat Persona"
	if kind == domain.PromptKindLiveTalk {
		name = "Buddy AI Live Talk Persona"
	}

	prompt := &domain.SystemPrompt{
		ID:          fmt.Sprintf("prompt-%s-v%d-%d", kind, newVersion, time.Now().Unix()),
		Kind:        kind,
		Name:        fmt.Sprintf("%s v%d", name, newVersion),
		Content:     strings.TrimSpace(req.Content),
		Version:     newVersion,
		IsActive:    true,
		UpdatedBy:   adminID,
		UpdatedAt:   time.Now(),
		Description: req.Description,
	}

	if err := s.repo.SavePrompt(ctx, prompt); err != nil {
		return nil, fmt.Errorf("failed to save system prompt: %w", err)
	}

	return prompt, nil
}

// RenameSession updates the title of an existing chat session.
func (s *Service) RenameSession(ctx context.Context, studentID, sessionID string, isAdmin bool, newTitle string) (*domain.ChatSession, error) {
	v := validator.New()
	v.Required("title", newTitle)
	if v.HasErrors() {
		return nil, v.Error()
	}

	session, err := s.GetSession(ctx, studentID, sessionID, isAdmin)
	if err != nil {
		return nil, err
	}

	session.Title = strings.TrimSpace(newTitle)
	session.UpdatedAt = time.Now()

	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to rename session: %w", err)
	}

	return session, nil
}

// deriveSessionTitle intelligently generates a 3-6 word conversation title from user prompt or AI response.
// Fully preserves non-ASCII scripts (Sinhala, Tamil, etc.) without byte-slicing corruption.
func deriveSessionTitle(userPrompt, aiReply string) string {
	// 1. Try to extract clean topic heading from AI reply (e.g. ### Exam Stress Management)
	lines := strings.Split(aiReply, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			clean := strings.TrimLeft(trimmed, "#* `")
			clean = strings.TrimSpace(clean)
			// Strip leading numbers like "1. "
			runes := []rune(clean)
			if len(runes) > 3 && runes[1] == '.' && runes[2] == ' ' {
				clean = strings.TrimSpace(string(runes[3:]))
			}
			lower := strings.ToLower(clean)
			runesCount := len([]rune(clean))
			if runesCount >= 4 && runesCount <= 50 &&
				!strings.Contains(lower, "mindful reflection") &&
				!strings.Contains(lower, "next step") &&
				!strings.Contains(lower, "hello") &&
				!strings.Contains(lower, "action steps") {
				return clean
			}
		}
	}

	// 2. Fall back to user prompt
	cleanPrompt := strings.TrimSpace(userPrompt)
	lower := strings.ToLower(cleanPrompt)
	prefixes := []string{
		"can you help me with", "can you explain", "could you explain",
		"tell me about", "what is the", "what are the", "how do i",
		"how to", "i want to know", "help me understand", "give me guidance on",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			cleanPrompt = strings.TrimSpace(cleanPrompt[len(p):])
			break
		}
	}

	cleanPrompt = strings.Trim(cleanPrompt, "?!.,;:\"' ")
	words := strings.Fields(cleanPrompt)
	if len(words) == 0 {
		return "Conversation with Buddy"
	}

	if len(words) > 6 {
		words = words[:6]
	}

	// Unicode-safe word capitalization: capitalize only if first rune has upper case, don't slice bytes!
	var capitalized []string
	for _, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			first := unicode.ToUpper(runes[0])
			capitalized = append(capitalized, string(first)+string(runes[1:]))
		}
	}

	title := strings.Join(capitalized, " ")
	titleRunes := []rune(title)
	if len(titleRunes) > 42 {
		title = string(titleRunes[:40]) + "..."
	}
	return title
}
