package features

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

func AnalyzeStyle(messages []storage.Message) string {
	var userTexts []string
	for _, m := range messages {
		if m.Role == "user" {
			userTexts = append(userTexts, m.Text)
		}
	}

	if len(userTexts) == 0 {
		return models.MsgMirrorNoData
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

	switch {
	case avgLen < 15:
		traits = append(traits, models.MsgStyleVShort)
	case avgLen < 40:
		traits = append(traits, models.MsgStyleShort)
	case avgLen < 100:
		traits = append(traits, models.MsgStyleMedium)
	default:
		traits = append(traits, models.MsgStyleLong)
	}

	if totalChars > 0 {
		capsRatio := float64(capsCount) / float64(totalChars)
		if capsRatio > 0.3 {
			traits = append(traits, models.MsgStyleCaps)
		}
	}

	noPunctRatio := float64(noPunctuation) / float64(msgCount)
	if noPunctRatio > 0.7 {
		traits = append(traits, models.MsgStyleNoDots)
	}

	if exclamationCount > msgCount {
		traits = append(traits, models.MsgStyleExclam)
	}

	if questionCount > msgCount {
		traits = append(traits, models.MsgStyleQuestions)
	}

	if ellipsisCount > 0 {
		traits = append(traits, models.MsgStyleEllipsis)
	}

	if emojiCount > msgCount {
		traits = append(traits, models.MsgStyleEmoji)
	} else if emojiCount == 0 {
		traits = append(traits, models.MsgStyleNoEmoji)
	}

	var samples []string
	limit := 5
	if len(userTexts) < limit {
		limit = len(userTexts)
	}
	for i := len(userTexts) - limit; i < len(userTexts); i++ {
		samples = append(samples, fmt.Sprintf("- \"%s\"", userTexts[i]))
	}

	result := models.MsgStyleHeader + strings.Join(traits, "\n") +
		models.MsgStyleSamples + strings.Join(samples, "\n")

	return result
}

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
