package bot

import (
	"log"
	"sync"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
	tele "gopkg.in/telebot.v4"
)

var thinkingPhrases = models.ThinkingPhrases

// startThinking sends a typing indicator, then a "thinking" message and animates dots.
// Returns reply func that deletes the thinking message and sends the real answer.
func (b *Bot) startThinking(c tele.Context) (reply func(text string, opts ...interface{}) error, stop func()) {
	// Send typing action so user sees "typing..." immediately
	if err := b.tg.Notify(c.Recipient(), tele.Typing); err != nil {
		log.Printf("[%d] typing notify error: %v", c.Sender().ID, err)
	}

	// Send WITHOUT reply keyboard — otherwise Telegram blocks Edit
	msg, err := b.tg.Send(c.Recipient(), models.ThinkingInitial)
	if err != nil {
		log.Printf("[%d] thinking send error: %v", c.Sender().ID, err)
		return func(text string, opts ...interface{}) error {
			return c.Send(text, opts...)
		}, func() {}
	}

	var mu sync.Mutex
	stopped := false
	step := 2

	ticker := time.NewTicker(3 * time.Second)

	go func() {
		for range ticker.C {
			mu.Lock()
			if stopped {
				mu.Unlock()
				return
			}
			phrase := thinkingPhrases[step%len(thinkingPhrases)]
			step++
			mu.Unlock()

			if _, err := b.tg.Edit(msg, phrase); err != nil {
				log.Printf("[%d] thinking edit error: %v", c.Sender().ID, err)
			}
		}
	}()

	stopFn := func() {
		mu.Lock()
		stopped = true
		mu.Unlock()
		ticker.Stop()
	}

	replyFn := func(text string, opts ...interface{}) error {
		stopFn()
		// Delete the thinking message
		if err := b.tg.Delete(msg); err != nil {
			log.Printf("[%d] thinking delete error: %v", c.Sender().ID, err)
		}
		// Send real answer with reply keyboard
		return c.Send(text, opts...)
	}

	return replyFn, stopFn
}
