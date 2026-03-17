package features

import (
	"fmt"
	"log"
	"time"

	tele "gopkg.in/telebot.v4"
)

// BreathingStep defines a single phase of the breathing exercise.
type BreathingStep struct {
	Text     string
	Duration time.Duration
}

// BreathingCycle is the sequence of steps in one breathing round.
var BreathingCycle = []BreathingStep{
	{"🫁 Вдох...", 4 * time.Second},
	{"⏸️ Задержи...", 4 * time.Second},
	{"💨 Выдох...", 4 * time.Second},
}

// BreathingRounds is the number of full cycles to perform.
const BreathingRounds = 3

// RunBreathing edits the given message through a guided breathing exercise.
func RunBreathing(bot *tele.Bot, msg *tele.Message) {
	for round := 0; round < BreathingRounds; round++ {
		for _, step := range BreathingCycle {
			text := fmt.Sprintf("%s  (%d/%d)", step.Text, round+1, BreathingRounds)

			if _, err := bot.Edit(msg, text); err != nil {
				log.Printf("breathing edit error: %v", err)
				return
			}

			time.Sleep(step.Duration)
		}
	}

	if _, err := bot.Edit(msg, "✅ Готово. Как ощущения?"); err != nil {
		log.Printf("breathing final edit error: %v", err)
	}
}
