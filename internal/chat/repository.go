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

func (r *FirestoreChatRepository) GetActivePrompt(ctx context.Context) (*domain.SystemPrompt, error) {
	iter := r.client.Collection(CollectionSystemPrompts).
		Where("is_active", "==", true).
		Limit(1).
		Documents(ctx)

	doc, err := iter.Next()
	if err == iterator.Done || err != nil {
		// Return default seeded prompt
		return &domain.SystemPrompt{
			ID:        "default-buddy-prompt",
			Name:      "Buddy AI Default Prompt (Jinasena Training Foundation)",
			Content:   domain.DefaultBuddySystemPrompt,
			Version:   1,
			IsActive:  true,
			UpdatedAt: time.Now(),
		}, nil
	}

	var p domain.SystemPrompt
	if err := doc.DataTo(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *FirestoreChatRepository) SavePrompt(ctx context.Context, prompt *domain.SystemPrompt) error {
	prompt.UpdatedAt = time.Now()
	_, err := r.client.Collection(CollectionSystemPrompts).Doc(prompt.ID).Set(ctx, prompt)
	return err
}

func (r *FirestoreChatRepository) ListPrompts(ctx context.Context) ([]*domain.SystemPrompt, error) {
	iter := r.client.Collection(CollectionSystemPrompts).Documents(ctx)
	var prompts []*domain.SystemPrompt
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var p domain.SystemPrompt
		if err := doc.DataTo(&p); err == nil {
			prompts = append(prompts, &p)
		}
	}
	return prompts, nil
}

// Helper document type
type ChatSessionDocument = domain.ChatSession

// MemoryChatRepository provides thread-safe in-memory storage for unit testing and offline development.
type MemoryChatRepository struct {
	mu           sync.RWMutex
	sessions     map[string]*domain.ChatSession
	messages     map[string][]*domain.ChatMessage
	memories     map[string]*domain.StudentPersonalIntelligence
	activePrompt *domain.SystemPrompt
}

// NewMemoryChatRepository initializes an in-memory repository seeded with default Buddy system prompt.
func NewMemoryChatRepository() domain.ChatRepository {
	return &MemoryChatRepository{
		sessions: make(map[string]*domain.ChatSession),
		messages: make(map[string][]*domain.ChatMessage),
		memories: make(map[string]*domain.StudentPersonalIntelligence),
		activePrompt: &domain.SystemPrompt{
			ID:        "default-buddy-prompt",
			Name:      "Buddy AI Default Prompt (Jinasena Training Foundation)",
			Content:   domain.DefaultBuddySystemPrompt,
			Version:   1,
			IsActive:  true,
			UpdatedAt: time.Now(),
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
	m.messages[message.SessionID] = append(m.messages[message.SessionID], message)
	return nil
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

func (m *MemoryChatRepository) GetActivePrompt(ctx context.Context) (*domain.SystemPrompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activePrompt, nil
}

func (m *MemoryChatRepository) SavePrompt(ctx context.Context, prompt *domain.SystemPrompt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activePrompt = prompt
	return nil
}

func (m *MemoryChatRepository) ListPrompts(ctx context.Context) ([]*domain.SystemPrompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return []*domain.SystemPrompt{m.activePrompt}, nil
}
