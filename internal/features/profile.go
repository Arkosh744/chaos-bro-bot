package features

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

// CategoryLabels maps profile fact categories to their display labels.
var CategoryLabels = models.CategoryLabels

func ExtractFacts(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) error {
	summary, _, err := store.GetSummary(userID)
	if err != nil {
		return fmt.Errorf("get summary: %w", err)
	}

	msgs, err := store.GetLastMessages(userID, 10)
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}

	if summary == "" && len(msgs) == 0 {
		return nil
	}

	var input strings.Builder
	if summary != "" {
		input.WriteString("Summary:\n" + summary + "\n\n")
	}

	currentProfile, _ := store.GetFactsAsText(userID)
	if currentProfile != "" {
		input.WriteString("Текущий профиль:\n" + currentProfile + "\n\n")
	}

	input.WriteString("Последние сообщения:\n")
	for _, m := range msgs {
		input.WriteString(m.Role + ": " + m.Text + "\n")
	}

	resp, err := cl.Ask(ctx, ProfileExtractPrompt, input.String())
	if err != nil {
		return fmt.Errorf("claude extract: %w", err)
	}

	resp = strings.TrimSpace(resp)
	if resp == "ПУСТО" || resp == "" {
		return nil
	}

	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		category := strings.TrimSpace(parts[0])
		fact := strings.TrimSpace(parts[1])

		if _, ok := CategoryLabels[category]; !ok {
			continue
		}
		if fact == "" {
			continue
		}

		if err := store.SaveFact(userID, category, fact); err != nil {
			log.Printf("[%d] save fact error (%s): %v", userID, category, err)
		}
	}

	return nil
}

func FormatProfile(facts []storage.UserFact) string {
	if len(facts) == 0 {
		return models.MsgProfileEmpty
	}

	var sb strings.Builder
	sb.WriteString(models.MsgProfileHeader)

	for _, f := range facts {
		label, ok := CategoryLabels[f.Category]
		if !ok {
			label = f.Category
		}
		sb.WriteString(label + ": " + f.Fact + "\n")
	}

	sb.WriteString(models.MsgProfileFooter)
	return sb.String()
}
