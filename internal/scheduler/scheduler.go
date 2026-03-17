package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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

	if !s.cfg.Enabled || s.cfg.OwnerID == 0 {
		log.Println("Scheduler disabled (capsule delivery still active)")
		return
	}
	log.Printf("Scheduler started: pings between %d:00-%d:00 for user %d", s.cfg.MinHour, s.cfg.MaxHour, s.cfg.OwnerID)
	go s.loop()
	go s.morningCheckLoop()
	go s.digestLoop()
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

	// Daily quest
	quest, err := s.claude.Ask(context.Background(), features.DailyQuestPrompt, "Дай квест на сегодня")
	if err == nil && quest != "" {
		if _, err := s.tg.Send(recipient, "\U0001F4DC Квест дня: "+quest); err != nil {
			log.Printf("daily quest send: %v", err)
		}
	} else if err != nil {
		log.Printf("daily quest generate: %v", err)
	}

	// Pre-generate daily lie so it doesn't slow down handleText
	lie, truth, err := features.GenerateLie(context.Background(), s.claude)
	if err != nil {
		log.Printf("[%d] pre-generate lie error: %v", s.cfg.OwnerID, err)
	} else {
		today := time.Now().Format("2006-01-02")
		if err := s.store.SaveLie(s.cfg.OwnerID, lie, truth, today); err != nil {
			log.Printf("[%d] save pre-generated lie error: %v", s.cfg.OwnerID, err)
		} else {
			log.Printf("[%d] daily lie pre-generated", s.cfg.OwnerID)
		}
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
