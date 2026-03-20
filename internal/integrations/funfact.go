package integrations

import (
	"context"
	"fmt"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

const dayFactPrompt = `Какой сегодня день? %s. Расскажи один интересный/смешной факт про этот день в истории. 1-2 предложения, дерзко. На русском.`

// GetDayFact generates an interesting historical fact about the given date using Claude.
func GetDayFact(cl *claude.Client, date time.Time) (string, error) {
	if cl == nil {
		return "", fmt.Errorf("claude client is nil")
	}

	dateStr := date.Format("2 January 2006")
	prompt := fmt.Sprintf(dayFactPrompt, dateStr)

	fact, err := cl.Ask(context.Background(), prompt, "Факт дня")
	if err != nil {
		return "", fmt.Errorf("generate day fact: %w", err)
	}

	return fact, nil
}
