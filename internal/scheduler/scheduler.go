package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	tele "gopkg.in/telebot.v4"
)

type Config struct {
	Enabled bool
	MinHour int
	MaxHour int
	OwnerID int64
}

type Scheduler struct {
	cfg          Config
	tg           *tele.Bot
	claude       *claude.Client
	store        *storage.Storage
	stop         chan struct{}
	recentQuotes []string
	mu           sync.Mutex
}

func New(cfg Config, tg *tele.Bot, cl *claude.Client, store *storage.Storage) *Scheduler {
	return &Scheduler{
		cfg:    cfg,
		tg:     tg,
		claude: cl,
		store:  store,
		stop:   make(chan struct{}),
	}
}

// SetEnabled enables or disables the scheduler at runtime.
func (s *Scheduler) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Enabled = enabled
	log.Printf("Scheduler enabled=%v", enabled)
}

// IsEnabled returns whether the scheduler is currently enabled.
func (s *Scheduler) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Enabled
}

// SetHours updates the allowed ping hours at runtime.
func (s *Scheduler) SetHours(min, max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.MinHour = min
	s.cfg.MaxHour = max
	log.Printf("Scheduler hours updated: %d:00-%d:00", min, max)
}

// GetConfig returns a copy of the current scheduler configuration.
func (s *Scheduler) GetConfig() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Scheduler) Start() {
	// Capsule and reminder delivery runs always — user-created items must be delivered regardless of scheduler state
	go s.capsuleLoop()
	go s.reminderLoop()
	go s.backupLoop()

	if !s.cfg.Enabled || s.cfg.OwnerID == 0 {
		log.Println("Scheduler disabled (capsule delivery still active)")
		return
	}
	log.Printf("Scheduler started: pings between %d:00-%d:00 for user %d", s.cfg.MinHour, s.cfg.MaxHour, s.cfg.OwnerID)
	go s.loop()
	go s.morningCheckLoop()
	go s.digestLoop()
	go s.habitReminderLoop()
	go s.sleepWarningLoop()
	go s.eveningCheckLoop()
}

func (s *Scheduler) Stop() {
	close(s.stop)
}

// SendPingNow triggers an immediate ping to the specified user.
func (s *Scheduler) SendPingNow(userID int64) {
	s.sendPingTo(userID)
}

func (s *Scheduler) loop() {
	for {
		delay := s.randomDelay()
		log.Printf("Next ping in %s", delay.Round(time.Minute))
		timer := time.NewTimer(delay)

		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() {
				continue
			}
			s.sendPing()
		}
	}
}

// randomDelay returns 2-6 hours, adjusted to stay within the allowed window.
func (s *Scheduler) randomDelay() time.Duration {
	minMinutes := 120 // 2 hours
	maxMinutes := 360 // 6 hours
	minutes := minMinutes + rand.Intn(maxMinutes-minMinutes)
	delay := time.Duration(minutes) * time.Minute

	next := time.Now().Add(delay)

	// If next ping lands outside window, push to next day's min hour
	if next.Hour() >= s.cfg.MaxHour || next.Hour() < s.cfg.MinHour {
		tomorrow := time.Now().AddDate(0, 0, 1)
		next = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(),
			s.cfg.MinHour, rand.Intn(60), 0, 0, tomorrow.Location())
		delay = time.Until(next)
	}

	if delay < time.Minute {
		delay = time.Minute
	}
	return delay
}

func (s *Scheduler) sendPing() {
	s.sendPingTo(s.cfg.OwnerID)
}

func (s *Scheduler) sendPingTo(userID int64) {
	var msg string

	// 40% quote, 30% grounding, 30% trickster with context
	roll := rand.Intn(100)
	switch {
	case roll < 40:
		msg = s.generateQuotePing()
	case roll < 70:
		msg = "🌍 " + features.RandomGrounding()
	default:
		msg = s.generateTricksterPingFor(userID)
	}

	recipient := &chatRecipient{id: userID}
	if _, err := s.tg.Send(recipient, msg); err != nil {
		log.Printf("scheduler send to %d: %v", userID, err)
	} else {
		log.Printf("scheduler ping sent to %d: %.50s...", userID, msg)
	}

	// Save bot message to storage
	if s.store != nil {
		if _, err := s.store.SaveMessage(userID, "bot", msg); err != nil {
			log.Printf("scheduler save message: %v", err)
		}
	}
}

func (s *Scheduler) generateQuotePing() string {
	s.mu.Lock()
	recent := make([]string, len(s.recentQuotes))
	copy(recent, s.recentQuotes)
	s.mu.Unlock()

	quote, err := features.GenerateQuote(context.Background(), s.claude, recent)
	if err != nil {
		log.Printf("scheduler quote error: %v", err)
		return "🎮 " + features.RandomFallback()
	}

	s.mu.Lock()
	s.recentQuotes = append(s.recentQuotes, quote)
	if len(s.recentQuotes) > 10 {
		s.recentQuotes = s.recentQuotes[len(s.recentQuotes)-10:]
	}
	s.mu.Unlock()

	return "🎮 " + quote
}

func (s *Scheduler) generateTricksterPing() string {
	return s.generateTricksterPingFor(s.cfg.OwnerID)
}

func (s *Scheduler) generateTricksterPingFor(userID int64) string {
	// Build context from storage
	var userCtx string
	if s.store != nil {
		summary, _, err := s.store.GetSummary(userID)
		if err != nil {
			log.Printf("scheduler get summary: %v", err)
		}
		msgs, err := s.store.GetLastMessages(userID, 5)
		if err != nil {
			log.Printf("scheduler get messages: %v", err)
		}
		userCtx = features.BuildContext(summary, msgs)
	}

	systemPrompt := features.TricksterSystemPrompt
	if userCtx != "" {
		systemPrompt = systemPrompt + "\n\n" + userCtx
	}

	reply, err := s.claude.Ask(context.Background(), systemPrompt,
		"Напиши пользователю что-нибудь. Вы давно не общались. Просто так, без повода. Можешь спросить как дела или вспомнить что-то из прошлых разговоров.")
	if err != nil {
		log.Printf("scheduler trickster error: %v", err)
		return features.RandomFallback()
	}
	return reply
}

type chatRecipient struct {
	id int64
}

func (r *chatRecipient) Recipient() string {
	return fmt.Sprintf("%d", r.id)
}

func (s *Scheduler) capsuleLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.deliverCapsules()
		}
	}
}

func (s *Scheduler) morningCheckLoop() {
	for {
		now := time.Now()
		// Next check-in: tomorrow between 9:00-9:59
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 9, rand.Intn(60), 0, 0, now.Location())
		if now.Hour() < 9 {
			// If before 9am today, schedule for today
			next = time.Date(now.Year(), now.Month(), now.Day(), 9, rand.Intn(60), 0, 0, now.Location())
		}

		log.Printf("Next morning check-in at %s", next.Format("2006-01-02 15:04"))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() {
				continue
			}
			s.sendMorningCheck()
		}
	}
}

func (s *Scheduler) sendMorningCheck() {
	if s.cfg.OwnerID == 0 {
		return
	}

	// Message 1: Mood check with inline buttons
	inline := &tele.ReplyMarkup{}
	rows := []tele.Row{
		inline.Row(
			inline.Data("1", "mood_1"), inline.Data("2", "mood_2"),
			inline.Data("3", "mood_3"), inline.Data("4", "mood_4"),
			inline.Data("5", "mood_5"),
		),
		inline.Row(
			inline.Data("6", "mood_6"), inline.Data("7", "mood_7"),
			inline.Data("8", "mood_8"), inline.Data("9", "mood_9"),
			inline.Data("10", "mood_10"),
		),
	}
	inline.Inline(rows...)

	recipient := &chatRecipient{id: s.cfg.OwnerID}
	if _, err := s.tg.Send(recipient, "Утро. Как ты от 1 до 10?", inline); err != nil {
		log.Printf("morning check send: %v", err)
	}

	// Message 2: Morning ritual package (after 5s delay)
	time.Sleep(5 * time.Second)

	var profileCtx string
	if s.store != nil {
		profile, err := s.store.GetFactsAsText(s.cfg.OwnerID)
		if err != nil {
			log.Printf("[%d] morning ritual get profile: %v", s.cfg.OwnerID, err)
		}
		if profile != "" {
			profileCtx = profile
		}
	}
	if profileCtx == "" {
		profileCtx = "Профиль пока не заполнен."
	}

	prompt := fmt.Sprintf(features.MorningRitualPrompt, profileCtx)
	ritual, err := s.claude.Ask(context.Background(), prompt, "Утренний пакет")
	if err != nil {
		log.Printf("[%d] morning ritual generate: %v", s.cfg.OwnerID, err)
	} else if ritual != "" {
		if _, err := s.tg.Send(recipient, ritual); err != nil {
			log.Printf("[%d] morning ritual send: %v", s.cfg.OwnerID, err)
		}
	}

	// Weekly challenge: generate on Monday, remind on other days
	now := time.Now()
	if now.Weekday() == time.Monday {
		// Monday: generate new weekly challenge
		challengeText, chErr := s.claude.Ask(context.Background(), features.WeeklyChallengePrompt, "Придумай недельный челлендж")
		if chErr != nil {
			log.Printf("[%d] weekly challenge generate error: %v", s.cfg.OwnerID, chErr)
		} else {
			offset := int(now.Weekday()) - int(time.Monday)
			if offset < 0 {
				offset += 7
			}
			monday := now.AddDate(0, 0, -offset)
			weekStart := monday.Format("2006-01-02")
			if err := s.store.SaveWeeklyChallenge(s.cfg.OwnerID, challengeText, weekStart); err != nil {
				log.Printf("[%d] save weekly challenge error: %v", s.cfg.OwnerID, err)
			} else {
				msg := fmt.Sprintf("🏋️ Челлендж недели:\n\n%s\n\n/challenge — прогресс\n/challenge done — отметить день", challengeText)
				if _, err := s.tg.Send(recipient, msg); err != nil {
					log.Printf("[%d] send weekly challenge error: %v", s.cfg.OwnerID, err)
				}
				log.Printf("[%d] weekly challenge generated", s.cfg.OwnerID)
			}
		}
	} else {
		// Other days: remind about active challenge if exists
		challenge, _, completedDays, chErr := s.store.GetCurrentChallenge(s.cfg.OwnerID)
		if chErr == nil && challenge != "" && completedDays < 7 {
			progress := ""
			for i := 0; i < completedDays; i++ {
				progress += "✅"
			}
			for i := 0; i < 7-completedDays; i++ {
				progress += "⬜"
			}
			reminder := fmt.Sprintf("Не забудь про челлендж: %s\n%s (%d/7) /challenge done", challenge, progress, completedDays)
			if _, err := s.tg.Send(recipient, reminder); err != nil {
				log.Printf("[%d] send challenge reminder error: %v", s.cfg.OwnerID, err)
			}
		}
	}

	// Pre-generate daily lie so it doesn't slow down handleText
	lie, truth, lErr := features.GenerateLie(context.Background(), s.claude)
	if lErr != nil {
		log.Printf("[%d] pre-generate lie error: %v", s.cfg.OwnerID, lErr)
	} else {
		today := time.Now().Format("2006-01-02")
		if err := s.store.SaveLie(s.cfg.OwnerID, lie, truth, today); err != nil {
			log.Printf("[%d] save pre-generated lie error: %v", s.cfg.OwnerID, err)
		} else {
			log.Printf("[%d] daily lie pre-generated", s.cfg.OwnerID)
		}
	}

	// Linked users mood comparison
	s.checkLinkedUsersMood()
}

// checkLinkedUsersMood compares mood scores of linked users and notifies them.
func (s *Scheduler) checkLinkedUsersMood() {
	if s.store == nil {
		return
	}

	links, err := s.store.GetAllActiveLinks()
	if err != nil {
		log.Printf("linked users mood check error: %v", err)
		return
	}

	for _, link := range links {
		moodA, errA := s.store.GetLatestMoodScore(link.UserA)
		moodB, errB := s.store.GetLatestMoodScore(link.UserB)

		if errA != nil || errB != nil || moodA == 0 || moodB == 0 {
			continue
		}

		_, nameA, _, _ := s.store.GetUserProfile(link.UserA)
		_, nameB, _, _ := s.store.GetUserProfile(link.UserB)
		if nameA == "" {
			nameA = "Твой связанный"
		}
		if nameB == "" {
			nameB = "Твой связанный"
		}

		// Both have mood data today — compare and notify
		var msg string
		diff := moodA - moodB
		if diff < 0 {
			diff = -diff
		}

		switch {
		case moodA == moodB:
			msg = fmt.Sprintf("Ты и %s оба сегодня на %d/10. Совпадение? Может поговорите?", nameB, moodA)
		case diff <= 2:
			msg = fmt.Sprintf("Ты %d/10, а %s — %d/10. Почти на одной волне.", moodA, nameB, moodB)
		case moodA < moodB:
			msg = fmt.Sprintf("У тебя %d/10, а у %s — %d/10. Может стоит списаться?", moodA, nameB, moodB)
		default:
			msg = fmt.Sprintf("У тебя %d/10, а у %s — %d/10. Может стоит списаться?", moodA, nameB, moodB)
		}

		recipientA := &chatRecipient{id: link.UserA}
		if _, err := s.tg.Send(recipientA, msg); err != nil {
			log.Printf("[%d] linked mood notify error: %v", link.UserA, err)
		}

		// Send reverse notification to user B
		var msgB string
		switch {
		case moodA == moodB:
			msgB = fmt.Sprintf("Ты и %s оба сегодня на %d/10. Совпадение? Может поговорите?", nameA, moodB)
		case diff <= 2:
			msgB = fmt.Sprintf("Ты %d/10, а %s — %d/10. Почти на одной волне.", moodB, nameA, moodA)
		default:
			msgB = fmt.Sprintf("У тебя %d/10, а у %s — %d/10. Может стоит списаться?", moodB, nameA, moodA)
		}

		recipientB := &chatRecipient{id: link.UserB}
		if _, err := s.tg.Send(recipientB, msgB); err != nil {
			log.Printf("[%d] linked mood notify error: %v", link.UserB, err)
		}

		log.Printf("linked mood comparison: %d(%d) <-> %d(%d)", link.UserA, moodA, link.UserB, moodB)
	}
}

func (s *Scheduler) eveningCheckLoop() {
	for {
		now := time.Now()
		// Next evening check: today or tomorrow at 21:00-21:59
		next := time.Date(now.Year(), now.Month(), now.Day(), 21, rand.Intn(60), 0, 0, now.Location())
		if now.Hour() >= 22 || (now.Hour() == 21 && now.After(next)) {
			// Already past tonight's window, schedule for tomorrow
			next = next.AddDate(0, 0, 1)
		}

		log.Printf("Next evening check at %s", next.Format("2006-01-02 15:04"))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() {
				continue
			}
			s.sendEveningCheck()
		}
	}
}

func (s *Scheduler) sendEveningCheck() {
	if s.cfg.OwnerID == 0 {
		return
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(
		inline.Data("\U0001F60A Что хорошего", "reflect_good"),
		inline.Data("\U0001F624 Что бесило", "reflect_bad"),
		inline.Data("🎯 Что завтра", "reflect_tomorrow"),
	))

	recipient := &chatRecipient{id: s.cfg.OwnerID}
	if _, err := s.tg.Send(recipient, "\U0001F319 Вечерний чек. Выбери:", inline); err != nil {
		log.Printf("evening check send: %v", err)
	}
}

func (s *Scheduler) deliverCapsules() {
	if s.store == nil {
		return
	}

	capsules, err := s.store.GetDueCapsules()
	if err != nil {
		log.Printf("capsule delivery error: %v", err)
		return
	}

	for _, cap := range capsules {
		msg := fmt.Sprintf("⏳ Капсула из прошлого:\n\n%s", cap.Text)
		recipient := &chatRecipient{id: cap.UserID}
		if _, err := s.tg.Send(recipient, msg); err != nil {
			log.Printf("capsule send to %d: %v", cap.UserID, err)
			continue
		}
		if err := s.store.MarkCapsuleDelivered(cap.ID); err != nil {
			log.Printf("capsule mark delivered %d: %v", cap.ID, err)
		}
		log.Printf("capsule delivered to %d: %.50s", cap.UserID, cap.Text)
	}
}

func (s *Scheduler) digestLoop() {
	for {
		now := time.Now()
		// Next Sunday at 20:00 + random minutes
		daysUntilSunday := (7 - int(now.Weekday())) % 7
		if daysUntilSunday == 0 && now.Hour() >= 20 {
			daysUntilSunday = 7
		}
		next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilSunday, 20, rand.Intn(60), 0, 0, now.Location())

		log.Printf("Next weekly digest at %s", next.Format("2006-01-02 15:04"))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() {
				continue
			}
			s.sendDigest()
		}
	}
}

func (s *Scheduler) reminderLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.deliverReminders()
		}
	}
}

func (s *Scheduler) deliverReminders() {
	if s.store == nil {
		return
	}

	reminders, err := s.store.GetDueReminders()
	if err != nil {
		log.Printf("reminder delivery error: %v", err)
		return
	}

	for _, r := range reminders {
		msg := fmt.Sprintf("⏰ Эй! Ты просил напомнить: %s", r.Text)
		recipient := &chatRecipient{id: r.UserID}
		if _, err := s.tg.Send(recipient, msg); err != nil {
			log.Printf("reminder send to %d: %v", r.UserID, err)
			continue
		}
		if err := s.store.MarkReminderDelivered(r.ID); err != nil {
			log.Printf("reminder mark delivered %d: %v", r.ID, err)
		}
		log.Printf("reminder delivered to %d: %.50s", r.UserID, r.Text)
	}
}

// habitReminderLoop sends habit reminders 3 times a day (10:00, 14:00, 19:00).
func (s *Scheduler) habitReminderLoop() {
	reminderHours := []int{10, 14, 19}

	for {
		now := time.Now()
		var nextReminder time.Time

		// Find the next reminder time
		for _, h := range reminderHours {
			candidate := time.Date(now.Year(), now.Month(), now.Day(), h, 0, 0, 0, now.Location())
			if candidate.After(now) {
				nextReminder = candidate
				break
			}
		}
		// If all today's times passed, schedule for tomorrow's first
		if nextReminder.IsZero() {
			tomorrow := now.AddDate(0, 0, 1)
			nextReminder = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), reminderHours[0], 0, 0, 0, now.Location())
		}

		log.Printf("Next habit reminder at %s", nextReminder.Format("2006-01-02 15:04"))
		timer := time.NewTimer(time.Until(nextReminder))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() {
				continue
			}
			s.sendHabitReminders()
		}
	}
}

func (s *Scheduler) sendHabitReminders() {
	if s.store == nil {
		return
	}

	userIDs, err := s.store.GetAllUsersWithHabits()
	if err != nil {
		log.Printf("habit reminder get users: %v", err)
		return
	}

	today := time.Now().Format("2006-01-02")

	for _, userID := range userIDs {
		undone, err := s.store.GetUndoneHabits(userID, today)
		if err != nil {
			log.Printf("habit reminder get undone for %d: %v", userID, err)
			continue
		}
		if len(undone) == 0 {
			continue
		}

		// Build reminder for each undone habit with inline buttons
		habits, _ := s.store.GetHabits(userID)
		for _, uh := range undone {
			// Find the index number for user-facing display
			idx := 0
			for i, h := range habits {
				if h.ID == uh.ID {
					idx = i + 1
					break
				}
			}

			inline := &tele.ReplyMarkup{}
			btnDone := inline.Data("\u2705 Сделал", fmt.Sprintf("habit_done_%d", idx))
			btnSkip := inline.Data("\u274C Нет", fmt.Sprintf("habit_skip_%d", idx))
			inline.Inline(inline.Row(btnDone, btnSkip))

			msg := fmt.Sprintf("Напоминание: %s", uh.Name)
			recipient := &chatRecipient{id: userID}
			if _, err := s.tg.Send(recipient, msg, inline); err != nil {
				log.Printf("habit reminder send to %d: %v", userID, err)
			}
		}
	}
}

// sleepWarningLoop checks weekly at 2:30 AM if user has been up late 3+ times this week.
func (s *Scheduler) sleepWarningLoop() {
	for {
		now := time.Now()
		// Check at 2:30 AM every day
		next := time.Date(now.Year(), now.Month(), now.Day(), 2, 30, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}

		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			if !s.IsEnabled() || s.store == nil {
				continue
			}
			s.checkSleepWarnings()
		}
	}
}

func (s *Scheduler) checkSleepWarnings() {
	userIDs, err := s.store.GetAllUsers()
	if err != nil {
		log.Printf("sleep warning get users: %v", err)
		return
	}

	now := time.Now()
	currentHour := now.Format("15:04")

	for _, u := range userIDs {
		// Check if user sent a message after 2:00 AM today
		lastTime, err := s.store.LastMessageTime(u.UserID)
		if err != nil || lastTime.IsZero() {
			continue
		}

		// Only warn if last message was recent (within last 30 min) and it's after 2 AM
		if time.Since(lastTime) > 30*time.Minute {
			continue
		}

		// Count late nights this week
		lateCount, err := s.store.GetLateNightMessageCount(u.UserID, 7, 2)
		if err != nil {
			log.Printf("sleep warning late count for %d: %v", u.UserID, err)
			continue
		}

		if lateCount >= 3 {
			msg := fmt.Sprintf("Ты опять не спишь в %s. Третий раз за неделю. Ложись.", currentHour)
			recipient := &chatRecipient{id: u.UserID}
			if _, err := s.tg.Send(recipient, msg); err != nil {
				log.Printf("sleep warning send to %d: %v", u.UserID, err)
			}
			log.Printf("sleep warning sent to %d (late %d times this week)", u.UserID, lateCount)
		}
	}
}

func (s *Scheduler) sendDigest() {
	if s.cfg.OwnerID == 0 || s.store == nil {
		return
	}

	digest, err := features.GenerateDigest(context.Background(), s.claude, s.store, s.cfg.OwnerID)
	if err != nil {
		log.Printf("digest error: %v", err)
		return
	}

	recipient := &chatRecipient{id: s.cfg.OwnerID}
	if _, err := s.tg.Send(recipient, "📋 Дайджест недели:\n\n"+digest); err != nil {
		log.Printf("digest send: %v", err)
	}
	log.Printf("weekly digest sent")
}

const maxBackupFiles = 7

// backupLoop creates a daily database backup at 4:00 AM, keeping the last 7 backups.
func (s *Scheduler) backupLoop() {
	if s.store == nil {
		return
	}

	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 4, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}

		log.Printf("Next database backup at %s", next.Format("2006-01-02 15:04"))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-timer.C:
			s.performBackup()
		}
	}
}

func (s *Scheduler) performBackup() {
	dbDir := filepath.Dir(s.store.DBPath())
	backupDir := filepath.Join(dbDir, "data", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		log.Printf("backup: create dir: %v", err)
		return
	}

	filename := fmt.Sprintf("backup_%s.db", time.Now().Format("20060102_150405"))
	destPath := filepath.Join(backupDir, filename)

	if err := s.store.Backup(destPath); err != nil {
		log.Printf("backup: %v", err)
		return
	}
	log.Printf("backup: created %s", destPath)

	// Cleanup old backups, keep only the last maxBackupFiles
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		log.Printf("backup: read dir: %v", err)
		return
	}

	var backups []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".db" {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) <= maxBackupFiles {
		return
	}

	sort.Strings(backups) // lexicographic sort works because filenames contain timestamps
	toDelete := backups[:len(backups)-maxBackupFiles]
	for _, name := range toDelete {
		path := filepath.Join(backupDir, name)
		if err := os.Remove(path); err != nil {
			log.Printf("backup: remove old %s: %v", name, err)
		} else {
			log.Printf("backup: removed old %s", name)
		}
	}
}
