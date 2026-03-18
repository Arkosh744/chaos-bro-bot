package features

import (
	"sort"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

// AnalyzeFrequentPhrases finds frequently used words (3+ occurrences) in user messages.
func AnalyzeFrequentPhrases(msgs []storage.Message) []string {
	wordCount := make(map[string]int)

	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}

		words := strings.Fields(strings.ToLower(m.Text))
		for _, w := range words {
			// Skip short/common words
			if len(w) < 4 {
				continue
			}
			// Skip common Russian words
			if isCommonWord(w) {
				continue
			}

			wordCount[w]++
		}
	}

	type wordFreq struct {
		word  string
		count int
	}

	var frequent []wordFreq
	for word, count := range wordCount {
		if count >= 3 {
			frequent = append(frequent, wordFreq{word: word, count: count})
		}
	}

	// Sort by frequency descending to get the most used words first
	sort.Slice(frequent, func(i, j int) bool {
		return frequent[i].count > frequent[j].count
	})

	// Limit to top 5
	if len(frequent) > 5 {
		frequent = frequent[:5]
	}

	result := make([]string, len(frequent))
	for i, wf := range frequent {
		result[i] = wf.word
	}

	return result
}

func isCommonWord(w string) bool {
	common := map[string]bool{
		"это": true, "что": true, "как": true, "для": true, "тоже": true,
		"можно": true, "нужно": true, "есть": true, "было": true, "будет": true,
		"очень": true, "просто": true, "когда": true, "потом": true, "тебя": true,
		"себя": true, "если": true, "чтобы": true, "меня": true, "мной": true,
		"тебе": true, "своё": true, "свой": true, "этот": true, "этой": true,
		"такой": true, "какой": true, "ещё": true, "даже": true, "между": true,
		"через": true, "после": true, "перед": true, "около": true, "может": true,
	}

	return common[w]
}

// PhrasesPromptSuffix returns a prompt suffix telling the bot to use these phrases.
func PhrasesPromptSuffix(phrases []string) string {
	if len(phrases) == 0 {
		return ""
	}

	return "\n\nПользователь часто использует слова: " + strings.Join(phrases, ", ") + ". Иногда вставляй их в свои ответы естественно."
}
