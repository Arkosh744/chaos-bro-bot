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
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
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

// claudeReply handles the common pattern: start thinking -> call Claude -> handle error -> send reply.
func (b *Bot) claudeReply(c tele.Context, ask func() (string, error), prefix string) error {
	replyFn, stop := b.startThinking(c)
	result, err := ask()
	if err != nil {
		stop()
		log.Printf("[%d] claude error: %v", c.Sender().ID, err)
		return c.Send(prefix+features.RandomFallback(), menu)
	}
	return replyFn(prefix+result, menu)
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

func (b *Bot) handleHelp(c tele.Context) error {
	help := `🎭 *Команды Трикстера:*

*Кнопки:*
👁 Очнись — техника заземления
🎲 Ебани куба — хаос-задание
🔮 Судьба — предсказание
🎱 Кинь кости — рандомайзер решений
🔥 Зажарь — roast на основе профиля
🧙 Мудрость — абсурдная мудрость
⭐ Гороскоп — антигороскоп дня
📊 Настроение — график за 7 дней
🪞 Зеркало — копирует твой стиль (10 сообщ.)
🫁 Дыши — дыхательный таймер

*Команды:*
/profile — твой профиль
/level — уровень отношений
/achievements — ачивки
/silence — режим эмоджи (24ч)
/truth — раскрыть сегодняшнюю ложь бота
/capsule N текст — капсула времени
/help — эта справка

*Просто пиши* — трикстер ответит с характером`
	return c.Send(help, menu, tele.ModeMarkdown)
}

func (b *Bot) handleStart(c tele.Context) error {
	log.Printf("[%d] /start from %s", c.Sender().ID, c.Sender().Username)
	b.saveUserProfile(c)
	name := tricksterNames[rand.Intn(len(tricksterNames))]
	greeting := fmt.Sprintf("Йо. Я *%s*.", name)
	if ego := features.GetAlterEgo(); ego != nil {
		greeting = fmt.Sprintf("Йо. Сегодня я *%s*. Режим: _%s_.", name, ego.Name)
	}

	// First message: intro
	if err := c.Send(greeting+"\n\nЯ дерзкий друг-трикстер. Не коуч, не AI, не мамка.\nЖми кнопки или просто пиши мне. /help — все команды.", menu, tele.ModeMarkdown); err != nil {
		log.Printf("[%d] start send error: %v", c.Sender().ID, err)
	}

	// Second message: a welcome grounding technique
	technique := features.RandomGrounding()
	return c.Send("👁 Вот тебе для начала:\n\n"+technique, menu)
}

func (b *Bot) saveUserProfile(c tele.Context) {
	s := c.Sender()
	if s == nil {
		return
	}
	if err := b.store.UpsertUserProfile(s.ID, s.Username, s.FirstName, s.LastName); err != nil {
		log.Printf("[%d] upsert user profile error: %v", s.ID, err)
	}
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
	log.Printf("[%d] prediction", c.Sender().ID)
	defer b.checkAndSendAchievements(c, "prediction")

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), features.PredictionPrompt, "Предскажи")
	}, "🔮 ")
}

func (b *Bot) handleSilence(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /silence", userID)

	// Already in silence mode — toggle off
	if b.store.IsSilenceMode(userID) {
		remaining := b.store.GetSilenceRemaining(userID)
		if err := b.store.SetCounter(userID, "silence_until", 0); err != nil {
			log.Printf("[%d] reset silence mode error: %v", userID, err)
		}
		log.Printf("[%d] silence mode deactivated, had %dh remaining", userID, remaining)
		return c.Send(fmt.Sprintf("\U0001F50A Молчание снято. Оставалось %dч.", remaining), menu)
	}

	// Activate silence mode for 24 hours
	until := time.Now().Add(24 * time.Hour)
	if err := b.store.SetCounter(userID, "silence_until", int(until.Unix())); err != nil {
		log.Printf("[%d] set silence mode error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	log.Printf("[%d] silence mode activated until %s", userID, until.Format(time.RFC3339))
	return c.Send("\U0001F92B", menu)
}

func (b *Bot) handleMirror(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /mirror", userID)

	// Check if already active — toggle off
	remaining, _ := b.store.GetCounter(userID, "mirror_remaining")
	if remaining > 0 {
		if err := b.store.SetCounter(userID, "mirror_remaining", 0); err != nil {
			log.Printf("[%d] reset mirror mode error: %v", userID, err)
		}
		return c.Send("\U0001FA9E Зеркало выключено.", menu)
	}

	if err := b.store.SetCounter(userID, "mirror_remaining", 10); err != nil {
		log.Printf("[%d] set mirror mode error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send("\U0001FA9E Зеркало активировано. Следующие 10 сообщений я буду говорить как ты.", menu)
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	userID := c.Sender().ID
	log.Printf("[%d] text: %s", userID, text)
	defer b.checkAndSendAchievements(c, "message")

	// Update user profile info
	b.saveUserProfile(c)

	// Save user message
	if _, err := b.store.SaveMessage(userID, "user", text); err != nil {
		log.Printf("[%d] save message error: %v", userID, err)
	}

	// Silence mode: respond only with emojis
	if b.store.IsSilenceMode(userID) {
		log.Printf("[%d] silence mode active", userID)

		replyFn, stop := b.startThinking(c)
		reply, err := b.claude.Ask(context.Background(), features.SilencePrompt, text)
		if err != nil {
			stop()
			log.Printf("[%d] silence reply error: %v", userID, err)
			return c.Send("\U0001F636", menu)
		}

		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save silence reply error: %v", userID, err)
		}

		return replyFn(reply, menu)
	}

	// Mirror mode: copy user's writing style
	if mirrorRemaining, _ := b.store.GetCounter(userID, "mirror_remaining"); mirrorRemaining > 0 {
		log.Printf("[%d] mirror mode active, remaining: %d", userID, mirrorRemaining)

		newVal, err := b.store.DecrementCounter(userID, "mirror_remaining")
		if err != nil {
			log.Printf("[%d] decrement mirror counter error: %v", userID, err)
		}

		// Get last messages for style analysis
		msgs, err := b.store.GetLastMessages(userID, 20)
		if err != nil {
			log.Printf("[%d] get messages for mirror error: %v", userID, err)
		}

		styleAnalysis := features.AnalyzeStyle(msgs)
		systemPrompt := fmt.Sprintf(features.MirrorPrompt, styleAnalysis)

		userCtx := b.buildUserContext(userID)
		if userCtx != "" {
			systemPrompt = systemPrompt + "\n\n" + userCtx
		}

		replyFn, stop := b.startThinking(c)
		reply, err := b.claude.Ask(context.Background(), systemPrompt, text)
		if err != nil {
			stop()
			log.Printf("[%d] mirror reply error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}

		if newVal == 0 {
			reply = reply + "\n\n\U0001FA9E Зеркало выключено. Я снова я."
		} else {
			reply = reply + fmt.Sprintf("\n\n_\U0001FA9E %d_", newVal)
		}

		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save mirror reply error: %v", userID, err)
		}

		return replyFn(reply, menu)
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

	// Offended reply if user was silent for >24h — return early, don't answer
	lastTime, err := b.store.LastMessageTime(userID)
	if err == nil && !lastTime.IsZero() && time.Since(lastTime) > 24*time.Hour {
		offended := features.OffendedReplies[rand.Intn(len(features.OffendedReplies))]
		if _, err := b.store.SaveMessage(userID, "bot", offended); err != nil {
			log.Printf("[%d] save offended error: %v", userID, err)
		}
		return c.Send(offended, menu)
	}

	// Bargain: 20% chance bot demands something before answering
	if rand.Intn(5) == 0 {
		bargain := features.Bargains[rand.Intn(len(features.Bargains))]
		if err := c.Send(bargain, menu); err != nil {
			log.Printf("[%d] bargain send error: %v", userID, err)
		}
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

	// Daily lie injection: once per day, slip a believable lie into the reply
	if features.ShouldLieToday(b.store, userID) {
		lie, truth, lieErr := features.GenerateLie(context.Background(), b.claude)
		if lieErr != nil {
			log.Printf("[%d] generate lie error: %v", userID, lieErr)
		} else {
			if saveErr := b.store.SaveLie(userID, lie, truth, time.Now().Format("2006-01-02")); saveErr != nil {
				log.Printf("[%d] save lie error: %v", userID, saveErr)
			} else {
				reply = features.InjectLie(reply, lie)
				log.Printf("[%d] daily lie injected", userID)
			}
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

	// Check for relationship level-up
	b.checkLevelUp(c)

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

	// Send WITHOUT reply keyboard — Telegram blocks Edit on messages with ReplyMarkup
	msg, err := b.tg.Send(c.Recipient(), "\U0001FAC1 Приготовься...")
	if err != nil {
		return c.Send("Не получилось запустить таймер. "+features.RandomFallback(), menu)
	}

	go features.RunBreathing(b.tg, msg, func() {
		b.checkAndSendAchievements(c, "breathing")
	})

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

func (b *Bot) handleTruth(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /truth", userID)

	today := time.Now().Format("2006-01-02")
	lie, truth, revealed, err := b.store.GetTodayLie(userID, today)
	if err != nil {
		log.Printf("[%d] get today lie error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if lie == "" {
		return c.Send("Сегодня я был честен. Или нет... \U0001F914", menu)
	}

	if revealed {
		return c.Send("Ты уже знаешь. Я соврал: "+lie+"\n\nНа самом деле: "+truth, menu)
	}

	if revealErr := b.store.RevealLie(userID, today); revealErr != nil {
		log.Printf("[%d] reveal lie error: %v", userID, revealErr)
	}

	return c.Send("Я соврал: "+lie+"\n\nНа самом деле: "+truth, menu)
}

func (b *Bot) handleProfile(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /profile", userID)

	facts, err := b.store.GetFacts(userID)
	if err != nil {
		log.Printf("[%d] get facts error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send(features.FormatProfile(facts), menu, tele.ModeMarkdown)
}

func (b *Bot) handleMoodGraph(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /mood", userID)

	entries, err := b.store.GetMoodHistory(userID, 7)
	if err != nil {
		log.Printf("[%d] mood history error: %v", userID, err)
		return c.Send("Не получилось загрузить историю настроения.", menu)
	}

	if len(entries) == 0 {
		return c.Send("Нет данных. Жди утреннего чекина.", menu)
	}

	graph := buildMoodASCII(entries)
	return c.Send("```\n"+graph+"```", menu, tele.ModeMarkdown)
}

// buildMoodASCII renders an ASCII chart of mood entries over the last 7 days.
// Each day shows the latest mood score. Days without data are left blank.
func buildMoodASCII(entries []storage.MoodEntry) string {
	now := time.Now()

	// Collect latest score per day for the last 7 days
	dayScores := make(map[int]int) // offset (0=6 days ago, 6=today) -> score
	for _, e := range entries {
		daysAgo := int(now.Sub(e.CreatedAt).Hours() / 24)
		if daysAgo > 6 {
			continue
		}
		offset := 6 - daysAgo
		dayScores[offset] = e.Score // last entry wins
	}

	// Build day labels (short weekday names in Russian)
	dayNames := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	var labels [7]string
	for i := 0; i < 7; i++ {
		d := now.AddDate(0, 0, i-6)
		wd := int(d.Weekday())
		// Convert Sunday=0 to index 6, Monday=1 to 0, etc.
		idx := (wd + 6) % 7
		labels[i] = dayNames[idx]
	}

	var sb strings.Builder
	sb.WriteString("Твоё настроение за 7 дней:\n\n")

	// Draw rows from 10 down to 1
	for row := 10; row >= 1; row-- {
		sb.WriteString(fmt.Sprintf("%2d|", row))
		for col := 0; col < 7; col++ {
			if score, ok := dayScores[col]; ok && score == row {
				sb.WriteString(" * ")
			} else {
				sb.WriteString("   ")
			}
		}
		sb.WriteString("\n")
	}

	// Bottom axis
	sb.WriteString("  +")
	sb.WriteString(strings.Repeat("---", 7))
	sb.WriteString("\n")

	// Day labels
	sb.WriteString("   ")
	for _, l := range labels {
		sb.WriteString(fmt.Sprintf("%-3s", l))
	}

	return sb.String()
}

func (b *Bot) handleRoast(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /roast", userID)

	userCtx := b.buildUserContext(userID)
	prompt := fmt.Sprintf(features.RoastPrompt, userCtx)

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), prompt, "Зароасти меня")
	}, "")
}

func (b *Bot) handleWisdom(c tele.Context) error {
	log.Printf("[%d] /wisdom", c.Sender().ID)

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), features.WisdomPrompt, "Дай мудрость")
	}, "\U0001F9D9 ")
}

func (b *Bot) handleHoroscope(c tele.Context) error {
	log.Printf("[%d] /horoscope", c.Sender().ID)

	today := time.Now().Format("2 January 2006")
	prompt := fmt.Sprintf(features.AntiHoroscopePrompt, today)

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), prompt, "Антигороскоп на сегодня")
	}, "\u2B50 ")
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

	profile, err := b.store.GetFactsAsText(userID)
	if err != nil {
		log.Printf("[%d] get profile error: %v", userID, err)
	}

	ctx := features.BuildContext(summary, msgs)
	if profile != "" {
		ctx = "Профиль пользователя:\n" + profile + "\n\n" + ctx
	}

	// Append relationship level context
	msgCount, err := b.store.GetMessageCount(userID)
	if err != nil {
		log.Printf("[%d] get message count error: %v", userID, err)
	}
	level := features.GetLevel(msgCount)
	ctx += features.LevelPromptSuffix(level)

	return ctx
}

func (b *Bot) handleLevel(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /level", userID)

	msgCount, err := b.store.GetMessageCount(userID)
	if err != nil {
		log.Printf("[%d] get message count error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	status := features.FormatLevelStatus(msgCount)
	return c.Send(status, menu, tele.ModeMarkdown)
}

// checkLevelUp detects if the user has reached a new relationship level and sends a notification.
func (b *Bot) checkLevelUp(c tele.Context) {
	userID := c.Sender().ID

	msgCount, err := b.store.GetMessageCount(userID)
	if err != nil {
		log.Printf("[%d] level check get count error: %v", userID, err)
		return
	}

	currentLevel := features.GetLevel(msgCount)

	storedLevel, err := b.store.GetCounter(userID, "relationship_level")
	if err != nil {
		storedLevel = 1
	}

	if currentLevel.Level > storedLevel {
		if err := b.store.SetCounter(userID, "relationship_level", currentLevel.Level); err != nil {
			log.Printf("[%d] set relationship level error: %v", userID, err)
			return
		}

		msg := features.LevelUpMessage(currentLevel)
		if msg != "" {
			log.Printf("[%d] level up: %d -> %d (%s)", userID, storedLevel, currentLevel.Level, currentLevel.Name)
			if err := c.Send(msg, menu, tele.ModeMarkdown); err != nil {
				log.Printf("[%d] level up send error: %v", userID, err)
			}
		}
	}
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

	// Extract user facts after summary update
	if err := features.ExtractFacts(context.Background(), b.claude, b.store, userID); err != nil {
		log.Printf("[%d] extract facts error: %v", userID, err)
	}

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
