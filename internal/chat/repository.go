package chat

import (
	"context"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
)

const (
	CollectionSystemPrompts   = "system_prompts"
	CollectionStudentMemories = "student_memories"
	CollectionChatSessions    = "chat_sessions"
	CollectionChatMessages    = "chat_messages"
)

// FirestoreChatRepository implements domain.ChatRepository backed by Cloud Firestore.
type FirestoreChatRepository struct {
	client *firestore.Client
}

// NewFirestoreChatRepository initializes a Firestore chat repository.
func NewFirestoreChatRepository(client *firestore.Client) domain.ChatRepository {
	return &FirestoreChatRepository{client: client}
}

func (r *FirestoreChatRepository) CreateSession(ctx context.Context, session *ChatSessionDocument) error {
	_, err := r.client.Collection(CollectionChatSessions).Doc(session.ID).Set(ctx, session)
	return err
}

func (r *FirestoreChatRepository) GetSession(ctx context.Context, sessionID string) (*domain.ChatSession, error) {
	doc, err := r.client.Collection(CollectionChatSessions).Doc(sessionID).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var s domain.ChatSession
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *FirestoreChatRepository) ListSessions(ctx context.Context, studentID string, limit int) ([]*domain.ChatSession, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	iter := r.client.Collection(CollectionChatSessions).
		Where("student_id", "==", studentID).
		Limit(limit).
		Documents(ctx)

	var sessions []*domain.ChatSession
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var s domain.ChatSession
		if err := doc.DataTo(&s); err == nil {
			sessions = append(sessions, &s)
		}
	}
	return sessions, nil
}

func (r *FirestoreChatRepository) ListAllSessionsForStudent(ctx context.Context, studentID string) ([]*domain.ChatSession, error) {
	iter := r.client.Collection(CollectionChatSessions).
		Where("student_id", "==", studentID).
		Documents(ctx)

	var sessions []*domain.ChatSession
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var s domain.ChatSession
		if err := doc.DataTo(&s); err == nil {
			sessions = append(sessions, &s)
		}
	}
	return sessions, nil
}

func (r *FirestoreChatRepository) UpdateSession(ctx context.Context, session *domain.ChatSession) error {
	session.UpdatedAt = time.Now()
	_, err := r.client.Collection(CollectionChatSessions).Doc(session.ID).Set(ctx, session)
	return err
}

func (r *FirestoreChatRepository) DeleteSession(ctx context.Context, sessionID string) error {
	// Cascade delete all messages associated with this session
	r.deleteMessagesBySession(ctx, sessionID)

	_, err := r.client.Collection(CollectionChatSessions).Doc(sessionID).Delete(ctx)
	return err
}

// deleteMessagesBySession removes every message belonging to a session. Firestore has
// no cascading delete of its own, so this always has to be done explicitly before (or
// alongside) removing the session document itself.
func (r *FirestoreChatRepository) deleteMessagesBySession(ctx context.Context, sessionID string) {
	msgIter := r.client.Collection(CollectionChatMessages).Where("session_id", "==", sessionID).Documents(ctx)
	for {
		doc, err := msgIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
		_, _ = doc.Ref.Delete(ctx)
	}
}

// DeleteSessionsByStudent cascades to every session (and each session's messages)
// belonging to studentID. Used when a user account is deleted so no chat history is
// left behind. Unlike ListSessions, this is not bounded by the 50-item API page-size
// cap — a user with more sessions than that must still be fully cleaned up.
func (r *FirestoreChatRepository) DeleteSessionsByStudent(ctx context.Context, studentID string) error {
	sessIter := r.client.Collection(CollectionChatSessions).Where("student_id", "==", studentID).Documents(ctx)
	for {
		doc, err := sessIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		r.deleteMessagesBySession(ctx, doc.Ref.ID)
		_, _ = doc.Ref.Delete(ctx)
	}
	return nil
}

func (r *FirestoreChatRepository) SaveMessage(ctx context.Context, message *domain.ChatMessage) error {
	_, err := r.client.Collection(CollectionChatMessages).Doc(message.ID).Set(ctx, message)
	return err
}

func (r *FirestoreChatRepository) GetMessage(ctx context.Context, messageID string) (*domain.ChatMessage, error) {
	doc, err := r.client.Collection(CollectionChatMessages).Doc(messageID).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var m domain.ChatMessage
	if err := doc.DataTo(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *FirestoreChatRepository) DeleteMessage(ctx context.Context, messageID string) error {
	_, err := r.client.Collection(CollectionChatMessages).Doc(messageID).Delete(ctx)
	return err
}

func (r *FirestoreChatRepository) ListMessages(ctx context.Context, sessionID string, limit int) ([]*domain.ChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	iter := r.client.Collection(CollectionChatMessages).
		Where("session_id", "==", sessionID).
		Limit(limit).
		Documents(ctx)

	var messages []*domain.ChatMessage
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var m domain.ChatMessage
		if err := doc.DataTo(&m); err == nil {
			messages = append(messages, &m)
		}
	}

	// Sort chronologically
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	return messages, nil
}

func (r *FirestoreChatRepository) GetPersonalIntelligence(ctx context.Context, studentID string) (*domain.StudentPersonalIntelligence, error) {
	doc, err := r.client.Collection(CollectionStudentMemories).Doc(studentID).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	var m domain.StudentPersonalIntelligence
	if err := doc.DataTo(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *FirestoreChatRepository) SavePersonalIntelligence(ctx context.Context, memory *domain.StudentPersonalIntelligence) error {
	memory.UpdatedAt = time.Now()
	_, err := r.client.Collection(CollectionStudentMemories).Doc(memory.StudentID).Set(ctx, memory)
	return err
}

func (r *FirestoreChatRepository) DeletePersonalIntelligence(ctx context.Context, studentID string) error {
	_, err := r.client.Collection(CollectionStudentMemories).Doc(studentID).Delete(ctx)
	return err
}

// defaultPromptFor returns the seeded default document for a prompt kind, used until
// an admin saves their own.
func defaultPromptFor(kind domain.PromptKind) *domain.SystemPrompt {
	if kind == domain.PromptKindLiveTalk {
		return &domain.SystemPrompt{
			ID:        "default-buddy-live-talk-prompt",
			Kind:      domain.PromptKindLiveTalk,
			Name:      "Buddy AI Live Talk Default Prompt (Jinasena Training Foundation)",
			Content:   domain.DefaultLiveTalkSystemPrompt,
			Version:   1,
			IsActive:  true,
			UpdatedAt: time.Now(),
		}
	}
	return &domain.SystemPrompt{
		ID:        "default-buddy-prompt",
		Kind:      domain.PromptKindChat,
		Name:      "Buddy AI Default Prompt (Jinasena Training Foundation)",
		Content:   domain.DefaultBuddySystemPrompt,
		Version:   1,
		IsActive:  true,
		UpdatedAt: time.Now(),
	}
}

// GetActivePrompt returns the active prompt for kind. It queries every is_active
// document rather than filtering by kind in Firestore, because every prompt document
// that existed before PromptKind was introduced has no "kind" field at all — those are
// implicitly PromptKindChat documents, and a Firestore equality filter on a missing
// field would silently fail to match them, which would make an admin's existing
// carefully-configured chat prompt vanish back to the hardcoded default the moment
// this shipped. If more than one matching active document turns up (e.g. left over
// from before an earlier version of this method failed to deactivate a prompt's
// predecessor when a new one was saved), the most recent one wins and the others are
// deactivated in the background — self-healing rather than returning an arbitrary one.
func (r *FirestoreChatRepository) GetActivePrompt(ctx context.Context, kind domain.PromptKind) (*domain.SystemPrompt, error) {
	iter := r.client.Collection(CollectionSystemPrompts).Where("is_active", "==", true).Documents(ctx)
	var candidates []*domain.SystemPrompt
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var p domain.SystemPrompt
		if err := doc.DataTo(&p); err != nil {
			continue
		}
		effectiveKind := p.Kind
		if effectiveKind == "" {
			effectiveKind = domain.PromptKindChat // pre-PromptKind documents
		}
		if effectiveKind == kind {
			candidates = append(candidates, &p)
		}
	}

	if len(candidates) == 0 {
		return defaultPromptFor(kind), nil
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt) })
	winner := candidates[0]
	if winner.Kind == "" {
		winner.Kind = kind
	}

	if len(candidates) > 1 {
		go func(losers []*domain.SystemPrompt) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for _, loser := range losers {
				_, _ = r.client.Collection(CollectionSystemPrompts).Doc(loser.ID).Update(bgCtx, []firestore.Update{
					{Path: "is_active", Value: false},
				})
			}
		}(candidates[1:])
	}

	return winner, nil
}

func (r *FirestoreChatRepository) SavePrompt(ctx context.Context, prompt *domain.SystemPrompt) error {
	prompt.UpdatedAt = time.Now()
	if _, err := r.client.Collection(CollectionSystemPrompts).Doc(prompt.ID).Set(ctx, prompt); err != nil {
		return err
	}

	if !prompt.IsActive {
		return nil
	}

	// Deactivate every other active document of this kind (including legacy documents
	// with no kind field, for PromptKindChat) so GetActivePrompt never has more than
	// one genuine candidate to pick between going forward.
	iter := r.client.Collection(CollectionSystemPrompts).Where("is_active", "==", true).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
		if doc.Ref.ID == prompt.ID {
			continue
		}
		var p domain.SystemPrompt
		if err := doc.DataTo(&p); err != nil {
			continue
		}
		effectiveKind := p.Kind
		if effectiveKind == "" {
			effectiveKind = domain.PromptKindChat
		}
		if effectiveKind == prompt.Kind {
			_, _ = doc.Ref.Update(ctx, []firestore.Update{{Path: "is_active", Value: false}})
		}
	}
	return nil
}

// Helper document type
type ChatSessionDocument = domain.ChatSession

// MemoryChatRepository provides thread-safe in-memory storage for unit testing and offline development.
type MemoryChatRepository struct {
	mu            sync.RWMutex
	sessions      map[string]*domain.ChatSession
	messages      map[string][]*domain.ChatMessage
	memories      map[string]*domain.StudentPersonalIntelligence
	activePrompts map[domain.PromptKind]*domain.SystemPrompt
}

// NewMemoryChatRepository initializes an in-memory repository seeded with the default
// chat and Live Talk system prompts.
func NewMemoryChatRepository() domain.ChatRepository {
	return &MemoryChatRepository{
		sessions: make(map[string]*domain.ChatSession),
		messages: make(map[string][]*domain.ChatMessage),
		memories: make(map[string]*domain.StudentPersonalIntelligence),
		activePrompts: map[domain.PromptKind]*domain.SystemPrompt{
			domain.PromptKindChat:     defaultPromptFor(domain.PromptKindChat),
			domain.PromptKindLiveTalk: defaultPromptFor(domain.PromptKindLiveTalk),
		},
	}
}

func (m *MemoryChatRepository) CreateSession(ctx context.Context, session *domain.ChatSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryChatRepository) GetSession(ctx context.Context, sessionID string) (*domain.ChatSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *s
	return &copy, nil
}

func (m *MemoryChatRepository) ListSessions(ctx context.Context, studentID string, limit int) ([]*domain.ChatSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []*domain.ChatSession
	for _, s := range m.sessions {
		if s.StudentID == studentID {
			results = append(results, s)
		}
	}
	return results, nil
}

func (m *MemoryChatRepository) ListAllSessionsForStudent(ctx context.Context, studentID string) ([]*domain.ChatSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []*domain.ChatSession
	for _, s := range m.sessions {
		if s.StudentID == studentID {
			results = append(results, s)
		}
	}
	return results, nil
}

func (m *MemoryChatRepository) UpdateSession(ctx context.Context, session *domain.ChatSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

func (m *MemoryChatRepository) DeleteSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	delete(m.messages, sessionID)
	return nil
}

func (m *MemoryChatRepository) DeleteSessionsByStudent(ctx context.Context, studentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.StudentID == studentID {
			delete(m.sessions, id)
			delete(m.messages, id)
		}
	}
	return nil
}

func (m *MemoryChatRepository) SaveMessage(ctx context.Context, message *domain.ChatMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.messages[message.SessionID]
	for i, existing := range msgs {
		if existing.ID == message.ID {
			msgs[i] = message // upsert in place, matching FirestoreChatRepository's Set semantics
			return nil
		}
	}
	m.messages[message.SessionID] = append(msgs, message)
	return nil
}

func (m *MemoryChatRepository) GetMessage(ctx context.Context, messageID string) (*domain.ChatMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, msgs := range m.messages {
		for _, msg := range msgs {
			if msg.ID == messageID {
				copy := *msg
				return &copy, nil
			}
		}
	}
	return nil, domain.ErrNotFound
}

func (m *MemoryChatRepository) DeleteMessage(ctx context.Context, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for sID, msgs := range m.messages {
		var filtered []*domain.ChatMessage
		for _, msg := range msgs {
			if msg.ID != messageID {
				filtered = append(filtered, msg)
			}
		}
		m.messages[sID] = filtered
	}
	return nil
}

func (m *MemoryChatRepository) ListMessages(ctx context.Context, sessionID string, limit int) ([]*domain.ChatMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.messages[sessionID]
	return msgs, nil
}

func (m *MemoryChatRepository) GetPersonalIntelligence(ctx context.Context, studentID string) (*domain.StudentPersonalIntelligence, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mem, ok := m.memories[studentID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *mem
	return &copy, nil
}

func (m *MemoryChatRepository) SavePersonalIntelligence(ctx context.Context, memory *domain.StudentPersonalIntelligence) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	memory.UpdatedAt = time.Now()
	m.memories[memory.StudentID] = memory
	return nil
}

func (m *MemoryChatRepository) DeletePersonalIntelligence(ctx context.Context, studentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.memories, studentID)
	return nil
}

func (m *MemoryChatRepository) GetActivePrompt(ctx context.Context, kind domain.PromptKind) (*domain.SystemPrompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.activePrompts[kind]; ok {
		return p, nil
	}
	return defaultPromptFor(kind), nil
}

func (m *MemoryChatRepository) SavePrompt(ctx context.Context, prompt *domain.SystemPrompt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if prompt.IsActive {
		m.activePrompts[prompt.Kind] = prompt
	}
	return nil
}
