package features

import (
	"context"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

func TricksterReply(ctx context.Context, cl *claude.Client, message string, userContext string) (string, error) {
	systemPrompt := TricksterSystemPrompt + TimeOfDayMood() + AlterEgoPromptSuffix()
	if userContext != "" {
		systemPrompt = systemPrompt + "\n\n" + userContext
	}
	return cl.Ask(ctx, systemPrompt, message)
}

// TricksterReplyWithLevel appends relationship level context to the system prompt.
func TricksterReplyWithLevel(ctx context.Context, cl *claude.Client, message string, userContext string, levelSuffix string) (string, error) {
	systemPrompt := TricksterSystemPrompt + TimeOfDayMood() + AlterEgoPromptSuffix() + levelSuffix
	if userContext != "" {
		systemPrompt = systemPrompt + "\n\n" + userContext
	}
	return cl.Ask(ctx, systemPrompt, message)
}
