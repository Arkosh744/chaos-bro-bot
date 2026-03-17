package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

const summaryUpdateThreshold = 20

func BuildContext(summary string, recentMsgs []storage.Message) string {
	var sb strings.Builder

	if summary != "" {
		sb.WriteString("## What you know about this person\n")
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}

	if len(recentMsgs) > 0 {
		sb.WriteString("## Recent conversation\n")
		for _, m := range recentMsgs {
			if m.Role == "user" {
				sb.WriteString("User: ")
			} else {
				sb.WriteString("Bot: ")
			}
			sb.WriteString(m.Text)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func NeedsSummaryUpdate(store *storage.Storage, userID int64) (bool, error) {
	_, lastID, err := store.GetSummary(userID)
	if err != nil {
		return false, err
	}
	count, err := store.MessageCountSince(userID, lastID)
	if err != nil {
		return false, err
	}
	return count >= summaryUpdateThreshold, nil
}

func UpdateSummary(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) error {
	currentSummary, lastID, err := store.GetSummary(userID)
	if err != nil {
		return fmt.Errorf("get summary: %w", err)
	}

	msgs, err := store.GetMessagesSince(userID, lastID, 30)
	if err != nil {
		return fmt.Errorf("get messages since: %w", err)
	}
	if len(msgs) == 0 {
		return nil
	}

	var conversation strings.Builder
	for _, m := range msgs {
		conversation.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Text))
	}

	prompt := fmt.Sprintf("Текущее summary:\n%s\n\nПоследние сообщения:\n%s",
		currentSummary, conversation.String())

	newSummary, err := cl.Ask(ctx, SummaryUpdatePrompt, prompt)
	if err != nil {
		return fmt.Errorf("claude summary: %w", err)
	}

	lastMsgID := msgs[len(msgs)-1].ID
	return store.UpdateSummary(userID, newSummary, lastMsgID)
}
