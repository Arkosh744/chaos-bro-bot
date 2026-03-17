package features

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

// todayDate returns current date as YYYY-MM-DD string.
func todayDate() string {
	return time.Now().Format("2006-01-02")
}

// ShouldLieToday returns true if no lie has been generated for this user today.
func ShouldLieToday(store *storage.Storage, userID int64) bool {
	lie, _, _, err := store.GetTodayLie(userID, todayDate())
	if err != nil {
		return false
	}
	return lie == ""
}

// GenerateLie calls Claude with LieGeneratorPrompt and parses the response into lie and truth parts.
func GenerateLie(ctx context.Context, cl *claude.Client) (lie string, truth string, err error) {
	raw, err := cl.Ask(ctx, LieGeneratorPrompt, "Соври мне")
	if err != nil {
		return "", "", fmt.Errorf("generate lie: %w", err)
	}

	return parseLieResponse(raw)
}

// parseLieResponse extracts lie and truth from the formatted response.
func parseLieResponse(raw string) (string, string, error) {
	var lie, truth string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ЛОЖЬ|") {
			lie = strings.TrimSpace(strings.TrimPrefix(line, "ЛОЖЬ|"))
		}
		if strings.HasPrefix(line, "ПРАВДА|") {
			truth = strings.TrimSpace(strings.TrimPrefix(line, "ПРАВДА|"))
		}
	}

	if lie == "" || truth == "" {
		return "", "", fmt.Errorf("parse lie response: unexpected format: %s", raw)
	}

	return lie, truth, nil
}

// InjectLie appends the lie into the reply with a natural-sounding prefix.
func InjectLie(reply, lie string) string {
	return reply + "\n\nКстати, " + lowercaseFirst(lie)
}

// lowercaseFirst converts the first rune of a string to lowercase.
func lowercaseFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if runes[0] >= 'А' && runes[0] <= 'Я' {
		runes[0] = runes[0] + ('а' - 'А')
	} else if runes[0] >= 'A' && runes[0] <= 'Z' {
		runes[0] = runes[0] + ('a' - 'A')
	}
	return string(runes)
}
