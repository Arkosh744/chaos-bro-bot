package features

import (
	"context"
	"strconv"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

const SentimentPrompt = models.SentimentPrompt

func AnalyzeSentiment(ctx context.Context, cl *claude.Client, text string) (int, error) {
	resp, err := cl.Ask(ctx, SentimentPrompt, text)
	if err != nil {
		return 0, err
	}
	score, err := strconv.Atoi(strings.TrimSpace(resp))
	if err != nil || score < 1 || score > 10 {
		return 5, nil // default to neutral
	}
	return score, nil
}
