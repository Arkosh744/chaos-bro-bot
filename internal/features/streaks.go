package features

// StreakReward represents a reward unlocked at a specific streak milestone.
type StreakReward struct {
	Emoji   string
	Message string
}

// StreakRewards maps streak day milestones to their rewards.
var StreakRewards = map[int]StreakReward{
	3:   {Emoji: "🔥", Message: "3 дня подряд! Разблокирована секретная команда: /wisdom теперь выдаёт двойную мудрость."},
	7:   {Emoji: "⭐", Message: "Неделя! Новый альтер-эго разблокирован: Бард."},
	14:  {Emoji: "💎", Message: "2 недели! Секретная команда: /roastme — бот roast'ит СЕБЯ."},
	30:  {Emoji: "🏆", Message: "МЕСЯЦ! Ты легенда. Разблокирован режим: /serious — бот отвечает серьёзно (1 раз в день)."},
	100: {Emoji: "👑", Message: "100 ДНЕЙ?! Ты кабан из кабанов. Секретная команда: /oracle — бот предсказывает будущее с пугающей точностью."},
}

// GetStreakReward returns the reward for a given streak day, or nil if none.
func GetStreakReward(days int) *StreakReward {
	if r, ok := StreakRewards[days]; ok {
		return &r
	}
	return nil
}
