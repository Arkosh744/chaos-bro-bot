package bot

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/integrations"
	"github.com/Arkosh744/chaos-bro-bot/internal/metrics"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
	tele "gopkg.in/telebot.v4"
)

var tricksterNames = models.TricksterNames

// isGroupChat returns true if the message is from a group or supergroup chat.
func isGroupChat(c tele.Context) bool {
	return c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup
}

// isBotMentioned checks if the bot was mentioned or replied to in a group message.
func (b *Bot) isBotMentioned(c tele.Context) bool {
	// Check if message is a reply to bot
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		if c.Message().ReplyTo.Sender.ID == b.tg.Me.ID {
			return true
		}
	}
	// Check if bot username is mentioned
	if b.tg.Me.Username != "" {
		return strings.Contains(strings.ToLower(c.Text()), "@"+strings.ToLower(b.tg.Me.Username))
	}
	return false
}

// replyOpts returns reply keyboard for private chats, nil for groups.
func (b *Bot) replyOpts(c tele.Context) *tele.ReplyMarkup {
	if isGroupChat(c) {
		return nil
	}
	return menu
}

func (b *Bot) checkAndSendAchievements(c tele.Context, event string) {
	unlocked := features.CheckAchievements(b.store, c.Sender().ID, event)
	for _, msg := range unlocked {
		if err := c.Send(msg); err != nil {
			log.Printf("[%d] achievement send error: %v", c.Sender().ID, err)
		}
	}
}

// claudeReply handles the common pattern: rate limit -> start thinking -> call Claude -> handle error -> send reply.
func (b *Bot) claudeReply(c tele.Context, ask func() (string, error), prefix string) error {
	userID := c.Sender().ID
	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	// Save the user's trigger (command text or button text)
	trigger := c.Text()
	if trigger == "" {
		trigger = "[button]"
	}
	if _, err := b.store.SaveMessage(userID, "user", trigger); err != nil {
		log.Printf("[%d] save trigger error: %v", userID, err)
	}

	b.incrementRateLimit(userID)
	replyFn, stop := b.startThinking(c)
	result, err := ask()
	if err != nil {
		stop()
		log.Printf("[%d] claude error: %v", userID, err)
		return c.Send(prefix+features.RandomFallback(), menu)
	}

	// Save bot reply
	if _, err := b.store.SaveMessage(userID, "bot", prefix+result); err != nil {
		log.Printf("[%d] save claude reply error: %v", userID, err)
	}

	return replyFn(prefix+result, menu)
}

// claudeReplyWithRepeat is like claudeReply but adds an inline "repeat" button.
func (b *Bot) claudeReplyWithRepeat(c tele.Context, ask func() (string, error), prefix, repeatCallback string) error {
	userID := c.Sender().ID
	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	trigger := c.Text()
	if trigger == "" {
		trigger = "[button]"
	}
	if _, err := b.store.SaveMessage(userID, "user", trigger); err != nil {
		log.Printf("[%d] save trigger error: %v", userID, err)
	}

	b.incrementRateLimit(userID)
	replyFn, stop := b.startThinking(c)
	result, err := ask()
	if err != nil {
		stop()
		log.Printf("[%d] claude error: %v", userID, err)
		return c.Send(prefix+features.RandomFallback(), menu)
	}

	if _, err := b.store.SaveMessage(userID, "bot", prefix+result); err != nil {
		log.Printf("[%d] save claude reply error: %v", userID, err)
	}

	inline := &tele.ReplyMarkup{}
	btn := inline.Data(models.BtnRepeatLabel, repeatCallback)
	inline.Inline(inline.Row(btn))

	return replyFn(prefix+result, menu, inline)
}

func (b *Bot) handleAchievements(c tele.Context) error {
	userID := c.Sender().ID
	names, err := b.store.GetAchievements(userID)
	if err != nil {
		return c.Send(features.RandomFallback(), menu)
	}
	if len(names) == 0 {
		return c.Send(models.MsgNoAchievements, menu)
	}

	msg := models.MsgAchievementsHeader
	unlockedSet := make(map[string]bool, len(names))
	for _, name := range names {
		unlockedSet[name] = true
		if def, ok := features.Achievements[name]; ok {
			msg += fmt.Sprintf("%s %s — %s\n", def.Emoji, def.Name, def.Desc)
		}
	}

	// Show progress for closest locked count-based achievements
	msgCount, _ := b.store.GetCounter(userID, "messages")
	type achProgress struct {
		name     string
		emoji    string
		current  int
		target   int
		achName  string
	}
	var progress []achProgress
	countAchs := map[string]int{
		"chatterbox_50":  50,
		"chatterbox_100": 100,
		"chatterbox_500": 500,
	}
	for key, threshold := range countAchs {
		if unlockedSet[key] {
			continue
		}
		if msgCount > 0 && msgCount < threshold {
			def := features.Achievements[key]
			progress = append(progress, achProgress{
				name: key, emoji: def.Emoji, current: msgCount, target: threshold, achName: def.Name,
			})
		}
	}

	// Sort by closest to unlock and take top 3
	for i := 0; i < len(progress); i++ {
		for j := i + 1; j < len(progress); j++ {
			if float64(progress[j].current)/float64(progress[j].target) > float64(progress[i].current)/float64(progress[i].target) {
				progress[i], progress[j] = progress[j], progress[i]
			}
		}
	}
	if len(progress) > 3 {
		progress = progress[:3]
	}

	if len(progress) > 0 {
		msg += models.MsgAchievementsNext
		for _, p := range progress {
			msg += fmt.Sprintf("\U0001F512 %s %s (%d/%d)\n", p.emoji, p.achName, p.current, p.target)
		}
	}

	return c.Send(msg, menu)
}

func (b *Bot) handlePhoto(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] photo", userID)

	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	b.incrementRateLimit(userID)
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
	return c.Send(models.MsgHelp, menu, tele.ModeMarkdown)
}

func (b *Bot) handleStart(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /start from %s", userID, c.Sender().Username)
	b.saveUserProfile(c)

	// Check if returning user
	msgCount, _ := b.store.GetMessageCount(userID)
	if msgCount > 0 {
		streak, _ := b.store.GetCounter(userID, "streak_days")
		level := features.GetLevel(msgCount)
		achievements, _ := b.store.GetAchievements(userID)

		status := fmt.Sprintf(models.FmtStartReturning,
			level.Emoji, level.Name, level.Level, streak, len(achievements))
		return c.Send(status, menu)
	}

	name := tricksterNames[rand.Intn(len(tricksterNames))]
	greeting := fmt.Sprintf(models.FmtStartGreeting, name)
	if ego := features.GetAlterEgo(); ego != nil {
		greeting = fmt.Sprintf(models.FmtStartAlterEgo, name, ego.Name)
	}

	// First message: intro
	if err := c.Send(greeting+models.MsgStartIntro, menu, tele.ModeMarkdown); err != nil {
		log.Printf("[%d] start send error: %v", userID, err)
	}

	// Second message: a welcome grounding technique
	technique := features.RandomGrounding()
	return c.Send(models.MsgStartGrounding+technique, menu)
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

	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
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
	return c.Send(models.MsgRandomizerPrompt, menu)
}

func (b *Bot) handlePrediction(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] prediction", userID)
	defer b.checkAndSendAchievements(c, "prediction")

	return b.claudeReplyWithRepeat(c, func() (string, error) {
		userCtx := b.buildUserContext(userID)
		prompt := features.PredictionPrompt
		if userCtx != "" {
			prompt = prompt + "\n\nКонтекст пользователя:\n" + userCtx
		}
		return b.claude.Ask(context.Background(), prompt, "Предскажи")
	}, "🔮 ", "repeat_prediction")
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
		return c.Send(fmt.Sprintf(models.FmtSilenceOff, remaining), menu)
	}

	// Activate silence mode for 24 hours
	until := time.Now().Add(24 * time.Hour)
	if err := b.store.SetCounter(userID, "silence_until", int(until.Unix())); err != nil {
		log.Printf("[%d] set silence mode error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	log.Printf("[%d] silence mode activated until %s", userID, until.Format(time.RFC3339))
	return c.Send(models.MsgSilenceOn, menu)
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
		return c.Send(models.MsgMirrorOff, menu)
	}

	if err := b.store.SetCounter(userID, "mirror_remaining", 10); err != nil {
		log.Printf("[%d] set mirror mode error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send(models.MsgMirrorOn, menu)
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	userID := c.Sender().ID
	logText := text
	if len(logText) > 50 {
		logText = logText[:50] + "..."
	}
	log.Printf("[%d] text: %s", userID, logText)
	metrics.IncrementMessages()
	metrics.TrackActiveUser(userID)
	defer b.checkAndSendAchievements(c, "message")

	// Update user profile info
	b.saveUserProfile(c)

	// Group chat: save with chatID, apply group filtering logic
	if isGroupChat(c) {
		chatID := c.Chat().ID
		senderName := c.Sender().FirstName
		if _, err := b.store.SaveMessage(chatID, "user", senderName+": "+text); err != nil {
			log.Printf("[%d] save group message error: %v", chatID, err)
		}

		// Check active duel answers before anything else
		if b.handleDuelAnswer(c) {
			return nil
		}

		// Check active quest answers
		if b.handleQuestAnswer(c) {
			return nil
		}

		if !b.isBotMentioned(c) {
			// Random interject chance — runtime config with fallback to startup value
			interjectChance := b.store.GetRuntimeConfigInt("interject_chance", b.groupInterjectChance)
			if interjectChance > 0 && rand.Intn(100) < interjectChance {
				go b.groupInterject(c, text)
			}
			return nil
		}

		// Strip @botname from text for processing
		if b.tg.Me.Username != "" {
			text = strings.ReplaceAll(text, "@"+b.tg.Me.Username, "")
			text = strings.TrimSpace(text)
		}

		// If text is empty after stripping mention, use a default prompt
		if text == "" {
			text = "Привет"
		}

		// Rate limit for group chat Claude calls
		if !b.checkRateLimit(c.Sender().ID) {
			return nil // silently ignore in groups
		}

		// For group messages, use simplified response flow
		ctx := b.buildGroupContext(c)
		replyFn, stop := b.startThinking(c)
		reply, err := features.TricksterReply(context.Background(), b.claude, text, ctx)
		if err != nil {
			stop()
			log.Printf("[%d] group trickster error: %v", chatID, err)
			return c.Send(features.RandomFallback())
		}

		if _, err := b.store.SaveMessage(chatID, "bot", reply); err != nil {
			log.Printf("[%d] save group bot message error: %v", chatID, err)
		}

		return replyFn(reply)
	}

	// Save user message (private chat)
	if _, err := b.store.SaveMessage(userID, "user", text); err != nil {
		log.Printf("[%d] save message error: %v", userID, err)
	}

	// Profile editing mode: save fact from user input
	if editing, _ := b.store.GetCounter(userID, "profile_editing"); editing > 0 {
		b.store.SetCounter(userID, "profile_editing", 0)
		parts := strings.SplitN(text, ":", 2)
		if len(parts) == 2 {
			category := strings.TrimSpace(parts[0])
			fact := strings.TrimSpace(parts[1])
			if _, ok := features.CategoryLabels[category]; ok && fact != "" {
				if err := b.store.SaveFact(userID, category, fact); err != nil {
					log.Printf("[%d] save profile fact error: %v", userID, err)
					return c.Send(models.MsgProfileSaveError, menu)
				}
				label := features.CategoryLabels[category]
				return c.Send(fmt.Sprintf(models.FmtProfileUpdated, label, fact), menu)
			}
		}
		return c.Send(models.MsgProfileFormatError, menu)
	}

	// Active game check: dispatch to game handler before main logic
	if gameType, _ := b.store.GetCounter(userID, "game_active"); gameType > 0 {
		return b.handleGameInput(c, gameType, text)
	}

	// Reflection mode: save reflection if waiting for input
	if reflectCat, _ := b.store.GetCounter(userID, "reflection_waiting"); reflectCat > 0 {
		var category string
		switch reflectCat {
		case 1:
			category = "good"
		case 2:
			category = "bad"
		case 3:
			category = "tomorrow"
		}

		if err := b.store.SaveReflection(userID, category, text); err != nil {
			log.Printf("[%d] save reflection error: %v", userID, err)
		}
		if err := b.store.SetCounter(userID, "reflection_waiting", 0); err != nil {
			log.Printf("[%d] reset reflection_waiting error: %v", userID, err)
		}

		log.Printf("[%d] reflection saved: %s", userID, category)
		return c.Send(models.MsgReflectionSaved, menu)
	}

	// Silence mode: respond only with emojis
	if b.store.IsSilenceMode(userID) {
		log.Printf("[%d] silence mode active", userID)

		if !b.checkRateLimit(userID) {
			return c.Send(models.MsgRateLimit, b.replyOpts(c))
		}

		b.incrementRateLimit(userID)
		replyFn, stop := b.startThinking(c)
		reply, err := b.claude.Ask(context.Background(), features.SilencePrompt, text)
		if err != nil {
			stop()
			log.Printf("[%d] silence reply error: %v", userID, err)
			return c.Send(models.MsgSilenceFallback, menu)
		}

		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save silence reply error: %v", userID, err)
		}

		return replyFn(reply, menu)
	}

	// Mirror mode: copy user's writing style
	if mirrorRemaining, _ := b.store.GetCounter(userID, "mirror_remaining"); mirrorRemaining > 0 {
		log.Printf("[%d] mirror mode active, remaining: %d", userID, mirrorRemaining)

		if !b.checkRateLimit(userID) {
			return c.Send(models.MsgRateLimit, b.replyOpts(c))
		}

		b.incrementRateLimit(userID)

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
			reply = reply + models.MsgMirrorDone
		} else if newVal == 5 {
			reply = reply + models.MsgMirrorHalf
		} else if newVal == 1 {
			reply = reply + models.MsgMirrorLast
		}

		if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
			log.Printf("[%d] save mirror reply error: %v", userID, err)
		}

		return replyFn(reply, menu)
	}

	// Interactive story mode: handle story choices
	if storyActive, _ := b.store.GetCounter(userID, "story_active"); storyActive > 0 {
		choice := strings.TrimSpace(strings.ToLower(text))
		isChoice1 := choice == "1" || choice == "первый" || choice == "один" || choice == "а"
		isChoice2 := choice == "2" || choice == "второй" || choice == "два" || choice == "б"

		if isChoice1 {
			return b.handleStoryContinue(c, "1")
		} else if isChoice2 {
			return b.handleStoryContinue(c, "2")
		} else {
			// Not a valid choice — abort story
			b.store.SetCounter(userID, "story_active", 0)
			if _, err := b.store.SaveMessage(userID, "bot", models.MsgStoryAbort); err != nil {
				log.Printf("[%d] save story abort error: %v", userID, err)
			}
			return c.Send(models.MsgStoryAbort, menu)
		}
	}

	// Custom easter eggs: check user-defined triggers before built-in
	if customEggs, err := b.store.GetCustomEasterEggs(); err == nil {
		if reply, ok := customEggs[strings.ToLower(text)]; ok {
			log.Printf("[%d] custom easter egg match", userID)
			if _, err := b.store.SaveMessage(userID, "bot", reply); err != nil {
				log.Printf("[%d] save bot message error: %v", userID, err)
			}
			return c.Send(reply, menu)
		}
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

	// Offended reply if user was silent for >24h — send but continue processing
	lastTime, err := b.store.LastMessageTime(userID)
	if err == nil && !lastTime.IsZero() && time.Since(lastTime) > 24*time.Hour {
		offended := features.OffendedReplies[rand.Intn(len(features.OffendedReplies))]
		if _, err := b.store.SaveMessage(userID, "bot", offended); err != nil {
			log.Printf("[%d] save offended error: %v", userID, err)
		}
		if err := c.Send(offended, menu); err != nil {
			log.Printf("[%d] offended send error: %v", userID, err)
		}
		// Continue processing the actual message — don't return
	}

	// Rate limit: max Claude calls per hour per user
	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	b.incrementRateLimit(userID)

	// Bargain: configurable chance (default 20%) bot demands something before answering
	bargainChance := b.store.GetRuntimeConfigInt("bargain_chance", 20)
	if bargainChance > 0 && rand.Intn(100) < bargainChance {
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
		reply = models.MsgRandomizerPrefix + reply
	} else {
		reply, err = features.TricksterReply(context.Background(), b.claude, text, userCtx)
		if err != nil {
			stop()
			log.Printf("[%d] trickster error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}
	}

	// Contextual recall: ~15% chance to add a memory reference
	if features.ShouldRecall() {
		summary, _, _ := b.store.GetSummary(userID)
		profile, _ := b.store.GetFactsAsText(userID)
		recall, recallErr := features.GenerateRecall(context.Background(), b.claude, summary, profile)
		if recallErr == nil && recall != "" {
			reply = reply + "\n\n" + recall
		}
	}

	// Daily lie injection: use pre-generated lie or generate on the fly
	today := time.Now().Format("2006-01-02")
	lie, _, lieExists := features.GetTodayLie(b.store, userID)
	if lieExists {
		// Lie exists (pre-generated or from previous attempt)
		injectedKey := "lie_injected_" + today
		injected, _ := b.store.GetCounter(userID, injectedKey)
		if injected == 0 {
			reply = features.InjectLie(reply, lie)
			b.store.SetCounter(userID, injectedKey, 1)
			log.Printf("[%d] daily lie injected (pre-generated)", userID)
		}
	} else if features.ShouldLieToday(b.store, userID) {
		// No pre-generated lie, generate now (fallback)
		newLie, newTruth, genErr := features.GenerateLie(context.Background(), b.claude)
		if genErr == nil {
			b.store.SaveLie(userID, newLie, newTruth, today)
			reply = features.InjectLie(reply, newLie)
			injectedKey := "lie_injected_" + today
			b.store.SetCounter(userID, injectedKey, 1)
			log.Printf("[%d] daily lie injected (generated)", userID)
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

	// Track streak
	b.checkStreak(c)

	// Check if summary needs update (async, don't block response)
	go b.maybeUpdateSummary(userID)

	// 5% chance: send voice reply instead of text (if TTS available and reply is short)
	voiceSent := false
	if b.tts != nil && len(reply) < 200 && rand.Intn(100) < 5 {
		// First-time TTS warning
		warned, _ := b.store.GetCounter(userID, "tts_warned")
		if warned == 0 {
			_ = c.Send(models.MsgTTSWarning, menu, tele.ModeMarkdown)
			b.store.SetCounter(userID, "tts_warned", 1)
		}

		path, ttsErr := b.tts.Synthesize(context.Background(), reply)
		if ttsErr == nil {
			// Delete thinking message via replyFn with the text, then also send voice
			_ = replyFn(reply, menu)
			audio := &tele.Voice{File: tele.FromDisk(path)}
			if sendErr := c.Send(audio); sendErr != nil {
				log.Printf("[%d] tts voice send error: %v", userID, sendErr)
			} else {
				voiceSent = true
			}
			os.Remove(path)
		}
	}

	// Send text reply (if voice was not sent)
	if !voiceSent {
		if sendErr := replyFn(reply, menu); sendErr != nil {
			return sendErr
		}
	}

	// Random kaomoji reaction (~8% chance)
	if features.ShouldReact() {
		reaction := features.RandomReaction()
		if err := c.Send(reaction, menu); err != nil {
			log.Printf("[%d] reaction send error: %v", userID, err)
		}
	}

	// Daily reward for first message of the day
	b.checkDailyReward(c)

	// Contextual suggestion (~10% chance, max 1 per day)
	b.maybeSuggestFeature(c)

	// Async sentiment analysis + mood drop detection
	go func() {
		score, sErr := features.AnalyzeSentiment(context.Background(), b.claude, text)
		if sErr != nil {
			return
		}
		if _, saveErr := b.store.SaveMessage(userID, "bot", fmt.Sprintf("[auto_mood:%d]", score)); saveErr != nil {
			log.Printf("[%d] save auto_mood error: %v", userID, saveErr)
		}

		b.checkMoodDrop(userID)
	}()

	return nil
}

// checkDailyReward sends a daily reward for the first message of the day.
func (b *Bot) checkDailyReward(c tele.Context) {
	userID := c.Sender().ID
	today := time.Now().Format("2006-01-02")
	key := "daily_reward_" + today
	claimed, _ := b.store.GetCounter(userID, key)
	if claimed > 0 {
		return
	}

	b.store.SetCounter(userID, key, 1)
	reward := models.DailyRewards[rand.Intn(len(models.DailyRewards))]
	if err := c.Send(reward, menu); err != nil {
		log.Printf("[%d] daily reward send error: %v", userID, err)
	}
}

// maybeSuggestFeature suggests an unused feature (~10% chance, max 1 per day).
func (b *Bot) maybeSuggestFeature(c tele.Context) {
	userID := c.Sender().ID
	today := time.Now().Format("2006-01-02")
	key := "suggestion_" + today
	shown, _ := b.store.GetCounter(userID, key)
	if shown > 0 || rand.Intn(10) != 0 {
		return
	}

	b.store.SetCounter(userID, key, 1)
	if err := c.Send(models.FeatureSuggestions[rand.Intn(len(models.FeatureSuggestions))], menu); err != nil {
		log.Printf("[%d] suggestion send error: %v", userID, err)
	}
}

// checkMoodDrop detects when user mood drops significantly and sends an empathetic message.
func (b *Bot) checkMoodDrop(userID int64) {
	moods, err := b.store.GetRecentAutoMoods(userID, 6)
	if err != nil || len(moods) < 6 {
		return
	}

	// moods[0..2] = most recent 3, moods[3..5] = previous 3
	recentSum := 0
	for _, m := range moods[:3] {
		recentSum += m
	}
	previousSum := 0
	for _, m := range moods[3:6] {
		previousSum += m
	}

	recentAvg := float64(recentSum) / 3.0
	previousAvg := float64(previousSum) / 3.0

	if previousAvg-recentAvg >= 3.0 {
		if _, err := b.tg.Send(&tele.User{ID: userID}, models.MsgMoodDrop); err != nil {
			log.Printf("[%d] mood drop send error: %v", userID, err)
		}
	}
}

func (b *Bot) handleVoiceOut(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /voice", userID)

	if b.tts == nil {
		return c.Send(models.MsgTTSNotInstalled, menu)
	}

	payload := c.Message().Payload
	if payload == "" {
		payload = models.MsgTTSDefault
	}

	_, stop := b.startThinking(c)

	path, err := b.tts.Synthesize(context.Background(), payload)
	if err != nil {
		stop()
		log.Printf("[%d] tts error: %v", userID, err)
		return c.Send(models.MsgTTSFail+features.RandomFallback(), menu)
	}
	defer os.Remove(path)

	stop()

	audio := &tele.Voice{File: tele.FromDisk(path)}
	return c.Send(audio)
}

func (b *Bot) handleVoice(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] voice message", userID)
	defer b.checkAndSendAchievements(c, "voice")

	if b.whisper == nil {
		return c.Send(models.MsgVoiceNotConfigured, menu)
	}

	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	voice := c.Message().Voice
	if voice == nil {
		return nil
	}

	// SEC-015: Limit voice message size to prevent OOM
	const maxVoiceSize = 5 * 1024 * 1024 // 5MB
	if voice.FileSize > maxVoiceSize {
		return c.Send(models.MsgVoiceTooLarge, menu)
	}

	b.incrementRateLimit(userID)
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

	audioData, err := io.ReadAll(io.LimitReader(reader, maxVoiceSize))
	if err != nil {
		stop()
		log.Printf("[%d] voice read error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	text, err := b.whisper.Transcribe(audioData, "voice.ogg")
	if err != nil {
		stop()
		log.Printf("[%d] whisper error: %v", userID, err)
		return c.Send(models.MsgVoiceNotHeard, menu)
	}

	logText := text
	if len(logText) > 50 {
		logText = logText[:50] + "..."
	}
	log.Printf("[%d] transcribed: %s", userID, logText)

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
	msg, err := b.tg.Send(c.Recipient(), models.MsgBreathingStart)
	if err != nil {
		return c.Send(models.MsgBreathingFail+features.RandomFallback(), menu)
	}

	go features.RunBreathing(b.tg, msg, func() {
		b.checkAndSendAchievements(c, "breathing")
	})

	return nil
}

func (b *Bot) handleMeditate(c tele.Context) error {
	log.Printf("[%d] /meditate", c.Sender().ID)

	msg, err := b.tg.Send(c.Recipient(), models.MsgMeditateStart)
	if err != nil {
		return c.Send(models.MsgMeditateFail+features.RandomFallback(), menu)
	}

	go features.RunMeditation(b.tg, msg, nil)

	return nil
}

func (b *Bot) handleCapsule(c tele.Context) error {
	userID := c.Sender().ID
	text := c.Message().Payload
	log.Printf("[%d] capsule: %s", userID, text)

	if text == "" {
		return c.Send(models.MsgCapsuleFormat, menu)
	}

	// Parse: first word is number of days, rest is text
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		return c.Send(models.MsgCapsuleFormatShort, menu)
	}

	days := 0
	if _, err := fmt.Sscanf(parts[0], "%d", &days); err != nil || days < 1 || days > 365 {
		return c.Send(models.MsgCapsuleDaysRange, menu)
	}

	deliverAt := time.Now().AddDate(0, 0, days)
	if err := b.store.SaveCapsule(userID, parts[1], deliverAt); err != nil {
		log.Printf("[%d] save capsule error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	b.checkAndSendAchievements(c, "capsule")

	return c.Send(fmt.Sprintf(models.FmtCapsuleSaved, days), menu)
}

var moodReplies = models.MoodReplies

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

	return c.Edit(fmt.Sprintf(models.FmtMoodScore, score, reply), tele.ModeMarkdown)
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
		return c.Send(models.MsgTruthHonest, menu)
	}

	if revealed {
		return c.Send(fmt.Sprintf(models.FmtTruthRevealed, lie, truth), menu)
	}

	if revealErr := b.store.RevealLie(userID, today); revealErr != nil {
		log.Printf("[%d] reveal lie error: %v", userID, revealErr)
	}

	return c.Send(fmt.Sprintf(models.FmtTruthReveal, lie, truth), menu)
}

func (b *Bot) handleProfile(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /profile", userID)

	facts, err := b.store.GetFacts(userID)
	if err != nil {
		log.Printf("[%d] get facts error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	inline := &tele.ReplyMarkup{}
	btn := inline.Data(models.BtnEditProfileLabel, "edit_profile")
	inline.Inline(inline.Row(btn))

	return c.Send(features.FormatProfile(facts), menu, tele.ModeMarkdown, inline)
}

func (b *Bot) handleEditProfile(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] edit_profile callback", userID)
	b.store.SetCounter(userID, "profile_editing", 1)
	return c.Send(models.MsgProfileEditPrompt, menu)
}

func (b *Bot) handleMoodGraph(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /mood", userID)

	entries, err := b.store.GetMoodHistory(userID, 7)
	if err != nil {
		log.Printf("[%d] mood history error: %v", userID, err)
		return c.Send(models.MsgMoodError, menu)
	}

	if len(entries) == 0 {
		return c.Send(models.MsgMoodNoData, menu)
	}

	graph := buildMoodASCII(entries)

	// Append trend analysis
	if len(entries) >= 2 {
		avg := 0.0
		for _, e := range entries {
			avg += float64(e.Score)
		}
		avg /= float64(len(entries))

		best, worst := entries[0], entries[0]
		for _, e := range entries {
			if e.Score > best.Score {
				best = e
			}
			if e.Score < worst.Score {
				worst = e
			}
		}

		days := []string{"Вс", "Пн", "Вт", "Ср", "Чт", "Пт", "Сб"}
		graph += fmt.Sprintf("\n\nСреднее: %.1f | Лучший: %s (%d) | Худший: %s (%d)",
			avg, days[best.CreatedAt.Weekday()], best.Score, days[worst.CreatedAt.Weekday()], worst.Score)
	}

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
	defer b.checkAndSendAchievements(c, "roast")

	userCtx := b.buildUserContext(userID)

	// Enrich roast with recent user quotes for personalization
	msgs, _ := b.store.GetLastMessages(userID, 20)
	var recentQuotes []string
	for _, m := range msgs {
		if m.Role == "user" && len(m.Text) > 10 {
			recentQuotes = append(recentQuotes, m.Text)
		}
	}
	quotesCtx := ""
	if len(recentQuotes) > 0 {
		picked := recentQuotes
		if len(picked) > 3 {
			// Pick 3 random quotes
			rand.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
			picked = picked[:3]
		}
		quotesCtx = "\n\nНедавние цитаты пользователя (используй для roast):\n"
		for _, q := range picked {
			quotesCtx += "- \"" + q + "\"\n"
		}
	}

	prompt := fmt.Sprintf(features.RoastPrompt, userCtx) + quotesCtx

	return b.claudeReplyWithRepeat(c, func() (string, error) {
		return b.claude.AskWithModel(context.Background(), b.smartModel, prompt, "Зароасти меня")
	}, "", "repeat_roast")
}

func (b *Bot) handleWisdom(c tele.Context) error {
	log.Printf("[%d] /wisdom", c.Sender().ID)
	defer b.checkAndSendAchievements(c, "wisdom")

	return b.claudeReplyWithRepeat(c, func() (string, error) {
		return b.claude.Ask(context.Background(), features.WisdomPrompt, "Дай мудрость")
	}, "\U0001F9D9 ", "repeat_wisdom")
}

func (b *Bot) handleHoroscope(c tele.Context) error {
	log.Printf("[%d] /horoscope", c.Sender().ID)
	defer b.checkAndSendAchievements(c, "horoscope")

	today := time.Now().Format("2 January 2006")
	prompt := fmt.Sprintf(features.AntiHoroscopePrompt, today)

	return b.claudeReplyWithRepeat(c, func() (string, error) {
		return b.claude.Ask(context.Background(), prompt, "Антигороскоп на сегодня")
	}, "\u2B50 ", "repeat_horoscope")
}

func (b *Bot) handlePlaylist(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /playlist", userID)

	userCtx := b.buildUserContext(userID)
	prompt := fmt.Sprintf(features.PlaylistPrompt, userCtx)

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), prompt, "Подбери плейлист")
	}, "")
}

func (b *Bot) handleFuture(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /future", userID)

	userCtx := b.buildUserContext(userID)
	prompt := fmt.Sprintf(features.FutureLetterPrompt, userCtx)

	return b.claudeReply(c, func() (string, error) {
		return b.claude.AskWithModel(context.Background(), b.smartModel, prompt, "Напиши письмо из будущего")
	}, models.MsgFuturePrefix)
}

func (b *Bot) handleAnon(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send(models.MsgAnonGroupOnly)
	}

	text := c.Message().Payload
	if text == "" {
		return c.Send(models.MsgAnonFormat)
	}

	// Length limit
	if len(text) > 500 {
		text = text[:500] + "..."
	}

	// Rate limit: 1 per 5 minutes per user per chat
	anonKey := fmt.Sprintf("anon_%d_%d", c.Chat().ID, c.Sender().ID)
	lastAnon, _ := b.store.GetCounter(c.Sender().ID, anonKey)
	if lastAnon > 0 && time.Now().Unix()-int64(lastAnon) < 300 {
		return c.Send(models.MsgAnonCooldown)
	}
	b.store.SetCounter(c.Sender().ID, anonKey, int(time.Now().Unix()))

	// Delete original message
	if err := b.tg.Delete(c.Message()); err != nil {
		log.Printf("delete anon source: %v", err)
	}

	// 10% chance bot "accidentally" reveals author
	prefix := models.MsgAnonPrefix
	if rand.Intn(10) == 0 {
		prefix = fmt.Sprintf(models.FmtAnonRevealed, c.Sender().FirstName)
	}

	return c.Send(prefix + text)
}

func (b *Bot) buildUserContext(userID int64) string {
	summary, _, err := b.store.GetSummary(userID)
	if err != nil {
		log.Printf("[%d] get summary error: %v", userID, err)
	}

	msgs, err := b.store.GetLastMessages(userID, 50)
	if err != nil {
		log.Printf("[%d] get messages error: %v", userID, err)
	}

	// Use only last 5 messages for recent conversation context
	recentMsgs := msgs
	if len(recentMsgs) > 5 {
		recentMsgs = recentMsgs[:5]
	}

	profile, err := b.store.GetFactsAsText(userID)
	if err != nil {
		log.Printf("[%d] get profile error: %v", userID, err)
	}

	ctx := features.BuildContext(summary, recentMsgs)
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

	// Analyze user's frequent phrases from wider message history
	if len(msgs) > 10 {
		phrases := features.AnalyzeFrequentPhrases(msgs)
		ctx += features.PhrasesPromptSuffix(phrases)
	}

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

// --- Remind ---

var remindRe = regexp.MustCompile(`^(\d+)([mhd])\s+(.+)$`)

func (b *Bot) handleRemind(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /remind: %s", userID, payload)

	if payload == "" {
		return c.Send(models.MsgRemindFormat, menu)
	}

	matches := remindRe.FindStringSubmatch(payload)
	if matches == nil {
		return c.Send(models.MsgRemindFormat, menu)
	}

	amount, _ := strconv.Atoi(matches[1])
	unit := matches[2]
	text := matches[3]

	var dur time.Duration
	var humanTime string
	switch unit {
	case "m":
		dur = time.Duration(amount) * time.Minute
		humanTime = fmt.Sprintf("%d мин.", amount)
	case "h":
		dur = time.Duration(amount) * time.Hour
		humanTime = fmt.Sprintf("%d ч.", amount)
	case "d":
		dur = time.Duration(amount) * 24 * time.Hour
		humanTime = fmt.Sprintf("%d дн.", amount)
	}

	if dur < time.Minute {
		return c.Send(models.MsgRemindMinTime, menu)
	}

	remindAt := time.Now().Add(dur)
	if err := b.store.SaveReminder(userID, text, remindAt); err != nil {
		log.Printf("[%d] save reminder error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send(fmt.Sprintf(models.FmtRemindSaved, humanTime), menu)
}

// --- Streak ---

func dateToInt(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

func (b *Bot) checkStreak(c tele.Context) {
	userID := c.Sender().ID
	today := dateToInt(time.Now())

	lastDate, err := b.store.GetCounter(userID, "last_streak_date")
	if err != nil {
		// No counter yet — first message ever
		lastDate = 0
	}

	if today == lastDate {
		// Already counted today
		return
	}

	streak, _ := b.store.GetCounter(userID, "streak_days")
	record, _ := b.store.GetCounter(userID, "streak_record")

	if today == lastDate+1 {
		// Consecutive day
		streak++
	} else {
		// Gap — reset
		streak = 1
	}

	if err := b.store.SetCounter(userID, "streak_days", streak); err != nil {
		log.Printf("[%d] set streak_days error: %v", userID, err)
	}
	if err := b.store.SetCounter(userID, "last_streak_date", today); err != nil {
		log.Printf("[%d] set last_streak_date error: %v", userID, err)
	}

	if streak > record {
		if err := b.store.SetCounter(userID, "streak_record", streak); err != nil {
			log.Printf("[%d] set streak_record error: %v", userID, err)
		}
	}

	// Check for streak reward milestones
	if reward := features.GetStreakReward(streak); reward != nil {
		milestone := fmt.Sprintf("%s %s", reward.Emoji, reward.Message)
		if err := c.Send(milestone, menu); err != nil {
			log.Printf("[%d] streak milestone send error: %v", userID, err)
		}
	}

	// Streak achievements
	if streak == 7 {
		b.checkAndSendAchievements(c, "streak_7")
	}
	if streak == 30 {
		b.checkAndSendAchievements(c, "streak_30")
	}
}

func (b *Bot) handleStreak(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /streak", userID)

	streak, _ := b.store.GetCounter(userID, "streak_days")
	record, _ := b.store.GetCounter(userID, "streak_record")

	if streak == 0 {
		return c.Send(models.MsgStreakNone, menu)
	}

	return c.Send(fmt.Sprintf(models.FmtStreak, streak, record), menu)
}

// --- Habit Tracker ---

func (b *Bot) handleHabit(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /habit: %s", userID, payload)

	today := time.Now().Format("2006-01-02")

	// Default: list
	if payload == "" || payload == "list" {
		return b.habitList(c, userID, today)
	}

	if strings.HasPrefix(payload, "add ") {
		name := strings.TrimPrefix(payload, "add ")
		name = strings.TrimSpace(name)
		if name == "" {
			return c.Send(models.MsgHabitAddFormat, menu)
		}
		_, err := b.store.AddHabit(userID, name)
		if err != nil {
			log.Printf("[%d] add habit error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}
		return c.Send(models.MsgHabitAdded, menu)
	}

	if strings.HasPrefix(payload, "done ") {
		return b.habitDone(c, userID, payload, today)
	}

	if strings.HasPrefix(payload, "delete ") {
		return b.habitDelete(c, userID, payload)
	}

	return c.Send(models.MsgHabitUnknown, menu)
}

func (b *Bot) habitList(c tele.Context, userID int64, today string) error {
	habits, err := b.store.GetHabits(userID)
	if err != nil {
		log.Printf("[%d] get habits error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if len(habits) == 0 {
		return c.Send(models.MsgHabitNoHabits, menu)
	}

	var sb strings.Builder
	sb.WriteString(models.MsgHabitListHeader)

	for i, h := range habits {
		done, _ := b.store.GetHabitLog(h.ID, today)
		streak, _ := b.store.GetHabitStreak(h.ID)

		status := "❌"
		if done {
			status = "✅"
		}

		streakStr := ""
		if streak > 0 {
			streakStr = fmt.Sprintf(" (streak: %d)", streak)
		}

		sb.WriteString(fmt.Sprintf("%d. %s %s%s\n", i+1, status, h.Name, streakStr))
	}

	sb.WriteString(models.MsgHabitListFooter)
	return c.Send(sb.String(), menu)
}

func (b *Bot) habitDone(c tele.Context, userID int64, payload, today string) error {
	numStr := strings.TrimPrefix(payload, "done ")
	num, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil || num < 1 {
		return c.Send(models.MsgHabitDoneFormat, menu)
	}

	habits, err := b.store.GetHabits(userID)
	if err != nil || num > len(habits) {
		return c.Send(models.MsgHabitNotFound, menu)
	}

	habit := habits[num-1]
	already, _ := b.store.GetHabitLog(habit.ID, today)
	if already {
		return c.Send(models.MsgHabitAlreadyDone, menu)
	}

	if err := b.store.LogHabit(habit.ID, today); err != nil {
		log.Printf("[%d] log habit error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	streak, _ := b.store.GetHabitStreak(habit.ID)

	reply := models.HabitDoneReplies[rand.Intn(len(models.HabitDoneReplies))]
	if streak > 1 {
		reply += fmt.Sprintf(models.FmtHabitStreak, streak)
	}

	return c.Send(fmt.Sprintf("✅ %s — %s", habit.Name, reply), menu)
}

func (b *Bot) habitDelete(c tele.Context, userID int64, payload string) error {
	numStr := strings.TrimPrefix(payload, "delete ")
	num, err := strconv.Atoi(strings.TrimSpace(numStr))
	if err != nil || num < 1 {
		return c.Send(models.MsgHabitDeleteFormat, menu)
	}

	habits, err := b.store.GetHabits(userID)
	if err != nil || num > len(habits) {
		return c.Send(models.MsgHabitNotFound, menu)
	}

	habit := habits[num-1]
	if err := b.store.DeleteHabit(habit.ID); err != nil {
		log.Printf("[%d] delete habit error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send(fmt.Sprintf(models.FmtHabitDeleted, habit.Name), menu)
}

// --- Habit inline button callbacks ---

func (b *Bot) handleHabitDoneCallback(c tele.Context, idx int) error {
	userID := c.Sender().ID
	log.Printf("[%d] habit_done callback: %d", userID, idx)

	today := time.Now().Format("2006-01-02")
	habits, err := b.store.GetHabits(userID)
	if err != nil || idx > len(habits) || idx < 1 {
		return c.Respond(&tele.CallbackResponse{Text: models.MsgHabitCBNotFound})
	}

	habit := habits[idx-1]
	already, _ := b.store.GetHabitLog(habit.ID, today)
	if already {
		return c.Respond(&tele.CallbackResponse{Text: models.MsgHabitCBAlready})
	}

	if err := b.store.LogHabit(habit.ID, today); err != nil {
		log.Printf("[%d] habit callback log error: %v", userID, err)
		return c.Respond(&tele.CallbackResponse{Text: models.MsgHabitCBError})
	}

	streak, _ := b.store.GetHabitStreak(habit.ID)
	streakStr := ""
	if streak > 1 {
		streakStr = fmt.Sprintf(models.FmtHabitDoneInline, streak)
	}

	return c.Edit(fmt.Sprintf(models.FmtHabitDoneCallback, habit.Name, streakStr))
}

func (b *Bot) handleHabitSkipCallback(c tele.Context, idx int) error {
	userID := c.Sender().ID
	log.Printf("[%d] habit_skip callback: %d", userID, idx)

	habits, err := b.store.GetHabits(userID)
	if err != nil || idx > len(habits) || idx < 1 {
		return c.Respond(&tele.CallbackResponse{Text: models.MsgHabitCBNotFound})
	}

	habit := habits[idx-1]
	return c.Edit(fmt.Sprintf(models.FmtHabitSkipCallback, habit.Name))
}

// --- Journal ---

// journalAutoTags detects keywords in text and returns comma-separated tags.
func journalAutoTags(text string) string {
	lower := strings.ToLower(text)
	tagMap := map[string][]string{
		"work":  {"работ", "задач", "проект", "дедлайн", "митинг", "код", "баг", "релиз", "work"},
		"sport": {"зал", "трен", "бег", "спорт", "йог", "фитнес", "sport", "gym"},
		"mood":  {"настроен", "грустн", "радост", "тревог", "злост", "счастлив", "mood"},
		"food":  {"еда", "ед", "завтрак", "обед", "ужин", "готов", "food"},
		"sleep": {"сон", "спал", "спать", "бессонниц", "выспал", "sleep"},
	}

	var tags []string
	seen := make(map[string]bool)
	for tag, keywords := range tagMap {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) && !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
				break
			}
		}
	}
	return strings.Join(tags, ",")
}

func (b *Bot) handleJournal(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /journal: %s", userID, payload)

	if !b.checkRateLimit(userID) {
		return c.Send(models.MsgRateLimit, b.replyOpts(c))
	}

	// Subcommand: search
	if strings.HasPrefix(payload, "search ") {
		query := strings.TrimPrefix(payload, "search ")
		query = strings.TrimSpace(query)
		if query == "" {
			return c.Send(models.MsgJournalFormat, b.replyOpts(c))
		}
		return b.journalSearch(c, userID, query)
	}

	// Subcommand: stats
	if payload == "stats" {
		return b.journalStats(c, userID)
	}

	// No args: show last 5 entries
	if payload == "" {
		return b.journalList(c, userID)
	}

	// Save new entry
	tags := journalAutoTags(payload)
	if err := b.store.SaveJournalEntry(userID, payload, tags); err != nil {
		log.Printf("[%d] save journal error: %v", userID, err)
		return c.Send(features.RandomFallback(), b.replyOpts(c))
	}

	return c.Send(models.MsgJournalSaved, b.replyOpts(c))
}

func (b *Bot) journalList(c tele.Context, userID int64) error {
	entries, err := b.store.GetJournalEntries(userID, 5)
	if err != nil {
		log.Printf("[%d] get journal entries error: %v", userID, err)
		return c.Send(features.RandomFallback(), b.replyOpts(c))
	}
	if len(entries) == 0 {
		return c.Send(models.MsgJournalEmpty, b.replyOpts(c))
	}

	var sb strings.Builder
	sb.WriteString(models.MsgJournalHeader)
	for _, e := range entries {
		date := e.CreatedAt.Format("02.01 15:04")
		tags := e.Tags
		if tags == "" {
			tags = "-"
		}
		sb.WriteString(fmt.Sprintf(models.FmtJournalEntry, date, tags, e.Text))
	}
	return c.Send(sb.String(), b.replyOpts(c))
}

func (b *Bot) journalSearch(c tele.Context, userID int64, query string) error {
	entries, err := b.store.SearchJournal(userID, query)
	if err != nil {
		log.Printf("[%d] search journal error: %v", userID, err)
		return c.Send(features.RandomFallback(), b.replyOpts(c))
	}
	if len(entries) == 0 {
		return c.Send(models.MsgJournalSearchEmpty, b.replyOpts(c))
	}

	var sb strings.Builder
	sb.WriteString(models.MsgJournalHeader)
	for _, e := range entries {
		date := e.CreatedAt.Format("02.01 15:04")
		tags := e.Tags
		if tags == "" {
			tags = "-"
		}
		sb.WriteString(fmt.Sprintf(models.FmtJournalEntry, date, tags, e.Text))
	}
	return c.Send(sb.String(), b.replyOpts(c))
}

func (b *Bot) journalStats(c tele.Context, userID int64) error {
	total, topTags, err := b.store.GetJournalStats(userID)
	if err != nil {
		log.Printf("[%d] journal stats error: %v", userID, err)
		return c.Send(features.RandomFallback(), b.replyOpts(c))
	}
	if total == 0 {
		return c.Send(models.MsgJournalEmpty, b.replyOpts(c))
	}

	weekCount, err := b.store.GetJournalEntriesThisWeek(userID)
	if err != nil {
		log.Printf("[%d] journal week count error: %v", userID, err)
		weekCount = 0
	}

	tagsStr := "-"
	if len(topTags) > 0 {
		tagsStr = strings.Join(topTags, ", ")
	}

	return c.Send(fmt.Sprintf(models.FmtJournalStats, total, weekCount, tagsStr), b.replyOpts(c))
}

// --- Sleep Tracker ---

func (b *Bot) handleSleep(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /sleep", userID)

	days := features.AnalyzeSleep(b.store, userID, 7)
	report := features.FormatSleepReport(days)

	return c.Send(report, menu)
}

// --- Group chat ---

// groupInterject sends a random unsolicited comment on a group message.
func (b *Bot) groupInterject(c tele.Context, text string) {
	chatID := c.Chat().ID

	ctx := b.buildGroupContext(c)
	prompt := fmt.Sprintf(models.FmtGroupInterjectPrompt, text)

	systemPrompt := features.TricksterSystemPrompt + features.TimeOfDayMood() + features.DayOfWeekMood()
	if ctx != "" {
		systemPrompt = systemPrompt + "\n\n" + ctx
	}

	reply, err := b.claude.Ask(context.Background(), systemPrompt, prompt)
	if err != nil {
		log.Printf("[%d] group interject error: %v", chatID, err)
		return
	}

	if _, err := b.store.SaveMessage(chatID, "bot", reply); err != nil {
		log.Printf("[%d] save group interject error: %v", chatID, err)
	}

	// No reply keyboard in groups
	if err := c.Send(reply); err != nil {
		log.Printf("[%d] group interject send error: %v", chatID, err)
	}
}

// buildGroupContext builds conversation context for group chats using chatID.
func (b *Bot) buildGroupContext(c tele.Context) string {
	chatID := c.Chat().ID
	msgs, err := b.store.GetLastMessages(chatID, 10)
	if err != nil {
		log.Printf("[%d] get group messages error: %v", chatID, err)
		return ""
	}
	return features.BuildContext("", msgs)
}

// handleTricksterIntro introduces the bot in a group chat.
func (b *Bot) handleTricksterIntro(c tele.Context) error {
	if !isGroupChat(c) {
		return c.Send(models.MsgTricksterGroupOnly, menu)
	}
	intro := "Йо, народ! Я Трикстер — дерзкий друг-подъёбщик.\n\n" +
		"Упоминайте меня @" + b.tg.Me.Username + " или отвечайте на мои сообщения.\n" +
		"Иногда я буду вставлять свои 5 копеек сам.\n\n" +
		"Команды работают и тут: /help\n\n" +
		"Чтобы я мог подслушивать и вставлять комментарии — отключите Group Privacy в @BotFather."
	return c.Send(intro)
}

// --- Leaderboard ---

func (b *Bot) handleTop(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /top", userID)

	users, err := b.store.GetAllUsers()
	if err != nil {
		log.Printf("[%d] get all users error: %v", userID, err)
		return c.Send(features.RandomFallback(), b.replyOpts(c))
	}

	if len(users) == 0 {
		return c.Send(models.MsgTopEmpty, b.replyOpts(c))
	}

	// In group chat, filter to only group members
	if isGroupChat(c) {
		chatID := c.Chat().ID
		groupUserIDs, _ := b.store.GetGroupUserIDs(chatID)
		if len(groupUserIDs) > 0 {
			memberSet := make(map[int64]bool, len(groupUserIDs))
			for _, uid := range groupUserIDs {
				memberSet[uid] = true
			}
			var filtered []storage.UserInfo
			for _, u := range users {
				if memberSet[u.UserID] {
					filtered = append(filtered, u)
				}
			}
			if len(filtered) > 0 {
				users = filtered
			}
		}
	}

	// Sort by message count descending
	sortedUsers := make([]storage.UserInfo, len(users))
	copy(sortedUsers, users)
	for i := 0; i < len(sortedUsers); i++ {
		for j := i + 1; j < len(sortedUsers); j++ {
			if sortedUsers[j].MessageCount > sortedUsers[i].MessageCount {
				sortedUsers[i], sortedUsers[j] = sortedUsers[j], sortedUsers[i]
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(models.MsgTopHeader)

	limit := 10
	if len(sortedUsers) < limit {
		limit = len(sortedUsers)
	}

	for i := 0; i < limit; i++ {
		u := sortedUsers[i]
		name := u.FirstName
		if name == "" {
			name = u.Username
		}
		if name == "" {
			name = fmt.Sprintf("User %d", u.UserID)
		}

		level := features.GetLevel(u.MessageCount)

		prefix := ""
		if i == 0 {
			prefix = "👑 "
		}

		sb.WriteString(fmt.Sprintf("%d. %s%s — %d сообщ. (%s)\n", i+1, prefix, name, u.MessageCount, level.Name))
	}

	// Find current user's rank
	userRank := 0
	for i, u := range sortedUsers {
		if u.UserID == userID {
			userRank = i + 1
			break
		}
	}

	if userRank > 0 {
		sb.WriteString(fmt.Sprintf(models.FmtTopSelf, userRank, len(sortedUsers)))
	}

	return c.Send(sb.String(), b.replyOpts(c))
}

// --- Secret commands (streak rewards) ---

func (b *Bot) handleRoastMe(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /roastme", userID)

	streak, _ := b.store.GetCounter(userID, "streak_days")
	if streak < 14 {
		return c.Send(models.FmtRoastMeLocked+strconv.Itoa(streak), b.replyOpts(c))
	}

	return b.claudeReply(c, func() (string, error) {
		return b.claude.Ask(context.Background(), models.RoastMeSelfPrompt, "Зароасть себя")
	}, "🤡 ")
}

func (b *Bot) handleSerious(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /serious", userID)

	streak, _ := b.store.GetCounter(userID, "streak_days")
	if streak < 30 {
		return c.Send(models.FmtSeriousLocked+strconv.Itoa(streak), b.replyOpts(c))
	}

	// Check if already used today
	today := time.Now().Format("2006-01-02")
	key := "serious_" + today
	used, _ := b.store.GetCounter(userID, key)
	if used > 0 {
		return c.Send(models.MsgSeriousUsedToday, b.replyOpts(c))
	}

	// Mark as used
	if err := b.store.SetCounter(userID, key, 1); err != nil {
		log.Printf("[%d] set serious counter error: %v", userID, err)
	}

	payload := c.Message().Payload
	if payload == "" {
		payload = models.MsgSeriousDefault
	}

	return b.claudeReply(c, func() (string, error) {
		return b.claude.AskWithModel(context.Background(), b.smartModel,
			models.SeriousPrompt,
			payload)
	}, "🎩 ")
}

// --- Evening reflection handlers ---

func (b *Bot) handleReflectGood(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] reflect_good", userID)

	if err := b.store.SetCounter(userID, "reflection_waiting", 1); err != nil {
		log.Printf("[%d] set reflection_waiting error: %v", userID, err)
	}

	return c.Edit(models.MsgReflectGoodPrompt)
}

func (b *Bot) handleReflectBad(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] reflect_bad", userID)

	if err := b.store.SetCounter(userID, "reflection_waiting", 2); err != nil {
		log.Printf("[%d] set reflection_waiting error: %v", userID, err)
	}

	return c.Edit(models.MsgReflectBadPrompt)
}

func (b *Bot) handleReflectTomorrow(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] reflect_tomorrow", userID)

	if err := b.store.SetCounter(userID, "reflection_waiting", 3); err != nil {
		log.Printf("[%d] set reflection_waiting error: %v", userID, err)
	}

	return c.Edit(models.MsgReflectTomorrowPrompt)
}

// --- Weekly Challenge ---

func (b *Bot) handleChallenge(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /challenge: %s", userID, payload)

	if strings.TrimSpace(payload) == "done" {
		return b.challengeDone(c, userID)
	}

	challenge, weekStart, completedDays, err := b.store.GetCurrentChallenge(userID)
	if err != nil {
		log.Printf("[%d] get challenge error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if challenge == "" {
		return c.Send(models.MsgChallengeNone, menu)
	}

	progress := strings.Repeat("✅", completedDays) + strings.Repeat("⬜", 7-completedDays)
	msg := fmt.Sprintf(models.FmtChallengeStatus, weekStart, challenge, progress, completedDays)
	return c.Send(msg, menu)
}

func (b *Bot) challengeDone(c tele.Context, userID int64) error {
	challenge, weekStart, completedDays, err := b.store.GetCurrentChallenge(userID)
	if err != nil {
		log.Printf("[%d] get challenge error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if challenge == "" {
		return c.Send(models.MsgChallengeNoActive, menu)
	}

	if completedDays >= 7 {
		return c.Send(models.MsgChallengeDone, menu)
	}

	if err := b.store.IncrementChallengeDays(userID, weekStart); err != nil {
		log.Printf("[%d] increment challenge error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	completedDays++
	progress := strings.Repeat("✅", completedDays) + strings.Repeat("⬜", 7-completedDays)

	if completedDays == 7 {
		return c.Send(fmt.Sprintf(models.FmtChallengeComplete, challenge, progress), menu)
	}

	reply := models.ChallengeDoneReplies[rand.Intn(len(models.ChallengeDoneReplies))]
	return c.Send(fmt.Sprintf("✅ %s\n\n%s (%d/7)", reply, progress, completedDays), menu)
}

// --- Interactive Storytelling ---

func (b *Bot) handleStory(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /story", userID)

	// Reset story state
	if err := b.store.SetCounter(userID, "story_active", 1); err != nil {
		log.Printf("[%d] set story_active error: %v", userID, err)
	}
	if err := b.store.SetCounter(userID, "story_step", 0); err != nil {
		log.Printf("[%d] set story_step error: %v", userID, err)
	}

	replyFn, stop := b.startThinking(c)
	opening, err := b.claude.AskWithModel(context.Background(), b.smartModel, features.StoryStartPrompt, "Начни историю")
	if err != nil {
		stop()
		log.Printf("[%d] story start error: %v", userID, err)
		if sErr := b.store.SetCounter(userID, "story_active", 0); sErr != nil {
			log.Printf("[%d] reset story_active error: %v", userID, sErr)
		}
		return c.Send(features.RandomFallback(), menu)
	}

	// Save story message for context
	if _, err := b.store.SaveMessage(userID, "bot", "[story] "+opening); err != nil {
		log.Printf("[%d] save story msg error: %v", userID, err)
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(
		inline.Data("1\uFE0F\u20E3", "story_1"),
		inline.Data("2\uFE0F\u20E3", "story_2"),
	))

	return replyFn(models.MsgStoryPrefix+opening, inline)
}

func (b *Bot) handleStoryContinue(c tele.Context, choice string) error {
	userID := c.Sender().ID
	log.Printf("[%d] story continue: choice=%s", userID, choice)

	step, _ := b.store.GetCounter(userID, "story_step")
	step++

	if err := b.store.SetCounter(userID, "story_step", step); err != nil {
		log.Printf("[%d] set story_step error: %v", userID, err)
	}

	// Build context from recent [story] messages
	msgs, err := b.store.GetLastMessages(userID, 20)
	if err != nil {
		log.Printf("[%d] get story context error: %v", userID, err)
	}

	var storyCtx strings.Builder
	for _, m := range msgs {
		if strings.HasPrefix(m.Text, "[story] ") {
			storyCtx.WriteString(strings.TrimPrefix(m.Text, "[story] "))
			storyCtx.WriteString("\n")
		}
	}

	choiceText := "Вариант " + choice
	prompt := fmt.Sprintf(features.StoryContinuePrompt, storyCtx.String(), choiceText)

	replyFn, stop := b.startThinking(c)
	continuation, err := b.claude.AskWithModel(context.Background(), b.smartModel, prompt, "Продолжи историю, выбор: "+choice)
	if err != nil {
		stop()
		log.Printf("[%d] story continue error: %v", userID, err)
		if sErr := b.store.SetCounter(userID, "story_active", 0); sErr != nil {
			log.Printf("[%d] reset story_active error: %v", userID, sErr)
		}
		return c.Send(features.RandomFallback(), menu)
	}

	// Save story message
	if _, err := b.store.SaveMessage(userID, "bot", "[story] "+continuation); err != nil {
		log.Printf("[%d] save story msg error: %v", userID, err)
	}

	// If step >= 4, end the story
	if step >= 4 {
		if sErr := b.store.SetCounter(userID, "story_active", 0); sErr != nil {
			log.Printf("[%d] reset story_active error: %v", userID, sErr)
		}
		return replyFn(fmt.Sprintf(models.MsgStoryEnd, continuation), menu)
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(
		inline.Data("1\uFE0F\u20E3", "story_1"),
		inline.Data("2\uFE0F\u20E3", "story_2"),
	))

	return replyFn(models.MsgStoryPrefix+continuation, inline)
}

func (b *Bot) handleStoryCallback(c tele.Context, choice string) error {
	userID := c.Sender().ID
	log.Printf("[%d] story callback: choice=%s", userID, choice)

	storyActive, _ := b.store.GetCounter(userID, "story_active")
	if storyActive == 0 {
		return c.Respond(&tele.CallbackResponse{Text: models.MsgStoryNoActive})
	}

	// Save user choice as message
	if _, err := b.store.SaveMessage(userID, "user", "[story] Выбор: "+choice); err != nil {
		log.Printf("[%d] save story choice error: %v", userID, err)
	}

	return b.handleStoryContinue(c, choice)
}

func (b *Bot) handleWeather(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /weather", userID)

	// Parse city from command args or fall back to profile
	city := strings.TrimSpace(c.Message().Payload)
	if city == "" {
		// Try to get city from user profile
		profileCity, err := b.store.GetFact(userID, "city")
		if err != nil {
			log.Printf("[%d] get city fact error: %v", userID, err)
		}
		city = profileCity
	}

	if city == "" {
		return c.Send(models.MsgWeatherNoCity, b.replyOpts(c))
	}

	weather, err := integrations.GetWeather(city)
	if err != nil {
		log.Printf("[%d] weather error: %v", userID, err)
		return c.Send(models.MsgWeatherError, b.replyOpts(c))
	}

	msg := fmt.Sprintf(models.FmtWeather, city, weather)
	return c.Send(msg, b.replyOpts(c))
}

func (b *Bot) handleRates(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /rates", userID)

	rates, err := integrations.GetRates()
	if err != nil {
		log.Printf("[%d] rates error: %v", userID, err)
		return c.Send(models.MsgRatesError, b.replyOpts(c))
	}

	// Generate a trickster comment about money
	comment, cErr := b.claude.Ask(context.Background(),
		"Ты трикстер. Скажи одно короткое ироничное предложение про деньги/курсы. На русском. Без контекста, просто фразу.",
		"Комментарий к курсам валют")
	if cErr != nil {
		log.Printf("[%d] rates comment error: %v", userID, cErr)
		comment = ""
	}

	msg := models.MsgRatesHeader + rates
	if comment != "" {
		msg += "\n\n" + comment
	}

	return c.Send(msg, b.replyOpts(c))
}

// --- Notes ---

func (b *Bot) handleNote(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /note: %s", userID, payload)

	if payload == "" {
		return c.Send(models.MsgNoteFormat, menu)
	}

	// /note clear — delete all notes
	if payload == "clear" {
		if err := b.store.DeleteAllNotes(userID); err != nil {
			log.Printf("[%d] delete all notes error: %v", userID, err)
			return c.Send(features.RandomFallback(), menu)
		}
		return c.Send(models.MsgNoteAllDeleted, menu)
	}

	// /note delete N — delete note by number
	if strings.HasPrefix(payload, "delete ") {
		numStr := strings.TrimPrefix(payload, "delete ")
		num, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil || num < 1 {
			return c.Send(models.MsgNoteFormat, menu)
		}

		notes, err := b.store.GetNotes(userID)
		if err != nil || num > len(notes) {
			return c.Send(models.MsgNoteNotFound, menu)
		}

		note := notes[num-1]
		if err := b.store.DeleteNote(note.ID, userID); err != nil {
			log.Printf("[%d] delete note error: %v", userID, err)
			return c.Send(models.MsgNoteNotFound, menu)
		}
		return c.Send(models.MsgNoteDeleted, menu)
	}

	// /note текст — save a new note
	if err := b.store.SaveNote(userID, payload); err != nil {
		log.Printf("[%d] save note error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}
	return c.Send(models.MsgNoteSaved, menu)
}

func (b *Bot) handleNotes(c tele.Context) error {
	userID := c.Sender().ID
	log.Printf("[%d] /notes", userID)

	notes, err := b.store.GetNotes(userID)
	if err != nil {
		log.Printf("[%d] get notes error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	if len(notes) == 0 {
		return c.Send(models.MsgNoteEmpty, menu)
	}

	var sb strings.Builder
	sb.WriteString(models.MsgNotesHeader)
	for i, n := range notes {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, n.Text))
	}
	return c.Send(sb.String(), menu)
}

// --- Pomodoro ---

func (b *Bot) handlePomodoro(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /pomo: %s", userID, payload)

	// /pomo stop — cancel active timer
	if payload == "stop" {
		active, _ := b.store.GetCounter(userID, "pomo_active")
		if active == 0 || time.Now().Unix() > int64(active) {
			return c.Send(models.MsgPomoNoActive, menu)
		}
		if err := b.store.SetCounter(userID, "pomo_active", 0); err != nil {
			log.Printf("[%d] reset pomo error: %v", userID, err)
		}
		return c.Send(models.MsgPomoStopped, menu)
	}

	// Check if already active
	active, _ := b.store.GetCounter(userID, "pomo_active")
	if active > 0 && time.Now().Unix() < int64(active) {
		return c.Send(models.MsgPomoActive, menu)
	}

	// Parse duration: default 25, or custom N
	duration := 25
	if payload != "" {
		d, err := strconv.Atoi(strings.TrimSpace(payload))
		if err != nil || d < 5 || d > 120 {
			return c.Send(models.MsgPomoRange, menu)
		}
		duration = d
	}

	// Store end time as unix timestamp (work + break = duration + 5 min)
	endTime := time.Now().Add(time.Duration(duration+5) * time.Minute)
	if err := b.store.SetCounter(userID, "pomo_active", int(endTime.Unix())); err != nil {
		log.Printf("[%d] set pomo_active error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	// Send start message
	if err := c.Send(fmt.Sprintf(models.FmtPomoStart, duration), menu); err != nil {
		return err
	}

	// Launch timer goroutine
	go b.runPomodoro(userID, duration)

	return nil
}

func (b *Bot) runPomodoro(userID int64, duration int) {
	recipient := &tele.User{ID: userID}

	// Wait for work period
	time.Sleep(time.Duration(duration) * time.Minute)

	// Check if still active (user may have cancelled)
	active, _ := b.store.GetCounter(userID, "pomo_active")
	if active == 0 {
		return
	}

	// Send break notification
	if _, err := b.tg.Send(recipient, models.MsgPomoBreak); err != nil {
		log.Printf("[%d] pomo break send error: %v", userID, err)
		return
	}

	// Wait for break period (5 min)
	time.Sleep(5 * time.Minute)

	// Clear active flag
	if err := b.store.SetCounter(userID, "pomo_active", 0); err != nil {
		log.Printf("[%d] reset pomo_active error: %v", userID, err)
	}

	// Send end notification
	if _, err := b.tg.Send(recipient, models.MsgPomoEnd); err != nil {
		log.Printf("[%d] pomo end send error: %v", userID, err)
	}
}

// --- Password Generator ---

func (b *Bot) handlePassword(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /pass: %s", userID, payload)

	length := 16
	if payload != "" {
		n, err := strconv.Atoi(strings.TrimSpace(payload))
		if err == nil && n >= 4 && n <= 128 {
			length = n
		}
	}

	pass, err := generatePassword(length)
	if err != nil {
		log.Printf("[%d] generate password error: %v", userID, err)
		return c.Send(features.RandomFallback(), menu)
	}

	return c.Send(fmt.Sprintf(models.FmtPassResult, pass), menu, tele.ModeMarkdown)
}

// generatePassword creates a random password using crypto/rand.
func generatePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
	result := make([]byte, length)
	for i := range result {
		idx, err := cryptoRandInt(len(charset))
		if err != nil {
			return "", fmt.Errorf("crypto rand: %w", err)
		}
		result[i] = charset[idx]
	}
	return string(result), nil
}

// cryptoRandInt returns a cryptographically random int in [0, max).
func cryptoRandInt(max int) (int, error) {
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

// --- Calculator ---

func (b *Bot) handleCalc(c tele.Context) error {
	userID := c.Sender().ID
	payload := c.Message().Payload
	log.Printf("[%d] /calc: %s", userID, payload)

	if payload == "" {
		return c.Send(models.MsgCalcFormat, menu)
	}

	result, err := calcExpression(payload)
	if err != nil {
		return c.Send(models.MsgCalcError, menu)
	}

	// Format result: remove trailing zeros for float
	var resultStr string
	if result == float64(int64(result)) {
		resultStr = strconv.FormatInt(int64(result), 10)
	} else {
		resultStr = strconv.FormatFloat(result, 'f', -1, 64)
	}

	return c.Send(resultStr, menu)
}

// calcExpression parses and evaluates "number operator number" expressions.
func calcExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)

	parts := calcRe.FindStringSubmatch(expr)
	if parts == nil {
		return 0, fmt.Errorf("invalid expression")
	}

	a, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse first number: %w", err)
	}

	op := parts[2]

	bVal, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return 0, fmt.Errorf("parse second number: %w", err)
	}

	switch op {
	case "+":
		return a + bVal, nil
	case "-":
		return a - bVal, nil
	case "*":
		return a * bVal, nil
	case "/":
		if bVal == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return a / bVal, nil
	default:
		return 0, fmt.Errorf("unknown operator: %s", op)
	}
}

var calcRe = regexp.MustCompile(`^(-?\d+(?:\.\d+)?)\s*([+\-*/])\s*(-?\d+(?:\.\d+)?)$`)
