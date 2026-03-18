package features

import (
	"log"
	"math/rand"
	"time"

	tele "gopkg.in/telebot.v4"
)

// MeditationStep represents a single step in a guided meditation.
type MeditationStep struct {
	Text     string
	Duration time.Duration
}

// Meditations is a pool of mini-meditations, each is a slice of steps with text + duration.
var Meditations = [][]MeditationStep{
	// Body scan
	{
		{"🧘 Закрой глаза...", 5 * time.Second},
		{"Почувствуй свои ступни на полу...", 8 * time.Second},
		{"Поднимись вниманием к коленям...", 8 * time.Second},
		{"Расслабь плечи... опусти их...", 8 * time.Second},
		{"Почувствуй дыхание... не меняй, просто наблюдай...", 10 * time.Second},
		{"Открой глаза когда будешь готов.", 5 * time.Second},
		{"✅ *Готово.* Как тело?", 0},
	},
	// 5-4-3-2-1 grounding
	{
		{"🧘 Сядь удобно...", 5 * time.Second},
		{"Назови 5 вещей которые видишь...", 12 * time.Second},
		{"4 вещи которые слышишь...", 10 * time.Second},
		{"3 вещи которые чувствуешь (на ощупь)...", 10 * time.Second},
		{"2 запаха...", 8 * time.Second},
		{"1 вкус...", 6 * time.Second},
		{"✅ *Ты здесь и сейчас.*", 0},
	},
	// Quick calm
	{
		{"🧘 Стоп. Пауза.", 5 * time.Second},
		{"Три глубоких вдоха...", 10 * time.Second},
		{"Назови одну вещь за которую благодарен сегодня...", 10 * time.Second},
		{"Улыбнись. Даже если не хочешь.", 5 * time.Second},
		{"✅ *Всё. Можно дальше.*", 0},
	},
}

// RunMeditation guides the user through a randomly selected meditation by editing the message.
func RunMeditation(bot *tele.Bot, msg *tele.Message, onComplete func()) {
	meditation := Meditations[rand.Intn(len(Meditations))]
	for _, step := range meditation {
		if _, err := bot.Edit(msg, step.Text, tele.ModeMarkdown); err != nil {
			log.Printf("meditation edit error: %v", err)
		}
		if step.Duration > 0 {
			time.Sleep(step.Duration)
		}
	}
	if onComplete != nil {
		onComplete()
	}
}
