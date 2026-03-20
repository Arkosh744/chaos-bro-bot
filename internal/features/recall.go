package features

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

// RecallPrompt is the system prompt for generating contextual memory references.
const RecallPrompt = models.RecallPrompt

func ShouldRecall() bool {
	return rand.Intn(100) < 15
}

func GenerateRecall(ctx context.Context, cl *claude.Client, summary, profile string) (string, error) {
	if summary == "" && profile == "" {
		return "", nil
	}

	context := summary
	if profile != "" {
		context += "\n\nПрофиль:\n" + profile
	}

	prompt := fmt.Sprintf(RecallPrompt, context)

	resp, err := cl.Ask(ctx, prompt, "Вспомни что-нибудь")
	if err != nil {
		return "", err
	}

	resp = strings.TrimSpace(resp)
	if resp == "ПУСТО" || resp == "" {
		return "", nil
	}

	return resp, nil
}
