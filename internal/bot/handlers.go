package bot

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"time"

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

func (b *Bot) checkAndSendAchievements(c tele.Context, event string) {
	unlocked := features.CheckAchievements(b.store, c.Sender().ID, event)
	for _, msg := range unlocked {
		if err := c.Send(msg); err != nil {
			log.Printf("[%d] achievement send error: %v", c.Sender().ID, err)
		}
	}
}

func (b *Bot) handleAchievements(c tele.Context) error {
	userID := c.Sender().ID
	names, err := b.store.GetAchievements(userID)
	if err != nil {
		return c.Send(features.RandomFallback(), menu)
	}
	if len(names) == 0 {
		return c.Send("У тебя пока нет ачивок. Давай, начинай играть.", menu)
	}

	msg := "\U0001F3C6 Твои ачивки:\n\n"
	for _, name := range names {
		if def, ok := features.Achievements[name]; ok {
			msg += fmt.Sprintf("%s %s — %s\n", def.Emoji, def.Name, def.Desc)
		}
	}
	return c.Send(msg, menu)
}

func (b *Bot) handlePhoto(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] photo", userID)

	replyFn, stop := b.startThinking(c)

	caption := c.Message().Caption
	prompt := "Пользователь прислал фотку."
	if caption != "" {
		prompt = "Пользователь прислал фотку с подписью: " + caption
	}

	userCtx := b.buildUserContext(userID)
	reply, err := features.TricksterReply(context.Background(), b.claude, prompt, userCtx)
	if err != nil {
		stop()
		log.Printf("[%d] photo reply error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if _, err := b.store.SaveMessage(userID, "user", "[\U0001F4F7] "+prompt); err != nil {
		log.Printf("[%d] save photo msg error: %v", userID, err)
	}
	if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
		log.Printf("[%d] save photo reply error: %v", userID, err)
	}

	b.checkAndSendAchievements(c, "photo")

	return replyFn(reply, menu)
}

func (b *Bot) handleStart(c tele.Context) error {
	log.Printf("[%d] /start from %s", c.Sender().ID, c.Sender().Username)
	name := tricksterNames[rand.Intn(len(tricksterNames))]
	greeting := fmt.Sprintf("Йо. Я %s. Жми кнопки или просто пиши мне.", name)
	if ego := features.GetAlterEgo(); ego != nil {
		greeting = fmt.Sprintf("Йо. Сегодня я %s. Режим: %s. Жми кнопки или просто пиши.", name, ego.Name)
	}
	return c.Send(greeting, menu)
}

func (b *Bot) handleGrounding(c tele.Context) error {
	log.Printf("[%d] grounding", c.Sender().ID)
	defer b.checkAndSendAchievements(c, "grounding")
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
	defer b.checkAndSendAchievements(c, "chaos")

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

func (b *Bot) handlePrediction(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] prediction", userID)
	defer b.checkAndSendAchievements(c, "prediction")

	replyFn, stop := b.startThinking(c)
	prediction, err := b.claude.Ask(context.Background(), features.PredictionPrompt, "Предскажи")
	if err != nil {
		stop()
		log.Printf("[%d] prediction error: %v", userID, err)
		return c.Send("🔮 "+features.RandomFallback(), menu)
	}
	return replyFn("🔮 "+prediction, menu)
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	userID := c.Sender().ID
	log.Printf("[%d] text: %s", userID, text)
	defer b.checkAndSendAchievements(c, "message")

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
		b.checkAndSendAchievements(c, "easter_egg")
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

	// Offended reply if user was silent for >24h
	lastTime, err := b.store.LastMessageTime(userID)
	if err == nil && !lastTime.IsZero() && time.Since(lastTime) > 24*time.Hour {
		offended := features.OffendedReplies[rand.Intn(len(features.OffendedReplies))]
		if _, err := b.store.SaveMessage(userID, "bot", offended); err != nil {
			log.Printf("[%d] save offended error: %v", userID, err)
		}
		if err := c.Send(offended, menu); err != nil {
			log.Printf("[%d] offended send error: %v", userID, err)
		}
	}

	// Bargain: 20% chance bot demands something before answering
	if rand.Intn(5) == 0 {
		bargain := features.Bargains[rand.Intn(len(features.Bargains))]
		if err := c.Send(bargain, menu); err != nil {
			log.Printf("[%d] bargain send error: %v", userID, err)
		}
		time.Sleep(2 * time.Second)
	}

	// Build context
	userCtx := b.buildUserContext(userID)

	// Start thinking animation
	replyFn, stop := b.startThinking(c)

	var reply string

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

	// Loot drop every 10 messages
	count, cErr := b.store.IncrementCounter(userID, "messages")
	if cErr == nil && count%10 == 0 {
		loot := features.RandomLoot()
		log.Printf("[%d] loot drop #%d: %s", userID, count, loot)
		if _, err := b.store.SaveMessage(userID, "bot", loot); err != nil {
			log.Printf("[%d] save loot error: %v", userID, err)
		}
		// Send loot after main reply
		defer func() {
			if err := c.Send(loot, menu); err != nil {
				log.Printf("[%d] loot send error: %v", userID, err)
			}
		}()
	}

	// Check if summary needs update (async, don't block response)
	go b.maybeUpdateSummary(userID)

	return replyFn(reply, menu)
}

func (b *Bot) handleVoice(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] voice message", userID)
	defer b.checkAndSendAchievements(c, "voice")

	if b.whisper == nil {
		return c.Send("Голосовые не настроены. Нужен Groq API ключ.", menu)
	}

	voice := c.Message().Voice
	if voice == nil {
		return nil
	}

	replyFn, stop := b.startThinking(c)

	file, err := b.tg.FileByID(voice.FileID)
	if err != nil {
		stop()
		log.Printf("[%d] voice download error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	reader, err := b.tg.File(&file)
	if err != nil {
		stop()
		log.Printf("[%d] voice file read error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}
	defer reader.Close()

	audioData, err := io.ReadAll(reader)
	if err != nil {
		stop()
		log.Printf("[%d] voice read error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	text, err := b.whisper.Transcribe(audioData, "voice.ogg")
	if err != nil {
		stop()
		log.Printf("[%d] whisper error: %v", userID, err)
		return c.Send("Не расслышал. Попробуй ещё раз или напиши текстом.", menu)
	}

	log.Printf("[%d] transcribed: %s", userID, text)

	if _, err := b.store.SaveMessage(userID, "user", "[voice] "+text); err != nil {
		log.Printf("[%d] save voice message error: %v", userID, err)
	}

	userCtx := b.buildUserContext(userID)

	reply, err := features.TricksterReply(context.Background(), b.claude, text, userCtx)
	if err != nil {
		stop()
		log.Printf("[%d] trickster error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
		log.Printf("[%d] save bot reply error: %v", userID, err)
	}

	go b.maybeUpdateSummary(userID)

	return replyFn(reply, menu)
}

func (b *Bot) handleBreathing(c tele.Context) error {
	log.Printf("[%d] breathing", c.Sender().ID)
	defer b.checkAndSendAchievements(c, "breathing")

	msg, err := b.tg.Send(c.Recipient(), "\U0001FAC1 Приготовься...", menu)
	if err != nil {
		return c.Send("Не получилось запустить таймер. "+features.RandomFallback(), menu)
	}

	go features.RunBreathing(b.tg, msg)

	return nil
}

func (b *Bot) handleCapsule(c tele.Context) error {
	userID := c.Sender().ID
	text := c.Message().Payload
	log.Printf("[%d] capsule: %s", userID, text)

	if text == "" {
		return c.Send("Формат: /capsule 7 твоё сообщение\nЧисло = через сколько дней доставить.", menu)
	}

	// Parse: first word is number of days, rest is text
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		return c.Send("Формат: /capsule 7 привет из прошлого", menu)
	}

	days := 0
	if _, err := fmt.Sscanf(parts[0], "%d", &days); err != nil || days < 1 || days > 365 {
		return c.Send("Дней от 1 до 365. Пример: /capsule 30 привет из прошлого", menu)
	}

	deliverAt := time.Now().AddDate(0, 0, days)
	if err := b.store.SaveCapsule(userID, parts[1], deliverAt); err != nil {
		log.Printf("[%d] save capsule error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	b.checkAndSendAchievements(c, "capsule")

	return c.Send(fmt.Sprintf("\u231B Записал. Доставлю через %d дн. Ты забудешь, а я — нет.", days), menu)
}

var moodReplies = map[int][]string{
	1:  {"Ого, жёстко. Ну, день только начался.", "Хуже быть не может — значит дальше только вверх. Или нет."},
	2:  {"Два из десяти? Бывало и лучше, а?", "Ладно, хотя бы честно."},
	3:  {"Три — это 'мог бы и не просыпаться'. Понимаю.", "Бывает. Кофе уже пил?"},
	4:  {"Четвёрка. Не дно, но близко. Держись.", "Могло быть хуже. Могло быть и лучше."},
	5:  {"Ровно посередине. Идеальный баланс хуйни.", "Пятёрка — это 'живой и ладно'."},
	6:  {"Шесть — это 'нормально'. Скучно, но стабильно.", "Выше среднего. Неплохо для утра."},
	7:  {"Семёрка! Кто-то сегодня выспался.", "Хороший показатель. Не расслабляйся."},
	8:  {"Восемь? Красава. Что случилось?", "Восьмёрка. Подозрительно хорошо."},
	9:  {"Девять?! Ты точно не врёшь?", "Девятка. Ну ты монстр."},
	10: {"Десять?! Кто ты и что сделал с настоящим юзером?", "Максимум? Ну окей, сегодня ты бог."},
}

func (b *Bot) handleMoodScore(c tele.Context, score int) error {
	userID := c.Sender().ID
	log.Printf("[%d] mood score: %d", userID, score)

	// Save mood to storage
	if _, err := b.store.SaveMessage(userID, "user", fmt.Sprintf("[mood:%d]", score)); err != nil {
		log.Printf("[%d] save mood error: %v", userID, err)
	}

	replies := moodReplies[score]
	reply := replies[rand.Intn(len(replies))]

	if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
		log.Printf("[%d] save mood reply error: %v", userID, err)
	}

	if score == 10 {
		b.checkAndSendAchievements(c, "mood_10")
	}
	if score == 1 {
		b.checkAndSendAchievements(c, "mood_1")
	}

	return c.Edit(fmt.Sprintf("Утро. Как ты от 1 до 10?\n\n*%d* — %s", score, reply), tele.ModeMarkdown)
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
		return
	}
	log.Printf("[%d] summary updated", userID)

	// Detect patterns after summary update
	pattern, err := features.DetectPatterns(context.Background(), b.claude, b.store, userID)
	if err != nil {
		log.Printf("[%d] pattern detect error: %v", userID, err)
		return
	}
	if pattern != "" {
		log.Printf("[%d] pattern detected: %s", userID, pattern)
		recipient := &tele.User{ID: userID}
		if _, err := b.tg.Send(recipient, "📊 "+pattern); err != nil {
			log.Printf("[%d] pattern send error: %v", userID, err)
		}
	}
}
