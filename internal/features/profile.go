package features

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

var categoryLabels = map[string]string{
	"name":          "👤 Имя",
	"age":           "🎂 Возраст",
	"city":          "📍 Город",
	"job":           "💼 Работа",
	"hobbies":       "🎮 Хобби",
	"music":         "🎵 Музыка",
	"games":         "🕹 Игры",
	"food":          "🍕 Еда",
	"mood_pattern":  "😤 Настроение",
	"relationships": "💑 Отношения",
	"pets":          "🐾 Питомцы",
	"goals":         "🎯 Цели",
	"quirks":        "🤪 Странности",
}

// ExtractFacts analyzes conversation context and extracts user facts.
func ExtractFacts(ctx context.Context, cl *claude.Client, store *storage.Storage, userID int64) error {
	summary, _, err := store.GetSummary(userID)
	if err != nil {
		return fmt.Errorf("get summary: %w", err)
	}

	msgs, err := store.GetLastMessages(userID, 10)
	if err != nil {
		return fmt.Errorf("get messages: %w", err)
	}

	if summary == "" && len(msgs) == 0 {
		return nil
	}

	// Build input for extraction
	var input strings.Builder
	if summary != "" {
		input.WriteString("Summary:\n" + summary + "\n\n")
	}

	// Current profile for context
	currentProfile, _ := store.GetFactsAsText(userID)
	if currentProfile != "" {
		input.WriteString("Текущий профиль:\n" + currentProfile + "\n\n")
	}

	input.WriteString("Последние сообщения:\n")
	for _, m := range msgs {
		input.WriteString(m.Role + ": " + m.Text + "\n")
	}

	resp, err := cl.Ask(ctx, ProfileExtractPrompt, input.String())
	if err != nil {
		return fmt.Errorf("claude extract: %w", err)
	}

	resp = strings.TrimSpace(resp)
	if resp == "ПУСТО" || resp == "" {
		return nil
	}

	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		category := strings.TrimSpace(parts[0])
		fact := strings.TrimSpace(parts[1])

		if _, ok := categoryLabels[category]; !ok {
			continue
		}
		if fact == "" {
			continue
		}

		if err := store.SaveFact(userID, category, fact); err != nil {
			log.Printf("[%d] save fact error (%s): %v", userID, category, err)
		}
	}

	return nil
}

// FormatProfile builds a readable profile string.
func FormatProfile(facts []storage.UserFact) string {
	if len(facts) == 0 {
		return "Пока ничего не знаю о тебе. Пиши больше — разберусь."
	}

	var sb strings.Builder
	sb.WriteString("📋 *Твой профиль:*\n\n")

	for _, f := range facts {
		label, ok := categoryLabels[f.Category]
		if !ok {
			label = f.Category
		}
		sb.WriteString(label + ": " + f.Fact + "\n")
	}

	sb.WriteString("\n_Собрано из наших разговоров. Я внимательный._")
	return sb.String()
}
