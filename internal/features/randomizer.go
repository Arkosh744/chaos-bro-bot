package features

import (
	"context"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

func Decide(ctx context.Context, cl *claude.Client, question string, userContext string) (string, error) {
	systemPrompt := RandomizerSystemPrompt + TimeOfDayMood()
	if userContext != "" {
		systemPrompt = systemPrompt + "\n\n" + userContext
	}
	return cl.Ask(ctx, systemPrompt, question)
}
