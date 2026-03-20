package features

import (
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

const SOSMessage = models.SOSMessage

func IsSOS(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range models.SOSKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
