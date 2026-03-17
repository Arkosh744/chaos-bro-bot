package features

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

// AnalyzeStyle examines recent user messages and produces a human-readable
// description of the writing style to inject into MirrorPrompt.
func AnalyzeStyle(messages []storage.Message) string {
	// Filter only user messages
	var userTexts []string
	for _, m := range messages {
		if m.Role == "user" {
			userTexts = append(userTexts, m.Text)
		}
	}

	if len(userTexts) == 0 {
		return "Недостаточно данных для анализа стиля. Пиши в своём обычном стиле трикстера."
	}

	var totalLen int
	var capsCount int
	var totalChars int
	var noPunctuation int
	var emojiCount int
	var exclamationCount int
	var questionCount int
	var ellipsisCount int

	for _, text := range userTexts {
		totalLen += utf8.RuneCountInString(text)

		for _, r := range text {
			totalChars++
			if unicode.IsUpper(r) && unicode.IsLetter(r) {
				capsCount++
			}
		}

		trimmed := strings.TrimSpace(text)
		if len(trimmed) > 0 {
			lastChar := trimmed[len(trimmed)-1]
			if lastChar != '.' && lastChar != '!' && lastChar != '?' {
				noPunctuation++
			}
		}

		emojiCount += countEmojis(text)

		exclamationCount += strings.Count(text, "!")
		questionCount += strings.Count(text, "?")
		ellipsisCount += strings.Count(text, "...")
	}

	msgCount := len(userTexts)
	avgLen := totalLen / msgCount

	var traits []string

	// Average message length
	switch {
	case avgLen < 15:
		traits = append(traits, "Пишет ОЧЕНЬ коротко (1-2 слова)")
	case avgLen < 40:
		traits = append(traits, "Пишет коротко (одно предложение)")
	case avgLen < 100:
		traits = append(traits, "Пишет средними сообщениями")
	default:
		traits = append(traits, "Пишет длинными сообщениями")
	}

	// Caps usage
	if totalChars > 0 {
		capsRatio := float64(capsCount) / float64(totalChars)
		if capsRatio > 0.3 {
			traits = append(traits, "Часто использует КАПС")
		}
	}

	// Punctuation
	noPunctRatio := float64(noPunctuation) / float64(msgCount)
	if noPunctRatio > 0.7 {
		traits = append(traits, "Не ставит точки в конце предложений")
	}

	if exclamationCount > msgCount {
		traits = append(traits, "Часто использует восклицательные знаки!")
	}

	if questionCount > msgCount {
		traits = append(traits, "Часто задаёт вопросы")
	}

	if ellipsisCount > 0 {
		traits = append(traits, "Использует многоточие...")
	}

	// Emoji
	if emojiCount > msgCount {
		traits = append(traits, "Активно использует эмоджи")
	} else if emojiCount == 0 {
		traits = append(traits, "Не использует эмоджи")
	}

	// Sample messages for Claude to see the raw style
	var samples []string
	limit := 5
	if len(userTexts) < limit {
		limit = len(userTexts)
	}
	for i := len(userTexts) - limit; i < len(userTexts); i++ {
		samples = append(samples, fmt.Sprintf("- \"%s\"", userTexts[i]))
	}

	result := "Характеристики стиля:\n" + strings.Join(traits, "\n") +
		"\n\nПримеры сообщений пользователя:\n" + strings.Join(samples, "\n")

	return result
}

// countEmojis returns a rough count of emoji characters in the text.
func countEmojis(text string) int {
	count := 0
	for _, r := range text {
		if r > 0x1F600 && r < 0x1FA00 {
			count++
		}
		if r >= 0x2600 && r <= 0x27BF {
			count++
		}
	}
	return count
}
