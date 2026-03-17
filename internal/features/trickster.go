package features

import (
	"context"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

func TricksterReply(ctx context.Context, cl *claude.Client, message string, userContext string) (string, error) {
	systemPrompt := TricksterSystemPrompt + TimeOfDayMood() + DayOfWeekMood() + AlterEgoPromptSuffix()
	if userContext != "" {
		systemPrompt = systemPrompt + "\n\n" + userContext
	}
	return cl.Ask(ctx, systemPrompt, message)
}