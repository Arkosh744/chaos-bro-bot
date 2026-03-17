package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

func GenerateQuote(ctx context.Context, cl *claude.Client, recentQuotes []string) (string, error) {
	recent := strings.Join(recentQuotes, "\n")
	prompt := fmt.Sprintf(QuotesSystemPrompt, recent)
	return cl.Ask(ctx, prompt, "Дай цитату")
}
