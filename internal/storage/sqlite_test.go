package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func TestStorage_SaveAndGet(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	id1, err := db.SaveMessage(123, "user", "hello")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := db.SaveMessage(123, "bot", "yo")
	if err != nil {
		t.Fatal(err)
	}
	if id2 <= id1 {
		t.Errorf("expected id2 > id1")
	}

	msgs, err := db.GetLastMessages(123, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2, got %d", len(msgs))
	}
	if msgs[0].Text != "hello" || msgs[1].Text != "yo" {
		t.Errorf("unexpected order: %v", msgs)
	}
}

func TestStorage_Summary(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	summary, lastID, err := db.GetSummary(123)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "" || lastID != 0 {
		t.Errorf("expected empty summary for new user")
	}

	if err := db.UpdateSummary(123, "user likes cats", 42); err != nil {
		t.Fatal(err)
	}

	summary, lastID, err = db.GetSummary(123)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "user likes cats" || lastID != 42 {
		t.Errorf("unexpected: %q %d", summary, lastID)
	}
}

func TestStorage_MessageCountSince(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.SaveMessage(123, "user", "a")
	id, _ := db.SaveMessage(123, "bot", "b")
	db.SaveMessage(123, "user", "c")

	count, err := db.MessageCountSince(123, id)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestStorage_IsolatesUsers(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.SaveMessage(1, "user", "alice")
	db.SaveMessage(2, "user", "bob")

	msgs, _ := db.GetLastMessages(1, 10)
	if len(msgs) != 1 || msgs[0].Text != "alice" {
		t.Errorf("user isolation broken: %v", msgs)
	}
}

func TestStorage_GetMessagesSince(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	db.SaveMessage(123, "user", "a")
	id, _ := db.SaveMessage(123, "bot", "b")
	db.SaveMessage(123, "user", "c")
	db.SaveMessage(123, "bot", "d")

	msgs, err := db.GetMessagesSince(123, id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages since id %d, got %d", id, len(msgs))
	}
	if msgs[0].Text != "c" || msgs[1].Text != "d" {
		t.Errorf("unexpected messages: %v", msgs)
	}
}

func TestStorage_SummaryUpsert(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	if err := db.UpdateSummary(123, "v1", 10); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateSummary(123, "v2", 20); err != nil {
		t.Fatal(err)
	}

	summary, lastID, err := db.GetSummary(123)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "v2" || lastID != 20 {
		t.Errorf("upsert failed: got %q %d", summary, lastID)
	}
}

func TestStorage_Capsules(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	deliver := time.Now().UTC().Add(-1 * time.Hour) // already due
	if err := db.SaveCapsule(123, "hello future me", deliver); err != nil {
		t.Fatal(err)
	}

	// Not yet due
	if err := db.SaveCapsule(123, "too early", time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	caps, err := db.GetDueCapsules()
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 1 {
		t.Fatalf("expected 1 due capsule, got %d", len(caps))
	}
	if caps[0].Text != "hello future me" {
		t.Errorf("unexpected text: %q", caps[0].Text)
	}

	if err := db.MarkCapsuleDelivered(caps[0].ID); err != nil {
		t.Fatal(err)
	}

	caps, err = db.GetDueCapsules()
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) != 0 {
		t.Fatalf("expected 0 after delivery, got %d", len(caps))
	}
}

func TestStorage_Counters(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	val, err := db.IncrementCounter(123, "messages")
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	val, err = db.IncrementCounter(123, "messages")
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}

	// Different counter
	val, err = db.IncrementCounter(123, "other")
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("expected 1 for other counter, got %d", val)
	}

	// Different user
	val, err = db.IncrementCounter(456, "messages")
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("expected 1 for different user, got %d", val)
	}
}

func TestStorage_LastMessageTime(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// No messages yet
	tm, err := db.LastMessageTime(123)
	if err != nil {
		t.Fatal(err)
	}
	if !tm.IsZero() {
		t.Error("expected zero time for no messages")
	}

	db.SaveMessage(123, "user", "hello")
	time.Sleep(10 * time.Millisecond)

	tm, err = db.LastMessageTime(123)
	if err != nil {
		t.Fatal(err)
	}
	if tm.IsZero() {
		t.Error("expected non-zero time after message")
	}
}

func TestStorage_Reminders(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Save a reminder that is already due
	due := time.Now().UTC().Add(-1 * time.Hour)
	if err := db.SaveReminder(123, "drink water", due); err != nil {
		t.Fatal(err)
	}

	// Save a reminder that is NOT due yet
	if err := db.SaveReminder(123, "future task", time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	reminders, err := db.GetDueReminders()
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 1 {
		t.Fatalf("expected 1 due reminder, got %d", len(reminders))
	}
	if reminders[0].Text != "drink water" {
		t.Errorf("unexpected text: %q", reminders[0].Text)
	}
	if reminders[0].UserID != 123 {
		t.Errorf("unexpected user_id: %d", reminders[0].UserID)
	}

	// Mark delivered
	if err := db.MarkReminderDelivered(reminders[0].ID); err != nil {
		t.Fatal(err)
	}

	reminders, err = db.GetDueReminders()
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 0 {
		t.Fatalf("expected 0 after delivery, got %d", len(reminders))
	}
}

func TestStorage_UserFacts(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// No facts initially
	facts, err := db.GetFacts(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}

	// Save facts
	if err := db.SaveFact(123, "name", "Ivan"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveFact(123, "city", "Moscow"); err != nil {
		t.Fatal(err)
	}

	facts, err = db.GetFacts(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}

	// Upsert: update existing category
	if err := db.SaveFact(123, "name", "Ivan Petrov"); err != nil {
		t.Fatal(err)
	}
	facts, err = db.GetFacts(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts after upsert, got %d", len(facts))
	}
	// Find the name fact
	for _, f := range facts {
		if f.Category == "name" && f.Fact != "Ivan Petrov" {
			t.Errorf("expected updated name, got: %q", f.Fact)
		}
	}

	// GetFactsAsText
	text, err := db.GetFactsAsText(123)
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Error("expected non-empty facts text")
	}

	// DeleteFact
	if err := db.DeleteFact(123, "name"); err != nil {
		t.Fatal(err)
	}
	facts, err = db.GetFacts(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact after delete, got %d", len(facts))
	}
	if facts[0].Category != "city" {
		t.Errorf("expected city fact to remain, got: %q", facts[0].Category)
	}
}

func TestStorage_GetFactsAsText_Empty(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	text, err := db.GetFactsAsText(999)
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("expected empty text for user with no facts, got: %q", text)
	}
}

func TestStorage_DailyLies(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	date := "2026-03-17"

	// No lie initially
	lie, truth, revealed, err := db.GetTodayLie(123, date)
	if err != nil {
		t.Fatal(err)
	}
	if lie != "" || truth != "" || revealed {
		t.Errorf("expected empty lie for new user, got: lie=%q truth=%q revealed=%v", lie, truth, revealed)
	}

	// Save lie
	if err := db.SaveLie(123, "cats can fly", "no they can't", date); err != nil {
		t.Fatal(err)
	}

	lie, truth, revealed, err = db.GetTodayLie(123, date)
	if err != nil {
		t.Fatal(err)
	}
	if lie != "cats can fly" {
		t.Errorf("unexpected lie: %q", lie)
	}
	if truth != "no they can't" {
		t.Errorf("unexpected truth: %q", truth)
	}
	if revealed {
		t.Error("expected not revealed yet")
	}

	// Reveal
	if err := db.RevealLie(123, date); err != nil {
		t.Fatal(err)
	}

	_, _, revealed, err = db.GetTodayLie(123, date)
	if err != nil {
		t.Fatal(err)
	}
	if !revealed {
		t.Error("expected revealed after RevealLie")
	}

	// Save lie for same user+date should be ignored (INSERT OR IGNORE)
	if err := db.SaveLie(123, "new lie", "new truth", date); err != nil {
		t.Fatal(err)
	}
	lie, _, _, err = db.GetTodayLie(123, date)
	if err != nil {
		t.Fatal(err)
	}
	if lie != "cats can fly" {
		t.Errorf("expected original lie to persist, got: %q", lie)
	}
}

func TestStorage_UserProfiles(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Upsert profile
	if err := db.UpsertUserProfile(123, "ivan", "Ivan", "Petrov"); err != nil {
		t.Fatal(err)
	}

	// Save a message so GetAllUsers returns this user
	if _, err := db.SaveMessage(123, "user", "hello"); err != nil {
		t.Fatal(err)
	}

	users, err := db.GetAllUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].UserID != 123 {
		t.Errorf("unexpected user_id: %d", users[0].UserID)
	}
	if users[0].Username != "ivan" {
		t.Errorf("unexpected username: %q", users[0].Username)
	}
	if users[0].FirstName != "Ivan" {
		t.Errorf("unexpected first_name: %q", users[0].FirstName)
	}
	if users[0].LastName != "Petrov" {
		t.Errorf("unexpected last_name: %q", users[0].LastName)
	}
	if users[0].MessageCount != 1 {
		t.Errorf("expected 1 message, got %d", users[0].MessageCount)
	}

	// Update profile
	if err := db.UpsertUserProfile(123, "ivan_new", "Ivan", "Sidorov"); err != nil {
		t.Fatal(err)
	}
	users, err = db.GetAllUsers()
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Username != "ivan_new" {
		t.Errorf("expected updated username, got: %q", users[0].Username)
	}
}

func TestStorage_Counters_SetAndGet(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Set counter
	if err := db.SetCounter(123, "test_counter", 42); err != nil {
		t.Fatal(err)
	}

	val, err := db.GetCounter(123, "test_counter")
	if err != nil {
		t.Fatal(err)
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	// Overwrite
	if err := db.SetCounter(123, "test_counter", 100); err != nil {
		t.Fatal(err)
	}
	val, err = db.GetCounter(123, "test_counter")
	if err != nil {
		t.Fatal(err)
	}
	if val != 100 {
		t.Errorf("expected 100 after overwrite, got %d", val)
	}
}

func TestStorage_SilenceMode(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Not in silence mode by default
	if db.IsSilenceMode(123) {
		t.Error("expected silence mode off by default")
	}
	if db.GetSilenceRemaining(123) != 0 {
		t.Error("expected 0 remaining by default")
	}

	// Set silence until future time
	future := time.Now().Add(2 * time.Hour).Unix()
	if err := db.SetCounter(123, "silence_until", int(future)); err != nil {
		t.Fatal(err)
	}

	if !db.IsSilenceMode(123) {
		t.Error("expected silence mode active")
	}
	remaining := db.GetSilenceRemaining(123)
	if remaining < 1 || remaining > 2 {
		t.Errorf("expected 1-2 hours remaining, got %d", remaining)
	}

	// Set silence until past time
	past := time.Now().Add(-1 * time.Hour).Unix()
	if err := db.SetCounter(123, "silence_until", int(past)); err != nil {
		t.Fatal(err)
	}

	if db.IsSilenceMode(123) {
		t.Error("expected silence mode off after past time")
	}
	if db.GetSilenceRemaining(123) != 0 {
		t.Errorf("expected 0 remaining after past time")
	}
}

func TestStorage_DecrementCounter(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Decrement non-existent counter -> 0
	val, err := db.DecrementCounter(123, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if val != 0 {
		t.Errorf("expected 0 for non-existent counter, got %d", val)
	}

	// Set to 3, decrement to 2
	if err := db.SetCounter(123, "lives", 3); err != nil {
		t.Fatal(err)
	}
	val, err = db.DecrementCounter(123, "lives")
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}

	// Decrement to 1
	val, err = db.DecrementCounter(123, "lives")
	if err != nil {
		t.Fatal(err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Decrement to 0
	val, err = db.DecrementCounter(123, "lives")
	if err != nil {
		t.Fatal(err)
	}
	if val != 0 {
		t.Errorf("expected 0, got %d", val)
	}

	// Decrement at 0 -> stays 0
	val, err = db.DecrementCounter(123, "lives")
	if err != nil {
		t.Fatal(err)
	}
	if val != 0 {
		t.Errorf("expected 0 when already at 0, got %d", val)
	}
}

func TestStorage_GetMoodHistory(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Insert mood messages
	db.SaveMessage(123, "user", "[mood:7]")
	db.SaveMessage(123, "user", "[mood:3]")
	db.SaveMessage(123, "user", "[mood:9]")
	// Non-mood message should be ignored
	db.SaveMessage(123, "user", "just a regular message")
	// Bot message with mood pattern should be ignored
	db.SaveMessage(123, "bot", "[mood:5]")

	entries, err := db.GetMoodHistory(123, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 mood entries, got %d", len(entries))
	}
}

func TestStorage_GetHourlyActivity(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Insert some user messages
	db.SaveMessage(123, "user", "msg1")
	db.SaveMessage(123, "user", "msg2")
	db.SaveMessage(123, "user", "msg3")
	// Bot messages should be excluded
	db.SaveMessage(123, "bot", "reply1")

	activity, err := db.GetHourlyActivity(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) == 0 {
		t.Fatal("expected at least 1 hourly activity entry")
	}

	// All messages were inserted at the same hour, so expect 1 entry
	totalCount := 0
	for _, h := range activity {
		totalCount += h.Count
		if h.Hour < 0 || h.Hour > 23 {
			t.Errorf("invalid hour: %d", h.Hour)
		}
	}
	if totalCount != 3 {
		t.Errorf("expected total count 3, got %d", totalCount)
	}
}

func TestStorage_GetMessageCount(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	count, err := db.GetMessageCount(123)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	db.SaveMessage(123, "user", "a")
	db.SaveMessage(123, "bot", "b")
	db.SaveMessage(123, "user", "c")

	count, err = db.GetMessageCount(123)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestStorage_Achievements(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	// Unlock new achievement
	isNew, err := db.UnlockAchievement(123, "first_message")
	if err != nil {
		t.Fatal(err)
	}
	if !isNew {
		t.Error("expected newly unlocked")
	}

	// Unlock same achievement again
	isNew, err = db.UnlockAchievement(123, "first_message")
	if err != nil {
		t.Fatal(err)
	}
	if isNew {
		t.Error("expected not newly unlocked for duplicate")
	}

	// Get achievements
	names, err := db.GetAchievements(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "first_message" {
		t.Errorf("unexpected achievements: %v", names)
	}

	// Different user has no achievements
	names, err = db.GetAchievements(456)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 achievements for different user, got %d", len(names))
	}
}

func newTestDB(t *testing.T) *storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
