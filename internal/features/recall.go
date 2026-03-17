package features

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

// RecallPrompt is the system prompt for generating contextual memory references.
const RecallPrompt = `На основе summary и профиля пользователя, сгенерируй ОДНУ короткую отсылку к прошлому разговору или факту.

Summary:
%s

Профиль:
%s

Правила:
- Одно предложение, естественное, как будто вспомнил
- Не "помнишь ты говорил" — а более живое: "Кстати, как там твой...", "О, а ты всё ещё...", "Эй, ты тогда говорил что..."
- Если нет интересных фактов — ответь ПУСТО
- На русском`

// ShouldRecall returns true ~15% of the time, but only if we have context.
func ShouldRecall() bool {
	return rand.Intn(100) < 15
}

// GenerateRecall creates a memory reference based on user context.
func GenerateRecall(ctx context.Context, cl *claude.Client, summary, profile string) (string, error) {
	if summary == "" && profile == "" {
		return "", nil
	}

	prompt := fmt.Sprintf(RecallPrompt, summary, profile)

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
