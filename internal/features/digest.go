package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

const DigestPrompt = models.DigestPrompt

// GenerateDigest creates a weekly trickster-style digest based on user summary and recent messages.
func GenerateDigest(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) (string, error) {
	summary, _, err := store.GetSummary(userID)
	if err != nil {
		return "", fmt.Errorf("get summary: %w", err)
	}

	msgs, err := store.GetLastMessages(userID, 50)
	if err != nil {
		return "", fmt.Errorf("get messages: %w", err)
	}

	userMsgCount := 0
	var recentTexts strings.Builder
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgCount++
		}
		fmt.Fprintf(&recentTexts, "%s: %s\n", m.Role, m.Text)
	}

	// Collect weekly highlights
	moods, _ := store.GetMoodHistory(userID, 7)
	var bestMood, worstMood int
	var bestDay, worstDay string
	if len(moods) > 0 {
		bestMood = moods[0].Score
		worstMood = moods[0].Score
		bestDay = moods[0].CreatedAt.Format("Mon")
		worstDay = moods[0].CreatedAt.Format("Mon")
		for _, m := range moods[1:] {
			if m.Score > bestMood {
				bestMood = m.Score
				bestDay = m.CreatedAt.Format("Mon")
			}
			if m.Score < worstMood {
				worstMood = m.Score
				worstDay = m.CreatedAt.Format("Mon")
			}
		}
	}

	achievements, _ := store.GetAchievements(userID)
	weekAgo := time.Now().AddDate(0, 0, -7)
	weekAchCount := 0
	for range achievements {
		// Count all achievements as approximate (no timestamp filtering available)
		weekAchCount++
	}

	streakDays, _ := store.GetCounter(userID, "streak_days")

	// Find most active day
	dayMsgs, _ := store.GetMessagesByDay(userID, 7)
	var mostActiveDay string
	var mostActiveDayCount int
	for _, d := range dayMsgs {
		if d.Count > mostActiveDayCount {
			mostActiveDayCount = d.Count
			mostActiveDay = d.Date
		}
	}
	if mostActiveDay != "" {
		if t, parseErr := time.Parse("2006-01-02", mostActiveDay); parseErr == nil {
			mostActiveDay = t.Format("Mon 02.01")
		}
	}

	// Total messages this week
	weeklyMsgCount, _ := store.GetMessageCountSinceDate(userID, weekAgo)

	var highlights strings.Builder
	highlights.WriteString(fmt.Sprintf("Всего сообщений за неделю: %d\n", weeklyMsgCount))
	if bestMood > 0 {
		highlights.WriteString(fmt.Sprintf("Лучшее настроение: %d/10 (%s)\n", bestMood, bestDay))
		highlights.WriteString(fmt.Sprintf("Худшее настроение: %d/10 (%s)\n", worstMood, worstDay))
	}
	highlights.WriteString(fmt.Sprintf("Всего ачивок: %d\n", weekAchCount))
	highlights.WriteString(fmt.Sprintf("Streak: %d дней\n", streakDays))
	if mostActiveDay != "" {
		highlights.WriteString(fmt.Sprintf("Самый активный день: %s (%d сообщ.)\n", mostActiveDay, mostActiveDayCount))
	}

	prompt := fmt.Sprintf("Summary пользователя:\n%s\n\n%s\nПоследние сообщения:\n%s",
		summary, highlights.String(), recentTexts.String())

	return cl.Ask(ctx, DigestPrompt, prompt)
}
