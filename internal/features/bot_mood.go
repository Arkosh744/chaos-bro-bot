package features

import (
	"fmt"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

func BotMoodSuffix(messageCountToday int) string {
	switch {
	case messageCountToday == 0:
		return models.BotMoodBored
	case messageCountToday <= 5:
		return models.BotMoodCurious
	case messageCountToday <= 15:
		return models.BotMoodHappy
	case messageCountToday <= 30:
		return fmt.Sprintf(models.FmtBotMoodHyper, messageCountToday)
	default:
		return fmt.Sprintf(models.FmtBotMoodTired, messageCountToday)
	}
}
