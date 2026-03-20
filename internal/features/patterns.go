package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

const PatternDetectPrompt = models.PatternDetectPrompt

// DetectPatterns analyzes user summary and recent messages to find recurring behavioral patterns.
// Returns a trickster-style quip if a pattern is found, empty string otherwise.
func DetectPatterns(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) (string, error) {
	summary, _, err := store.GetSummary(userID)
	if err != nil || summary == "" {
		return "", err
	}

	msgs, err := store.GetLastMessages(userID, 10)
	if err != nil {
		return "", fmt.Errorf("get last messages: %w", err)
	}

	var recent strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&recent, "%s: %s\n", m.Role, m.Text)
	}

	prompt := fmt.Sprintf("Summary:\n%s\n\nПоследние сообщения:\n%s", summary, recent.String())

	result, err := cl.Ask(ctx, PatternDetectPrompt, prompt)
	if err != nil {
		return "", fmt.Errorf("claude pattern detect: %w", err)
	}

	normalized := strings.ToLower(strings.TrimSpace(result))
	if normalized == "нет" {
		return "", nil
	}

	return result, nil
}
