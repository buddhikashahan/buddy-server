package domain

import "time"

// StudentProgressStats summarizes one student's engagement with Buddy AI. This is an
// activity/engagement proxy, not an academic-mastery measure — the platform has no
// quiz or assessment data to draw on, only chat session activity, so that's honestly
// what these numbers can and can't say about a student's actual learning progress.
type StudentProgressStats struct {
	StudentID string `json:"student_id"`

	TotalSessions  int `json:"total_sessions"`
	TotalMessages  int `json:"total_messages"`
	LiveTalkCount  int `json:"live_talk_count"`  // sessions that were spoken Live Talk calls
	ActiveDays     int `json:"active_days"`      // distinct calendar days with at least one session
	TimeSpentMins  int `json:"time_spent_mins"`  // see EngagementScore doc for how this is estimated
	AvgMsgsPerSess float64 `json:"avg_messages_per_session"`

	FirstActiveAt *time.Time `json:"first_active_at,omitempty"`
	LastActiveAt  *time.Time `json:"last_active_at,omitempty"`

	// EngagementScore is a weighted sum of the metrics above, used only to rank
	// students relative to each other (see analytics.computeEngagementScore) — it is
	// not a percentage or a grade, and has no meaning as an absolute number.
	EngagementScore float64 `json:"engagement_score"`
}

// LeaderboardEntry is one ranked row: a student's progress stats plus the display
// fields (name, batch) a leaderboard table needs, which StudentProgressStats alone
// doesn't carry.
type LeaderboardEntry struct {
	Rank        int    `json:"rank"`
	StudentName string `json:"student_name"`
	BatchName   string `json:"batch_name,omitempty"`
	StudentProgressStats
}
