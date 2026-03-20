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
	"github.com/Arkosh744/chaos-bro-bot/internal/integrations"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
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

	s.sendMorningBriefing(s.cfg.OwnerID)

	recipient := &chatRecipient{id: s.cfg.OwnerID}

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
				msg := fmt.Sprintf(models.FmtChallengeNew, challengeText)
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
			reminder := fmt.Sprintf(models.FmtChallengeReminder, challenge, progress, completedDays)
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

// sendMorningBriefing sends a combined morning message: mood buttons, weather, rates,
// day fact, and a Claude-generated greeting with daily quest and motivation.
func (s *Scheduler) sendMorningBriefing(userID int64) {
	recipient := &chatRecipient{id: userID}

	// Step 1: Mood check with inline buttons
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

	// Step 2: Gather external data concurrently
	var profileCtx string
	var weatherLine string
	var ratesLine string
	var factLine string
	var city string

	if s.store != nil {
		profile, err := s.store.GetFactsAsText(userID)
		if err != nil {
			log.Printf("[%d] morning briefing get profile: %v", userID, err)
		}
		if profile != "" {
			profileCtx = profile
		}

		cityFact, err := s.store.GetFact(userID, "city")
		if err != nil {
			log.Printf("[%d] morning briefing get city: %v", userID, err)
		}
		city = cityFact
	}
	if profileCtx == "" {
		profileCtx = models.MsgProfileNotFilled
	}

	// Fetch weather if city is known
	if city != "" {
		weather, err := integrations.GetWeather(city)
		if err != nil {
			log.Printf("[%d] morning briefing weather error: %v", userID, err)
		} else {
			weatherLine = fmt.Sprintf(models.FmtWeather, city, weather)
		}
	}

	// Fetch currency rates
	rates, err := integrations.GetRates()
	if err != nil {
		log.Printf("[%d] morning briefing rates error: %v", userID, err)
	} else {
		ratesLine = models.MsgRatesHeader + rates
	}

	// Fetch day fact via Claude
	fact, err := integrations.GetDayFact(s.claude, time.Now())
	if err != nil {
		log.Printf("[%d] morning briefing day fact error: %v", userID, err)
	} else {
		factLine = fact
	}

	// Step 3: Generate briefing via Claude with all context
	weatherCtx := weatherLine
	if weatherCtx == "" {
		weatherCtx = "нет данных"
	}
	ratesCtx := rates
	if ratesCtx == "" {
		ratesCtx = "нет данных"
	}
	factCtx := factLine
	if factCtx == "" {
		factCtx = "нет данных"
	}

	prompt := fmt.Sprintf(models.MorningBriefingPrompt, profileCtx, weatherCtx, ratesCtx, factCtx)
	briefing, err := s.claude.Ask(context.Background(), prompt, "Утренний брифинг")
	if err != nil {
		log.Printf("[%d] morning briefing generate: %v", userID, err)
	}

	// Step 4: Build and send the combined message
	var msg string
	if briefing != "" {
		msg = briefing
	}

	// Append weather line
	if weatherLine != "" {
		msg += "\n\n" + weatherLine
	}

	// Append compact rates
	if ratesLine != "" {
		msg += "\n" + ratesLine
	}

	// Append day fact
	if factLine != "" {
		msg += "\n\n" + factLine
	}

	// Append mood question at the end
	msg += "\n\nКак настроение? От 1 до 10:"

	if msg != "" {
		if _, err := s.tg.Send(recipient, msg, inline); err != nil {
			log.Printf("[%d] morning briefing send: %v", userID, err)
		}
	}
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
			nameA = models.MsgLinkedDefaultName
		}
		if nameB == "" {
			nameB = models.MsgLinkedDefaultName
		}

		// Both have mood data today — compare and notify
		var msg string
		diff := moodA - moodB
		if diff < 0 {
			diff = -diff
		}

		switch {
		case moodA == moodB:
			msg = fmt.Sprintf(models.FmtLinkedSameMood, nameB, moodA)
		case diff <= 2:
			msg = fmt.Sprintf(models.FmtLinkedCloseMood, moodA, nameB, moodB)
		default:
			msg = fmt.Sprintf(models.FmtLinkedDiffMood, moodA, nameB, moodB)
		}

		recipientA := &chatRecipient{id: link.UserA}
		if _, err := s.tg.Send(recipientA, msg); err != nil {
			log.Printf("[%d] linked mood notify error: %v", link.UserA, err)
		}

		// Send reverse notification to user B
		var msgB string
		switch {
		case moodA == moodB:
			msgB = fmt.Sprintf(models.FmtLinkedSameMood, nameA, moodB)
		case diff <= 2:
			msgB = fmt.Sprintf(models.FmtLinkedCloseMood, moodB, nameA, moodA)
		default:
			msgB = fmt.Sprintf(models.FmtLinkedDiffMood, moodB, nameA, moodA)
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

	// Smart evening reflection: only send if user had 3+ messages today AND average mood < 6
	if s.store != nil {
		userMsgCount, err := s.store.GetUserMessageCountTodayByRole(s.cfg.OwnerID, "user")
		if err != nil {
			log.Printf("[%d] evening check get today msg count: %v", s.cfg.OwnerID, err)
		}
		if userMsgCount < 3 {
			log.Printf("[%d] evening check skipped: only %d user messages today (need 3+)", s.cfg.OwnerID, userMsgCount)
			return
		}

		moods, err := s.store.GetRecentAutoMoods(s.cfg.OwnerID, 5)
		if err != nil {
			log.Printf("[%d] evening check get auto moods: %v", s.cfg.OwnerID, err)
		}
		if len(moods) > 0 {
			sum := 0
			for _, m := range moods {
				sum += m
			}
			avg := float64(sum) / float64(len(moods))
			if avg >= 6.0 {
				log.Printf("[%d] evening check skipped: avg mood %.1f >= 6 (user seems fine)", s.cfg.OwnerID, avg)
				return
			}
			log.Printf("[%d] evening check triggered: avg mood %.1f < 6, %d msgs today", s.cfg.OwnerID, avg, userMsgCount)
		}
	}

	inline := &tele.ReplyMarkup{}
	inline.Inline(inline.Row(
		inline.Data(models.BtnReflectGoodLabel, "reflect_good"),
		inline.Data(models.BtnReflectBadLabel, "reflect_bad"),
		inline.Data(models.BtnReflectTmrwLabel, "reflect_tomorrow"),
	))

	recipient := &chatRecipient{id: s.cfg.OwnerID}
	if _, err := s.tg.Send(recipient, models.MsgEveningCheck, inline); err != nil {
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
		msg := fmt.Sprintf(models.FmtCapsuleDelivered, cap.Text)
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
		msg := fmt.Sprintf(models.FmtRemindDeliver, r.Text)
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
			btnDone := inline.Data(models.BtnHabitDoneLabel, fmt.Sprintf("habit_done_%d", idx))
			btnSkip := inline.Data(models.BtnHabitSkipLabel, fmt.Sprintf("habit_skip_%d", idx))
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
			msg := fmt.Sprintf(models.FmtNightOwlWarning, currentHour)
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
	if _, err := s.tg.Send(recipient, models.MsgDigestPrefix+digest); err != nil {
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

// --- Proactive messages ---

// milestoneThresholds defines the round message counts that trigger a milestone notification.
var milestoneThresholds = []int{100, 200, 500, 1000, 2000, 5000}

// proactiveLoop runs hourly checks for streak warnings, milestone delivery, and weekend summaries.
func (s *Scheduler) proactiveLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			if !s.IsEnabled() || s.store == nil {
				continue
			}
			s.checkStreakWarnings()
			s.checkMilestoneDelivery()
			s.checkWeekendSummary()
		}
	}
}

// dateToInt converts a time to YYYYMMDD integer (same as bot/handlers.go dateToInt).
func dateToInt(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

// checkStreakWarnings warns users whose streak is about to expire.
// Triggers if: streak > 3, last_streak_date == yesterday, current hour >= 20.
func (s *Scheduler) checkStreakWarnings() {
	now := time.Now()
	if now.Hour() < 20 {
		return
	}

	yesterday := dateToInt(now.AddDate(0, 0, -1))
	todayStr := now.Format("20060102")

	users, err := s.store.GetAllUsers()
	if err != nil {
		log.Printf("streak warning get users: %v", err)
		return
	}

	for _, u := range users {
		// Skip negative user IDs (group chats)
		if u.UserID < 0 {
			continue
		}

		streakDays, err := s.store.GetCounter(u.UserID, "streak_days")
		if err != nil || streakDays <= 3 {
			continue
		}

		lastDate, err := s.store.GetCounter(u.UserID, "last_streak_date")
		if err != nil || lastDate != yesterday {
			continue
		}

		// Rate limit: max 1 streak warning per day per user
		warnKey := fmt.Sprintf("streak_warned_%s", todayStr)
		alreadyWarned, _ := s.store.GetCounter(u.UserID, warnKey)
		if alreadyWarned > 0 {
			continue
		}

		msg := fmt.Sprintf(models.FmtStreakWarning, streakDays)
		recipient := &chatRecipient{id: u.UserID}
		if _, sendErr := s.tg.Send(recipient, msg); sendErr != nil {
			log.Printf("[%d] streak warning send: %v", u.UserID, sendErr)
			continue
		}

		if err := s.store.SetCounter(u.UserID, warnKey, 1); err != nil {
			log.Printf("[%d] streak warning set counter: %v", u.UserID, err)
		}

		if _, err := s.store.SaveMessage(u.UserID, "bot", msg); err != nil {
			log.Printf("[%d] streak warning save msg: %v", u.UserID, err)
		}

		log.Printf("[%d] streak warning sent: %d day streak expiring", u.UserID, streakDays)
	}
}

// checkMilestoneDelivery checks for pending milestone notifications and delivers them after a delay.
func (s *Scheduler) checkMilestoneDelivery() {
	now := time.Now()
	todayStr := now.Format("20060102")

	users, err := s.store.GetAllUsers()
	if err != nil {
		log.Printf("milestone check get users: %v", err)
		return
	}

	for _, u := range users {
		if u.UserID < 0 {
			continue
		}

		// Rate limit: max 1 milestone per day per user
		milestoneKey := fmt.Sprintf("milestone_sent_%s", todayStr)
		alreadySent, _ := s.store.GetCounter(u.UserID, milestoneKey)
		if alreadySent > 0 {
			continue
		}

		totalCount, err := s.store.GetMessageCount(u.UserID)
		if err != nil {
			continue
		}

		for _, threshold := range milestoneThresholds {
			pendingKey := fmt.Sprintf("milestone_pending_%d", threshold)
			pendingTS, _ := s.store.GetCounter(u.UserID, pendingKey)

			if pendingTS > 0 {
				// Check if 2-6 hours have passed since the milestone was recorded
				elapsed := now.Unix() - int64(pendingTS)
				minDelay := int64(2 * 3600) // 2 hours
				maxDelay := int64(6 * 3600) // 6 hours
				randomDelay := minDelay + int64(rand.Intn(int(maxDelay-minDelay)))
				if elapsed < randomDelay {
					continue
				}

				// Generate personalized comment via Claude
				var msg string
				comment, clErr := s.claude.Ask(
					context.Background(),
					fmt.Sprintf(models.MilestoneCommentPrompt, threshold),
					fmt.Sprintf("Пользователь написал %d сообщений", threshold),
				)
				if clErr != nil {
					log.Printf("[%d] milestone comment generate: %v", u.UserID, clErr)
					msg = fmt.Sprintf(models.FmtMilestone, threshold)
				} else {
					msg = fmt.Sprintf(models.FmtMilestone, threshold) + " " + comment
				}

				recipient := &chatRecipient{id: u.UserID}
				if _, sendErr := s.tg.Send(recipient, msg); sendErr != nil {
					log.Printf("[%d] milestone send: %v", u.UserID, sendErr)
					continue
				}

				// Clear pending and mark as sent
				if err := s.store.SetCounter(u.UserID, pendingKey, 0); err != nil {
					log.Printf("[%d] milestone clear pending: %v", u.UserID, err)
				}
				if err := s.store.SetCounter(u.UserID, milestoneKey, 1); err != nil {
					log.Printf("[%d] milestone set sent: %v", u.UserID, err)
				}

				if _, err := s.store.SaveMessage(u.UserID, "bot", msg); err != nil {
					log.Printf("[%d] milestone save msg: %v", u.UserID, err)
				}

				log.Printf("[%d] milestone delivered: %d messages", u.UserID, threshold)
				break // One milestone at a time
			}

			// Check if user just crossed a threshold (current count >= threshold, no pending set)
			if totalCount >= threshold {
				// Check if already delivered for this threshold
				deliveredKey := fmt.Sprintf("milestone_delivered_%d", threshold)
				delivered, _ := s.store.GetCounter(u.UserID, deliveredKey)
				if delivered > 0 {
					continue
				}

				// Set pending timestamp for delayed delivery
				if err := s.store.SetCounter(u.UserID, pendingKey, int(now.Unix())); err != nil {
					log.Printf("[%d] milestone set pending: %v", u.UserID, err)
				}
				if err := s.store.SetCounter(u.UserID, deliveredKey, 1); err != nil {
					log.Printf("[%d] milestone set delivered: %v", u.UserID, err)
				}

				log.Printf("[%d] milestone pending set for %d messages", u.UserID, threshold)
				break
			}
		}
	}
}

// checkWeekendSummary sends a fun weekly mini-summary on Saturday at 12:00.
func (s *Scheduler) checkWeekendSummary() {
	now := time.Now()
	if now.Weekday() != time.Saturday || now.Hour() != 12 {
		return
	}

	todayStr := now.Format("20060102")

	users, err := s.store.GetAllUsers()
	if err != nil {
		log.Printf("weekend summary get users: %v", err)
		return
	}

	weekAgo := now.AddDate(0, 0, -7)

	for _, u := range users {
		if u.UserID < 0 {
			continue
		}

		// Rate limit: max 1 weekend summary per day per user
		summaryKey := fmt.Sprintf("weekend_summary_%s", todayStr)
		alreadySent, _ := s.store.GetCounter(u.UserID, summaryKey)
		if alreadySent > 0 {
			continue
		}

		// Check if user was active this week (5+ messages)
		weeklyMsgCount, err := s.store.GetUserMessageCountSinceDateByRole(u.UserID, weekAgo, "user")
		if err != nil {
			log.Printf("[%d] weekend summary get week count: %v", u.UserID, err)
			continue
		}
		if weeklyMsgCount < 5 {
			continue
		}

		// Count mood categories from auto_mood data
		moods, err := s.store.GetRecentAutoMoods(u.UserID, 50)
		if err != nil {
			log.Printf("[%d] weekend summary get moods: %v", u.UserID, err)
		}

		complaints := 0 // mood < 5
		joys := 0       // mood >= 7
		for _, m := range moods {
			if m < 5 {
				complaints++
			}
			if m >= 7 {
				joys++
			}
		}

		// Generate comment via Claude
		comment, clErr := s.claude.Ask(
			context.Background(),
			fmt.Sprintf(models.WeekendSummaryPrompt, weeklyMsgCount, complaints, joys),
			fmt.Sprintf("Сообщений: %d, жалоб: %d, радости: %d", weeklyMsgCount, complaints, joys),
		)
		if clErr != nil {
			log.Printf("[%d] weekend summary generate: %v", u.UserID, clErr)
			comment = fmt.Sprintf("Жаловался %d раз, радовался %d раз.", complaints, joys)
		}

		msg := fmt.Sprintf(models.FmtWeekendSummary, weeklyMsgCount, comment)
		recipient := &chatRecipient{id: u.UserID}
		if _, sendErr := s.tg.Send(recipient, msg); sendErr != nil {
			log.Printf("[%d] weekend summary send: %v", u.UserID, sendErr)
			continue
		}

		if err := s.store.SetCounter(u.UserID, summaryKey, 1); err != nil {
			log.Printf("[%d] weekend summary set counter: %v", u.UserID, err)
		}

		if _, err := s.store.SaveMessage(u.UserID, "bot", msg); err != nil {
			log.Printf("[%d] weekend summary save msg: %v", u.UserID, err)
		}

		log.Printf("[%d] weekend summary sent: %d msgs, %d complaints, %d joys", u.UserID, weeklyMsgCount, complaints, joys)
	}
}
