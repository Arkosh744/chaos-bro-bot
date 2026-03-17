package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	tele "gopkg.in/telebot.v4"
)

var tricksterNames = []string{
	"Локи", "Гримшоу Пепельный", "Шут Трёхликий", "Морфей с Района",
	"Джинн Кривое Зеркало", "Пак Лунный", "Рейнеке-лис",
	"Коловрат Бессонный", "Чеширский Бродяга", "Робин Безголовый",
	"Ананси Восьмирукий", "Барон Самди", "Койот Пыльный",
	"Гермес Подъездный", "Одиссей Диванный", "Мерлин Бухой",
	"Фенрир Домашний", "Тиль Уленшпигель", "Джокер из Пятёрки",
	"Голлум с Авито", "Добби Свободный", "Геральт из Пятёрочки",
	"Данте с Районной", "Кратос Уставший", "Довакин Ленивый",
}

func (b *Bot) handleStart(c tele.Context) error {
	log.Printf("[%d] /start from %s", c.Sender().ID, c.Sender().Username)
	name := tricksterNames[rand.Intn(len(tricksterNames))]
	greeting := fmt.Sprintf("Йо. Я %s. Жми кнопки или просто пиши мне.", name)
	return c.Send(greeting, menu)
}

func (b *Bot) handleGrounding(c tele.Context) error {
	log.Printf("[%d] grounding", c.Sender().ID)
	technique := features.RandomGrounding()

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(btnMoreGround))
	return c.Send("👁 "+technique, menu, inline)
}

func (b *Bot) handleGroundingMore(c tele.Context) error {
	log.Printf("[%d] grounding more (edit)", c.Sender().ID)
	technique := features.RandomGrounding()

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(btnMoreGround))
	return c.Edit("👁 "+technique, inline)
}

func (b *Bot) handleChaos(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] chaos", userID)

	// Sleep mode: no claude calls between 23:00 and 09:00
	if features.IsSleepTime() {
		reply := features.SleepReplies[rand.Intn(len(features.SleepReplies))]
		log.Printf("[%d] chaos sleep mode reply", userID)
		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save bot message error: %v", userID, err)
		}
		return c.Send(reply, menu)
	}

	reply, stop := b.startThinking(c)
	task, err := features.GenerateChaos(context.Background(), b.claude)
	if err != nil {
		stop()
		log.Printf("[%d] chaos error: %v", userID, err)
		task = features.RandomChaos()
		return c.Send("🎲 "+task, menu)
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(btnMoreChaos))
	return reply("🎲 "+task, inline)
}

func (b *Bot) handleChaosMore(c tele.Context) error {
	log.Printf("[%d] chaos more (edit)", c.Sender().ID)

	// For "more" we edit the existing message to thinking, then to result
	if _, err := b.tg.Edit(c.Message(), "🤔"); err != nil {
		log.Printf("[%d] chaos more edit error: %v", c.Sender().ID, err)
	}

	task, err := features.GenerateChaos(context.Background(), b.claude)
	if err != nil {
		log.Printf("[%d] chaos error: %v", c.Sender().ID, err)
		task = features.RandomChaos()
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(btnMoreChaos))
	return c.Edit("🎲 "+task, inline)
}

func (b *Bot) handleRandomize(c tele.Context) error {
	log.Printf("[%d] randomize", c.Sender().ID)
	return c.Send("Окей, кидай вопрос. Я решу за тебя.", menu)
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	userID := c.Sender().ID
	log.Printf("[%d] text: %s", userID, text)

	// Save user message
	if _, err := b.store.SaveMessage(userID, "user", text); err != nil {
		log.Printf("[%d] save message error: %v", userID, err)
	}

	// Easter eggs: instant reply for specific keywords
	if reply, ok := features.EasterEggs[strings.ToLower(text)]; ok {
		log.Printf("[%d] easter egg match", userID)
		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save bot message error: %v", userID, err)
		}
		return c.Send(reply, menu)
	}

	// Sleep mode: no claude calls between 23:00 and 09:00
	if features.IsSleepTime() {
		reply := features.SleepReplies[rand.Intn(len(features.SleepReplies))]
		log.Printf("[%d] sleep mode reply", userID)
		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save bot message error: %v", userID, err)
		}
		return c.Send(reply, menu)
	}

	// Build context
	userCtx := b.buildUserContext(userID)

	// Start thinking animation
	replyFn, stop := b.startThinking(c)

	var reply string
	var err error

	if len(text) > 0 && text[len(text)-1] == '?' {
		reply, err = features.Decide(context.Background(), b.claude, text, userCtx)
		if err != nil {
			stop()
			log.Printf("[%d] randomizer error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}
		reply = "🎰 " + reply
	} else {
		reply, err = features.TricksterReply(context.Background(), b.claude, text, userCtx)
		if err != nil {
			stop()
			log.Printf("[%d] trickster error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}
	}

	// Save bot reply
	if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
		log.Printf("[%d] save bot message error: %v", userID, err)
	}

	// Check if summary needs update (async, don't block response)
	go b.maybeUpdateSummary(userID)

	return replyFn(reply, menu)
}

func (b *Bot) buildUserContext(userID int64) string {
	summary, _, err := b.store.GetSummary(userID)
	if err != nil {
		log.Printf("[%d] get summary error: %v", userID, err)
	}

	msgs, err := b.store.GetLastMessages(userID, 5)
	if err != nil {
		log.Printf("[%d] get messages error: %v", userID, err)
	}

	return features.BuildContext(summary, msgs)
}

func (b *Bot) maybeUpdateSummary(userID int64) {
	needs, err := features.NeedsSummaryUpdate(b.store, userID)
	if err != nil {
		log.Printf("[%d] check summary error: %v", userID, err)
		return
	}
	if !needs {
		return
	}

	log.Printf("[%d] updating summary...", userID)
	if err := features.UpdateSummary(context.Background(), b.claude, b.store, userID); err != nil {
		log.Printf("[%d] update summary error: %v", userID, err)
	} else {
		log.Printf("[%d] summary updated", userID)
	}
}
