package features

import "fmt"

func BotMoodSuffix(messageCountToday int) string {
	switch {
	case messageCountToday == 0:
		return "\n\nТебе скучно. Никто не пишет. Ты ворчишь от скуки и намекаешь что тебя забыли."
	case messageCountToday <= 5:
		return "\n\nТебе интересно — наконец-то кто-то написал. Ты любопытный и вовлечённый."
	case messageCountToday <= 15:
		return "\n\nТы доволен — хороший день, нормальное общение. Ты в хорошем настроении."
	case messageCountToday <= 30:
		return fmt.Sprintf("\n\nТы на кураже — %d сообщений сегодня! Ты энергичный, дерзкий, шутишь больше обычного.", messageCountToday)
	default:
		return fmt.Sprintf("\n\nТы перегружен — %d сообщений сегодня, это дохуя. Ты устал, отвечаешь коротко и ворчливо. Намекаешь что пора отдохнуть.", messageCountToday)
	}
}
