package analytics_test

import (
	"context"
	"testing"
	"time"

	"buddy/server/internal/analytics"
	chatModule "buddy/server/internal/chat"
	"buddy/server/internal/domain"
	"buddy/server/internal/user"
)

func seedSession(t *testing.T, repo domain.ChatRepository, id, studentID, title string, messageCount int, createdAt, updatedAt time.Time) {
	t.Helper()
	if err := repo.CreateSession(context.Background(), &domain.ChatSession{
		ID: id, StudentID: studentID, Title: title, MessageCount: messageCount,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("failed to seed session %s: %v", id, err)
	}
}

func TestAnalyticsService_GetStudentProgress(t *testing.T) {
	chatRepo := chatModule.NewMemoryChatRepository()
	userRepo := user.NewMemoryRepository()
	svc := analytics.NewService(chatRepo, userRepo)
	ctx := context.Background()

	day1 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

	// Session 1: typed chat, 10 messages, 20 minutes, on day1.
	seedSession(t, chatRepo, "s1", "stu-1", "Study Advice", 10, day1, day1.Add(20*time.Minute))
	// Session 2: a Live Talk call, 4 messages, 15 minutes, on day2.
	seedSession(t, chatRepo, "s2", "stu-1", domain.LiveTalkSessionTitlePrefix+" - Jan 02", 4, day2, day2.Add(15*time.Minute))
	// A session belonging to a different student must never be counted.
	seedSession(t, chatRepo, "s3", "stu-2", "Someone Else's Chat", 100, day1, day1.Add(time.Hour))

	stats, err := svc.GetStudentProgress(ctx, "stu-1")
	if err != nil {
		t.Fatalf("GetStudentProgress failed: %v", err)
	}

	if stats.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", stats.TotalSessions)
	}
	if stats.TotalMessages != 14 {
		t.Errorf("expected 14 messages, got %d", stats.TotalMessages)
	}
	if stats.LiveTalkCount != 1 {
		t.Errorf("expected 1 live talk session, got %d", stats.LiveTalkCount)
	}
	if stats.ActiveDays != 2 {
		t.Errorf("expected 2 active days, got %d", stats.ActiveDays)
	}
	if stats.TimeSpentMins != 35 {
		t.Errorf("expected 35 minutes time spent, got %d", stats.TimeSpentMins)
	}
	if stats.AvgMsgsPerSess != 7 {
		t.Errorf("expected average of 7 messages per session, got %v", stats.AvgMsgsPerSess)
	}
	if stats.EngagementScore <= 0 {
		t.Errorf("expected a positive engagement score, got %v", stats.EngagementScore)
	}
}

func TestAnalyticsService_SessionDurationIsCapped(t *testing.T) {
	// A session opened and left idle for days shouldn't count that idle time as
	// "time spent" — verifies the per-session cap in computeStats.
	chatRepo := chatModule.NewMemoryChatRepository()
	userRepo := user.NewMemoryRepository()
	svc := analytics.NewService(chatRepo, userRepo)
	ctx := context.Background()

	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	seedSession(t, chatRepo, "s1", "stu-idle", "Left Open", 2, start, start.Add(72*time.Hour))

	stats, err := svc.GetStudentProgress(ctx, "stu-idle")
	if err != nil {
		t.Fatalf("GetStudentProgress failed: %v", err)
	}
	if stats.TimeSpentMins != 120 {
		t.Errorf("expected time spent capped at 120 minutes, got %d", stats.TimeSpentMins)
	}
}

func TestAnalyticsService_GetLeaderboard_RanksByEngagement(t *testing.T) {
	chatRepo := chatModule.NewMemoryChatRepository()
	userRepo := user.NewMemoryRepository()
	svc := analytics.NewService(chatRepo, userRepo)
	ctx := context.Background()

	now := time.Now()

	// A highly engaged student: many sessions, messages, and active days.
	if err := userRepo.CreateUser(ctx, &domain.User{ID: "stu-active", Email: "active@example.com", DisplayName: "Active Student", Role: domain.RoleStudent}); err != nil {
		t.Fatalf("failed to seed active user: %v", err)
	}
	for i := 0; i < 5; i++ {
		day := now.AddDate(0, 0, -i)
		seedSession(t, chatRepo, "active-s"+string(rune('0'+i)), "stu-active", "Chat", 20, day, day.Add(30*time.Minute))
	}

	// A barely engaged student: one short session.
	if err := userRepo.CreateUser(ctx, &domain.User{ID: "stu-quiet", Email: "quiet@example.com", DisplayName: "Quiet Student", Role: domain.RoleStudent}); err != nil {
		t.Fatalf("failed to seed quiet user: %v", err)
	}
	seedSession(t, chatRepo, "quiet-s1", "stu-quiet", "Chat", 2, now, now.Add(2*time.Minute))

	entries, err := svc.GetLeaderboard(ctx)
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 leaderboard entries, got %d", len(entries))
	}

	if entries[0].StudentName != "Active Student" || entries[0].Rank != 1 {
		t.Errorf("expected Active Student ranked #1, got %s at rank %d", entries[0].StudentName, entries[0].Rank)
	}
	if entries[1].StudentName != "Quiet Student" || entries[1].Rank != 2 {
		t.Errorf("expected Quiet Student ranked #2, got %s at rank %d", entries[1].StudentName, entries[1].Rank)
	}
	if entries[0].EngagementScore <= entries[1].EngagementScore {
		t.Errorf("expected the more engaged student to have a higher score: %v vs %v", entries[0].EngagementScore, entries[1].EngagementScore)
	}
}
