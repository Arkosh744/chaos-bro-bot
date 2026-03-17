package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

const DigestPrompt = `Ты — трикстер. Составь краткий еженедельный дайджест на основе данных.

Формат:
- Сколько сообщений написал
- О чём больше всего говорил
- Сколько раз ныл
- Что-нибудь подъёбывающее в конце

Коротко, 3-5 строк. Дерзко. На русском. Маты допустимы.`

// GenerateDigest creates a weekly trickster-style digest based on user summary and recent messages.
func GenerateDigest(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) (string, error) {
	summary, _, err := store.GetSummary(userID)
	if err != nil {
		return "", fmt.Errorf("get summary: %w", err)
	}

	msgs, err := store.GetLastMessages(userID, 50)
	if err != nil {
		return "", fmt.Errorf("get messages: %w", err)
	}

	userMsgCount := 0
	var recentTexts strings.Builder
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgCount++
		}
		fmt.Fprintf(&recentTexts, "%s: %s\n", m.Role, m.Text)
	}

	prompt := fmt.Sprintf("Summary пользователя:\n%s\n\nСообщений за неделю: %d\n\nПоследние сообщения:\n%s",
		summary, userMsgCount, recentTexts.String())

	return cl.Ask(ctx, DigestPrompt, prompt)
}
