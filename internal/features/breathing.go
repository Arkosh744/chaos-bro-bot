package features

import (
	"fmt"
	"log"
	"time"

	tele "gopkg.in/telebot.v4"
)

type breathPhase struct {
	emoji    string
	label    string
	seconds  int
	barFill  string
	barEmpty string
}

var phases = []breathPhase{
	{"🌬", "Вдох", 4, "▓", "░"},
	{"⏸", "Задержка", 4, "█", "░"},
	{"💨", "Выдох", 4, "▓", "░"},
}

// BreathingRounds is the number of full cycles to perform.
const BreathingRounds = 3

func buildBreathText(phase breathPhase, sec, round int) string {
	total := phase.seconds
	filled := sec
	empty := total - sec

	bar := ""
	for i := 0; i < filled; i++ {
		bar += phase.barFill
	}
	for i := 0; i < empty; i++ {
		bar += phase.barEmpty
	}

	return fmt.Sprintf(
		"%s  *%s*  [%s]  %dс\n\n_Раунд %d из %d_",
		phase.emoji, phase.label, bar, total-sec, round, BreathingRounds,
	)
}

// RunBreathing edits the given message through a guided breathing exercise.
// If onComplete is non-nil, it is called after the final message is sent.
func RunBreathing(bot *tele.Bot, msg *tele.Message, onComplete func()) {
	for round := 1; round <= BreathingRounds; round++ {
		for _, phase := range phases {
			for sec := 0; sec <= phase.seconds; sec++ {
				text := buildBreathText(phase, sec, round)
				if _, err := bot.Edit(msg, text, tele.ModeMarkdown); err != nil {
					log.Printf("breathing edit error: %v", err)
				}
				if sec < phase.seconds {
					time.Sleep(1 * time.Second)
				}
			}
		}
	}

	if _, err := bot.Edit(msg, "✅ *Готово.*\n\nКак ощущения? Напиши одним словом.", tele.ModeMarkdown); err != nil {
		log.Printf("breathing final edit error: %v", err)
	}

	if onComplete != nil {
		onComplete()
	}
}
