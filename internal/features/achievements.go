package features

import (
	"fmt"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

// AchievementDef describes a single achievement.
type AchievementDef struct {
	Name  string
	Emoji string
	Desc  string
}

// Achievements is the full catalog of unlockable achievements.
var Achievements = map[string]AchievementDef{
	"first_message":   {Name: "Первый контакт", Emoji: "\U0001F31F", Desc: "Написал первое сообщение"},
	"night_owl":       {Name: "Сова", Emoji: "\U0001F989", Desc: "Написал после полуночи"},
	"early_bird":      {Name: "Жаворонок", Emoji: "\U0001F426", Desc: "Написал до 7 утра"},
	"chatterbox_50":   {Name: "Болтун", Emoji: "\U0001F5E3\uFE0F", Desc: "50 сообщений"},
	"chatterbox_100":  {Name: "Трепло", Emoji: "\U0001F4E2", Desc: "100 сообщений"},
	"chatterbox_500":  {Name: "Легенда чата", Emoji: "\U0001F451", Desc: "500 сообщений"},
	"first_chaos":     {Name: "Первый хаос", Emoji: "\U0001F3B2", Desc: "Нажал 'Ебани куба'"},
	"first_ground":    {Name: "Заземлённый", Emoji: "\U0001F30D", Desc: "Нажал 'Очнись'"},
	"first_breath":    {Name: "Дышащий", Emoji: "\U0001FAC1", Desc: "Прошёл дыхательный таймер"},
	"first_predict":   {Name: "Гадатель", Emoji: "\U0001F52E", Desc: "Запросил предсказание"},
	"first_capsule":   {Name: "Путешественник во времени", Emoji: "\u231B", Desc: "Создал капсулу времени"},
	"first_voice":     {Name: "Голосистый", Emoji: "\U0001F3A4", Desc: "Отправил голосовое"},
	"easter_egg":      {Name: "Охотник за яйцами", Emoji: "\U0001F95A", Desc: "Нашёл пасхалку"},
	"mood_10":         {Name: "Бог", Emoji: "\u26A1", Desc: "Оценил настроение на 10"},
	"mood_1":          {Name: "Дно", Emoji: "\U0001F573\uFE0F", Desc: "Оценил настроение на 1"},
	"weekend_warrior": {Name: "Воин выходных", Emoji: "\u2694\uFE0F", Desc: "Написал в субботу И воскресенье"},
	"first_photo":     {Name: "Фотограф", Emoji: "\U0001F4F8", Desc: "Прислал фотку"},
}

// CheckAchievements checks and unlocks achievements based on event type.
// Returns list of newly unlocked achievement notification messages.
func CheckAchievements(store *storage.Storage, userID int64, event string) []string {
	var unlocked []string

	check := func(name string) {
		def, ok := Achievements[name]
		if !ok {
			return
		}
		isNew, err := store.UnlockAchievement(userID, name)
		if err != nil || !isNew {
			return
		}
		unlocked = append(unlocked, fmt.Sprintf("\U0001F3C6 Ачивка: %s %s — %s", def.Emoji, def.Name, def.Desc))
	}

	switch event {
	case "message":
		check("first_message")

		hour := time.Now().Hour()
		if hour >= 0 && hour < 5 {
			check("night_owl")
		}
		if hour >= 5 && hour < 7 {
			check("early_bird")
		}

		// Counter-based achievements using the existing "messages" counter
		count, err := store.GetCounter(userID, "messages")
		if err == nil {
			if count >= 50 {
				check("chatterbox_50")
			}
			if count >= 100 {
				check("chatterbox_100")
			}
			if count >= 500 {
				check("chatterbox_500")
			}
		}

		wd := time.Now().Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			check("weekend_warrior")
		}
	case "chaos":
		check("first_chaos")
	case "grounding":
		check("first_ground")
	case "breathing":
		check("first_breath")
	case "prediction":
		check("first_predict")
	case "capsule":
		check("first_capsule")
	case "voice":
		check("first_voice")
	case "easter_egg":
		check("easter_egg")
	case "mood_10":
		check("mood_10")
	case "mood_1":
		check("mood_1")
	case "photo":
		check("first_photo")
	}

	return unlocked
}
