package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"buddy/server/internal/domain"
)

// maxSessionDurationMins caps how much wall-clock time a single session can
// contribute to a student's estimated time-spent total. A session's duration is
// approximated as (last message timestamp − first message timestamp); without a cap,
// a conversation a student opened and only returned to hours or days later would count
// all of that idle time as "time spent", wildly overstating their real engagement.
const maxSessionDurationMins = 120

// Service computes student engagement/progress statistics from existing chat session
// data. There is no quiz or assessment data anywhere in this platform, so every metric
// here is an activity/engagement proxy — it describes how much a student has used
// Buddy AI, not how well they're learning or how much they've actually mastered.
type Service struct {
	chatRepo domain.ChatRepository
	userRepo domain.UserRepository
}

// NewService instantiates the analytics service.
func NewService(chatRepo domain.ChatRepository, userRepo domain.UserRepository) *Service {
	return &Service{chatRepo: chatRepo, userRepo: userRepo}
}

// GetStudentProgress computes one student's engagement stats.
func (s *Service) GetStudentProgress(ctx context.Context, studentID string) (*domain.StudentProgressStats, error) {
	if studentID == "" {
		return nil, domain.ErrInvalidInput
	}
	sessions, err := s.chatRepo.ListAllSessionsForStudent(ctx, studentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	stats := computeStats(studentID, sessions)
	return &stats, nil
}

// GetLeaderboard computes and ranks every student's engagement stats, most engaged
// first. This aggregates every student's full session history in one call, so it's
// meant as an admin/teacher reporting view rather than something called on every page
// load — one student's session-read failure is skipped rather than failing the whole
// leaderboard, since a partial ranking is far more useful here than none at all.
func (s *Service) GetLeaderboard(ctx context.Context) ([]*domain.LeaderboardEntry, error) {
	students, _, err := s.userRepo.ListStudents(ctx, domain.ListUsersFilter{Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("failed to list students: %w", err)
	}

	entries := make([]*domain.LeaderboardEntry, 0, len(students))
	for _, student := range students {
		sessions, sessErr := s.chatRepo.ListAllSessionsForStudent(ctx, student.User.ID)
		if sessErr != nil {
			continue
		}
		stats := computeStats(student.User.ID, sessions)
		entries = append(entries, &domain.LeaderboardEntry{
			StudentName:          student.User.DisplayName,
			BatchName:            student.Profile.BatchName,
			StudentProgressStats: stats,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].EngagementScore > entries[j].EngagementScore
	})
	for i, e := range entries {
		e.Rank = i + 1
	}

	return entries, nil
}

func computeStats(studentID string, sessions []*domain.ChatSession) domain.StudentProgressStats {
	stats := domain.StudentProgressStats{StudentID: studentID}
	if len(sessions) == 0 {
		return stats
	}

	stats.TotalSessions = len(sessions)
	activeDates := make(map[string]struct{})
	totalMinutes := 0

	for _, sess := range sessions {
		stats.TotalMessages += sess.MessageCount
		if strings.HasPrefix(sess.Title, domain.LiveTalkSessionTitlePrefix) {
			stats.LiveTalkCount++
		}

		if stats.FirstActiveAt == nil || sess.CreatedAt.Before(*stats.FirstActiveAt) {
			t := sess.CreatedAt
			stats.FirstActiveAt = &t
		}
		if stats.LastActiveAt == nil || sess.UpdatedAt.After(*stats.LastActiveAt) {
			t := sess.UpdatedAt
			stats.LastActiveAt = &t
		}

		activeDates[sess.CreatedAt.Format("2006-01-02")] = struct{}{}
		if !sess.UpdatedAt.IsZero() {
			activeDates[sess.UpdatedAt.Format("2006-01-02")] = struct{}{}
		}

		durationMins := int(sess.UpdatedAt.Sub(sess.CreatedAt).Minutes())
		if durationMins < 0 {
			durationMins = 0
		}
		if durationMins > maxSessionDurationMins {
			durationMins = maxSessionDurationMins
		}
		totalMinutes += durationMins
	}

	stats.ActiveDays = len(activeDates)
	stats.TimeSpentMins = totalMinutes
	stats.AvgMsgsPerSess = float64(stats.TotalMessages) / float64(stats.TotalSessions)
	stats.EngagementScore = computeEngagementScore(stats)

	return stats
}

// computeEngagementScore is a transparent weighted sum used purely to rank students
// relative to each other — it is not a percentage, grade, or absolute measure of
// anything, just one sortable number. Weights favor consistency (active days) and
// genuine two-way conversation (messages) over raw session count, which a student
// could otherwise inflate just by starting many short-lived chats.
func computeEngagementScore(stats domain.StudentProgressStats) float64 {
	return float64(stats.TotalMessages)*1.0 +
		float64(stats.ActiveDays)*8.0 +
		float64(stats.TimeSpentMins)*0.5 +
		float64(stats.TotalSessions)*2.0
}
