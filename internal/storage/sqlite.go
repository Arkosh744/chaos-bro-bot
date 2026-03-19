package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// migrations is a versioned list of database migration functions.
// Each function is executed exactly once, in order.
// IMPORTANT: never reorder or remove existing migrations — only append new ones.
var migrations = []func(tx *sql.Tx) error{
	// v1: initial schema — all tables from the monolithic CREATE TABLE block
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				role TEXT NOT NULL,
				text TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_messages_user ON messages(user_id, id DESC);
			CREATE TABLE IF NOT EXISTS context_summary (
				user_id INTEGER PRIMARY KEY,
				summary TEXT NOT NULL DEFAULT '',
				last_message_id INTEGER NOT NULL DEFAULT 0,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS capsules (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				text TEXT NOT NULL,
				deliver_at TIMESTAMP NOT NULL,
				delivered INTEGER NOT NULL DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_capsules_deliver ON capsules(delivered, deliver_at);
			CREATE TABLE IF NOT EXISTS counters (
				user_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				value INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (user_id, name)
			);
			CREATE TABLE IF NOT EXISTS achievements (
				user_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				unlocked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (user_id, name)
			);
			CREATE TABLE IF NOT EXISTS user_facts (
				user_id INTEGER NOT NULL,
				category TEXT NOT NULL,
				fact TEXT NOT NULL,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (user_id, category)
			);
			CREATE TABLE IF NOT EXISTS daily_lies (
				user_id INTEGER NOT NULL,
				lie_text TEXT NOT NULL,
				truth_text TEXT NOT NULL,
				revealed INTEGER NOT NULL DEFAULT 0,
				created_date TEXT NOT NULL,
				PRIMARY KEY (user_id, created_date)
			);
			CREATE TABLE IF NOT EXISTS user_profiles (
				user_id INTEGER PRIMARY KEY,
				username TEXT NOT NULL DEFAULT '',
				first_name TEXT NOT NULL DEFAULT '',
				last_name TEXT NOT NULL DEFAULT '',
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS reminders (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				text TEXT NOT NULL,
				remind_at TIMESTAMP NOT NULL,
				delivered INTEGER NOT NULL DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_reminders_deliver ON reminders(delivered, remind_at);
			CREATE TABLE IF NOT EXISTS habits (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				active INTEGER NOT NULL DEFAULT 1,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS habit_logs (
				habit_id INTEGER NOT NULL,
				done_date TEXT NOT NULL,
				PRIMARY KEY (habit_id, done_date)
			);
			CREATE TABLE IF NOT EXISTS reflections (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER NOT NULL,
				category TEXT NOT NULL,
				text TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX IF NOT EXISTS idx_reflections_user ON reflections(user_id, created_at DESC);
			CREATE TABLE IF NOT EXISTS prompt_overrides (
				name TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS weekly_challenges (
				user_id INTEGER NOT NULL,
				challenge TEXT NOT NULL,
				week_start TEXT NOT NULL,
				completed_days INTEGER NOT NULL DEFAULT 0,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (user_id, week_start)
			);
			CREATE TABLE IF NOT EXISTS custom_easter_eggs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				trigger_text TEXT NOT NULL UNIQUE,
				response TEXT NOT NULL,
				active INTEGER NOT NULL DEFAULT 1,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS custom_achievements (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				emoji TEXT NOT NULL DEFAULT '🏆',
				description TEXT NOT NULL,
				event TEXT NOT NULL,
				threshold INTEGER NOT NULL DEFAULT 1,
				active INTEGER NOT NULL DEFAULT 1
			);
			CREATE TABLE IF NOT EXISTS duels (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				chat_id INTEGER NOT NULL,
				challenger_id INTEGER NOT NULL,
				opponent_id INTEGER NOT NULL,
				question TEXT NOT NULL,
				challenger_answer TEXT,
				opponent_answer TEXT,
				winner_id INTEGER,
				status TEXT NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS group_quests (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				chat_id INTEGER NOT NULL,
				quest TEXT NOT NULL,
				answer_hint TEXT,
				winner_id INTEGER,
				status TEXT NOT NULL DEFAULT 'active',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
			CREATE TABLE IF NOT EXISTS linked_users (
				user_a INTEGER NOT NULL,
				user_b INTEGER NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (user_a, user_b)
			);
		`)
		return err
	},
	// v2: runtime_config table for hot-reloadable settings
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS runtime_config (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);
		`)
		return err
	},
}

type Message struct {
	ID        int64
	UserID    int64
	Role      string // "user" or "bot"
	Text      string
	CreatedAt time.Time
}

type Storage struct {
	db     *sql.DB
	dbPath string
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Storage{db: db, dbPath: dbPath}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Backup creates a backup of the database at the given path using VACUUM INTO.
func (s *Storage) Backup(destPath string) error {
	_, err := s.db.Exec("VACUUM INTO ?", destPath)
	if err != nil {
		return fmt.Errorf("vacuum into %s: %w", destPath, err)
	}
	return nil
}

// DBPath returns the path to the database file.
func (s *Storage) DBPath() string {
	return s.dbPath
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) migrate() error {
	// Ensure schema_version table exists (outside transaction — it's idempotent).
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Read current version (0 if no rows yet).
	var currentVersion int
	err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if currentVersion >= len(migrations) {
		return nil // already up to date
	}

	for i := currentVersion; i < len(migrations); i++ {
		version := i + 1

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration v%d: %w", version, err)
		}

		if err := migrations[i](tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("run migration v%d: %w", version, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("update schema version to v%d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration v%d: %w", version, err)
		}

		log.Printf("storage: applied migration v%d", version)
	}

	return nil
}

func (s *Storage) SaveMessage(userID int64, role, text string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO messages (user_id, role, text) VALUES (?, ?, ?)",
		userID, role, text,
	)
	if err != nil {
		return 0, fmt.Errorf("save message: %w", err)
	}
	return res.LastInsertId()
}

func (s *Storage) GetLastMessages(userID int64, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, role, text, created_at FROM messages WHERE user_id = ? ORDER BY id DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get last messages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Text, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	// Reverse to chronological order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Storage) GetSummary(userID int64) (string, int64, error) {
	var summary string
	var lastID int64
	err := s.db.QueryRow(
		"SELECT summary, last_message_id FROM context_summary WHERE user_id = ?", userID,
	).Scan(&summary, &lastID)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return summary, lastID, err
}

func (s *Storage) UpdateSummary(userID int64, summary string, lastMessageID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO context_summary (user_id, summary, last_message_id, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			summary = excluded.summary,
			last_message_id = excluded.last_message_id,
			updated_at = excluded.updated_at`,
		userID, summary, lastMessageID,
	)
	return err
}

func (s *Storage) MessageCountSince(userID, sinceID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE user_id = ? AND id > ?",
		userID, sinceID,
	).Scan(&count)
	return count, err
}

func (s *Storage) LastMessageTime(userID int64) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRow(
		"SELECT created_at FROM messages WHERE user_id = ? AND role = 'user' ORDER BY id DESC LIMIT 1",
		userID,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	return t, err
}

func (s *Storage) GetMessagesSince(userID, sinceID int64, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, role, text, created_at FROM messages WHERE user_id = ? AND id > ? ORDER BY id ASC LIMIT ?",
		userID, sinceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Text, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// --- Capsules ---

type Capsule struct {
	ID        int64
	UserID    int64
	Text      string
	DeliverAt time.Time
}

func (s *Storage) SaveCapsule(userID int64, text string, deliverAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO capsules (user_id, text, deliver_at) VALUES (?, ?, ?)",
		userID, text, deliverAt,
	)
	return err
}

func (s *Storage) GetDueCapsules() ([]Capsule, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, text, deliver_at FROM capsules WHERE delivered = 0 AND deliver_at <= CURRENT_TIMESTAMP",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var caps []Capsule
	for rows.Next() {
		var c Capsule
		if err := rows.Scan(&c.ID, &c.UserID, &c.Text, &c.DeliverAt); err != nil {
			return nil, err
		}
		caps = append(caps, c)
	}
	return caps, nil
}

func (s *Storage) MarkCapsuleDelivered(id int64) error {
	_, err := s.db.Exec("UPDATE capsules SET delivered = 1 WHERE id = ?", id)
	return err
}

// --- Counters ---

// --- Achievements ---

// UnlockAchievement tries to unlock an achievement for a user.
// Returns true if newly unlocked, false if already existed.
func (s *Storage) UnlockAchievement(userID int64, name string) (bool, error) {
	res, err := s.db.Exec(
		"INSERT OR IGNORE INTO achievements (user_id, name) VALUES (?, ?)",
		userID, name,
	)
	if err != nil {
		return false, fmt.Errorf("unlock achievement: %w", err)
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// GetAchievements returns all unlocked achievement names for a user, ordered by unlock time.
func (s *Storage) GetAchievements(userID int64) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT name FROM achievements WHERE user_id = ? ORDER BY unlocked_at",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get achievements: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan achievement: %w", err)
		}
		names = append(names, name)
	}
	return names, nil
}

// GetCounter returns the current counter value without incrementing.
func (s *Storage) GetCounter(userID int64, name string) (int, error) {
	var val int
	err := s.db.QueryRow(
		"SELECT value FROM counters WHERE user_id = ? AND name = ?",
		userID, name,
	).Scan(&val)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// --- User Facts ---

type UserFact struct {
	Category  string
	Fact      string
	UpdatedAt time.Time
}

func (s *Storage) SaveFact(userID int64, category, fact string) error {
	_, err := s.db.Exec(`
		INSERT INTO user_facts (user_id, category, fact, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, category) DO UPDATE SET
			fact = excluded.fact,
			updated_at = excluded.updated_at`,
		userID, category, fact,
	)
	return err
}

func (s *Storage) GetFacts(userID int64) ([]UserFact, error) {
	rows, err := s.db.Query(
		"SELECT category, fact, updated_at FROM user_facts WHERE user_id = ? ORDER BY category",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get facts: %w", err)
	}
	defer rows.Close()

	var facts []UserFact
	for rows.Next() {
		var f UserFact
		if err := rows.Scan(&f.Category, &f.Fact, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		facts = append(facts, f)
	}
	return facts, nil
}

// GetFact returns a single fact value by category. Returns empty string if not found.
func (s *Storage) GetFact(userID int64, category string) (string, error) {
	var fact string
	err := s.db.QueryRow(
		"SELECT fact FROM user_facts WHERE user_id = ? AND category = ?",
		userID, category,
	).Scan(&fact)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get fact: %w", err)
	}
	return fact, nil
}

func (s *Storage) GetFactsAsText(userID int64) (string, error) {
	facts, err := s.GetFacts(userID)
	if err != nil {
		return "", err
	}
	if len(facts) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for _, f := range facts {
		sb.WriteString(f.Category + ": " + f.Fact + "\n")
	}
	return sb.String(), nil
}

// GetMessageCount returns the total number of messages for a user.
func (s *Storage) GetMessageCount(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE user_id = ?",
		userID,
	).Scan(&count)
	return count, err
}

// GetMessageCountToday returns the number of messages for a user since midnight.
func (s *Storage) GetMessageCountToday(userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE user_id = ? AND created_at >= date('now', 'start of day')",
		userID,
	).Scan(&count)
	return count, err
}

// MoodEntry represents a single mood data point.
type MoodEntry struct {
	Score     int
	CreatedAt time.Time
}

// GetMoodHistory extracts mood scores from [mood:N] messages over the last N days.
func (s *Storage) GetMoodHistory(userID int64, days int) ([]MoodEntry, error) {
	rows, err := s.db.Query(`
		SELECT
			CAST(SUBSTR(text, 7, LENGTH(text) - 7) AS INTEGER) AS score,
			created_at
		FROM messages
		WHERE user_id = ?
			AND role = 'user'
			AND text LIKE '[mood:%]'
			AND created_at >= datetime('now', ? || ' days')
		ORDER BY created_at ASC`,
		userID, fmt.Sprintf("-%d", days),
	)
	if err != nil {
		return nil, fmt.Errorf("get mood history: %w", err)
	}
	defer rows.Close()

	var entries []MoodEntry
	for rows.Next() {
		var e MoodEntry
		if err := rows.Scan(&e.Score, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mood entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// HourlyActivity represents message count for a specific hour.
type HourlyActivity struct {
	Hour  int
	Count int
}

// GetHourlyActivity returns message counts grouped by hour of day.
func (s *Storage) GetHourlyActivity(userID int64) ([]HourlyActivity, error) {
	rows, err := s.db.Query(`
		SELECT CAST(strftime('%H', created_at) AS INTEGER) AS hour, COUNT(*) AS cnt
		FROM messages
		WHERE user_id = ? AND role = 'user'
		GROUP BY hour
		ORDER BY hour`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get hourly activity: %w", err)
	}
	defer rows.Close()

	var activity []HourlyActivity
	for rows.Next() {
		var h HourlyActivity
		if err := rows.Scan(&h.Hour, &h.Count); err != nil {
			return nil, fmt.Errorf("scan hourly activity: %w", err)
		}
		activity = append(activity, h)
	}
	return activity, nil
}

// GetMessageCountSinceDate returns the number of messages for a user since a given time.
func (s *Storage) GetMessageCountSinceDate(userID int64, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE user_id = ? AND created_at >= ?",
		userID, since,
	).Scan(&count)
	return count, err
}

// DeleteFact removes a user fact by category.
func (s *Storage) DeleteFact(userID int64, category string) error {
	_, err := s.db.Exec(
		"DELETE FROM user_facts WHERE user_id = ? AND category = ?",
		userID, category,
	)
	return err
}

// --- User Profiles (Telegram info) ---

// UpsertUserProfile saves or updates user's Telegram profile info.
func (s *Storage) UpsertUserProfile(userID int64, username, firstName, lastName string) error {
	_, err := s.db.Exec(`
		INSERT INTO user_profiles (user_id, username, first_name, last_name, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			username = excluded.username,
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			updated_at = excluded.updated_at`,
		userID, username, firstName, lastName,
	)
	return err
}

// UserInfo represents a user with aggregate message stats.
type UserInfo struct {
	UserID       int64
	Username     string
	FirstName    string
	LastName     string
	MessageCount int
	LastMessage  time.Time
}

// GetAllUsers returns all users who have sent or received messages, ordered by last activity.
func (s *Storage) GetAllUsers() ([]UserInfo, error) {
	rows, err := s.db.Query(`
		SELECT m.user_id,
			COALESCE(p.username, ''), COALESCE(p.first_name, ''), COALESCE(p.last_name, ''),
			COUNT(*) AS msg_count, COALESCE(MAX(m.created_at), '') AS last_msg
		FROM messages m
		LEFT JOIN user_profiles p ON m.user_id = p.user_id
		GROUP BY m.user_id
		ORDER BY last_msg DESC`)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
	}
	defer rows.Close()

	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		var lastMsg string
		if err := rows.Scan(&u.UserID, &u.Username, &u.FirstName, &u.LastName, &u.MessageCount, &lastMsg); err != nil {
			return nil, fmt.Errorf("scan user info: %w", err)
		}
		if lastMsg != "" {
			u.LastMessage, _ = time.Parse("2006-01-02 15:04:05", lastMsg)
		}
		users = append(users, u)
	}
	return users, nil
}

// --- Daily Lies ---

// SaveLie stores a lie for the user on a given date.
func (s *Storage) SaveLie(userID int64, lie, truth, date string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO daily_lies (user_id, lie_text, truth_text, created_date) VALUES (?, ?, ?, ?)",
		userID, lie, truth, date,
	)
	return err
}

// GetTodayLie returns today's lie for the user. Returns empty strings if no lie exists.
func (s *Storage) GetTodayLie(userID int64, date string) (lie string, truth string, revealed bool, err error) {
	var revealedInt int
	err = s.db.QueryRow(
		"SELECT lie_text, truth_text, revealed FROM daily_lies WHERE user_id = ? AND created_date = ?",
		userID, date,
	).Scan(&lie, &truth, &revealedInt)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	return lie, truth, revealedInt == 1, err
}

// RevealLie marks today's lie as revealed for the user.
func (s *Storage) RevealLie(userID int64, date string) error {
	_, err := s.db.Exec(
		"UPDATE daily_lies SET revealed = 1 WHERE user_id = ? AND created_date = ?",
		userID, date,
	)
	return err
}

// SetCounter sets a counter to a specific value (upsert).
func (s *Storage) SetCounter(userID int64, name string, value int) error {
	_, err := s.db.Exec(`
		INSERT INTO counters (user_id, name, value) VALUES (?, ?, ?)
		ON CONFLICT(user_id, name) DO UPDATE SET value = excluded.value`,
		userID, name, value,
	)
	return err
}

// IsSilenceMode checks if silence mode is active for a user.
// Returns true if current time is before the stored unix timestamp.
func (s *Storage) IsSilenceMode(userID int64) bool {
	val, err := s.GetCounter(userID, "silence_until")
	if err != nil {
		return false
	}
	return time.Now().Unix() < int64(val)
}

// GetSilenceRemaining returns how many hours remain in silence mode.
// Returns 0 if silence mode is not active.
func (s *Storage) GetSilenceRemaining(userID int64) int {
	val, err := s.GetCounter(userID, "silence_until")
	if err != nil {
		return 0
	}
	until := time.Unix(int64(val), 0)
	remaining := time.Until(until)
	if remaining <= 0 {
		return 0
	}
	hours := int(remaining.Hours())
	if hours == 0 {
		return 1 // less than an hour, but still active
	}
	return hours
}

// DecrementCounter decreases a counter by 1. Returns the new value.
// If the counter is already 0 or doesn't exist, returns 0 without modification.
func (s *Storage) DecrementCounter(userID int64, name string) (int, error) {
	val, err := s.GetCounter(userID, name)
	if err != nil || val <= 0 {
		return 0, nil
	}

	newVal := val - 1
	_, err = s.db.Exec(
		"UPDATE counters SET value = ? WHERE user_id = ? AND name = ?",
		newVal, userID, name,
	)
	if err != nil {
		return 0, fmt.Errorf("decrement counter: %w", err)
	}
	return newVal, nil
}

func (s *Storage) IncrementCounter(userID int64, name string) (int, error) {
	_, err := s.db.Exec(`
		INSERT INTO counters (user_id, name, value) VALUES (?, ?, 1)
		ON CONFLICT(user_id, name) DO UPDATE SET value = value + 1`,
		userID, name,
	)
	if err != nil {
		return 0, err
	}
	var val int
	err = s.db.QueryRow("SELECT value FROM counters WHERE user_id = ? AND name = ?", userID, name).Scan(&val)
	return val, err
}

// --- Reminders ---

type Reminder struct {
	ID       int64
	UserID   int64
	Text     string
	RemindAt time.Time
}

func (s *Storage) SaveReminder(userID int64, text string, remindAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO reminders (user_id, text, remind_at) VALUES (?, ?, ?)",
		userID, text, remindAt,
	)
	return err
}

func (s *Storage) GetDueReminders() ([]Reminder, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, text, remind_at FROM reminders WHERE delivered = 0 AND remind_at <= CURRENT_TIMESTAMP",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		if err := rows.Scan(&r.ID, &r.UserID, &r.Text, &r.RemindAt); err != nil {
			return nil, err
		}
		reminders = append(reminders, r)
	}
	return reminders, nil
}

func (s *Storage) MarkReminderDelivered(id int64) error {
	_, err := s.db.Exec("UPDATE reminders SET delivered = 1 WHERE id = ?", id)
	return err
}

// --- Habits ---

type Habit struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

func (s *Storage) AddHabit(userID int64, name string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO habits (user_id, name) VALUES (?, ?)",
		userID, name,
	)
	if err != nil {
		return 0, fmt.Errorf("add habit: %w", err)
	}
	return res.LastInsertId()
}

// GetHabits returns all active habits for a user.
func (s *Storage) GetHabits(userID int64) ([]Habit, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, name, created_at FROM habits WHERE user_id = ? AND active = 1 ORDER BY id",
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get habits: %w", err)
	}
	defer rows.Close()

	var habits []Habit
	for rows.Next() {
		var h Habit
		if err := rows.Scan(&h.ID, &h.UserID, &h.Name, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan habit: %w", err)
		}
		habits = append(habits, h)
	}
	return habits, nil
}

// DeleteHabit soft-deletes a habit by setting active=0.
func (s *Storage) DeleteHabit(id int64) error {
	_, err := s.db.Exec("UPDATE habits SET active = 0 WHERE id = ?", id)
	return err
}

// LogHabit marks a habit as done for a given date.
func (s *Storage) LogHabit(habitID int64, date string) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO habit_logs (habit_id, done_date) VALUES (?, ?)",
		habitID, date,
	)
	return err
}

// GetHabitLog checks if a habit was done on a given date.
func (s *Storage) GetHabitLog(habitID int64, date string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM habit_logs WHERE habit_id = ? AND done_date = ?",
		habitID, date,
	).Scan(&count)
	return count > 0, err
}

// GetHabitStreak counts consecutive days a habit was completed (ending today or yesterday).
func (s *Storage) GetHabitStreak(habitID int64) (int, error) {
	today := time.Now()
	streak := 0

	for i := 0; i < 365; i++ {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		done, err := s.GetHabitLog(habitID, date)
		if err != nil {
			return streak, err
		}
		if !done {
			// Allow skipping today (day not over yet)
			if i == 0 {
				continue
			}
			break
		}
		streak++
	}
	return streak, nil
}

// GetHabitStats returns a map of habitID -> done count for the last N days.
func (s *Storage) GetHabitStats(userID int64, days int) (map[int64]int, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(`
		SELECT hl.habit_id, COUNT(*) as cnt
		FROM habit_logs hl
		JOIN habits h ON h.id = hl.habit_id
		WHERE h.user_id = ? AND h.active = 1 AND hl.done_date >= ?
		GROUP BY hl.habit_id`,
		userID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get habit stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[int64]int)
	for rows.Next() {
		var habitID int64
		var count int
		if err := rows.Scan(&habitID, &count); err != nil {
			return nil, fmt.Errorf("scan habit stats: %w", err)
		}
		stats[habitID] = count
	}
	return stats, nil
}

// GetAllUsersWithHabits returns distinct user IDs that have active habits.
func (s *Storage) GetAllUsersWithHabits() ([]int64, error) {
	rows, err := s.db.Query("SELECT DISTINCT user_id FROM habits WHERE active = 1")
	if err != nil {
		return nil, fmt.Errorf("get users with habits: %w", err)
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, nil
}

// GetUndoneHabits returns active habits that are NOT done for the given date.
func (s *Storage) GetUndoneHabits(userID int64, date string) ([]Habit, error) {
	rows, err := s.db.Query(`
		SELECT h.id, h.user_id, h.name, h.created_at
		FROM habits h
		WHERE h.user_id = ? AND h.active = 1
			AND h.id NOT IN (SELECT habit_id FROM habit_logs WHERE done_date = ?)
		ORDER BY h.id`,
		userID, date,
	)
	if err != nil {
		return nil, fmt.Errorf("get undone habits: %w", err)
	}
	defer rows.Close()

	var habits []Habit
	for rows.Next() {
		var h Habit
		if err := rows.Scan(&h.ID, &h.UserID, &h.Name, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan undone habit: %w", err)
		}
		habits = append(habits, h)
	}
	return habits, nil
}

// --- Sleep tracking ---

// GetFirstAndLastMessageTimes returns the first and last user message times for a given date (YYYY-MM-DD).
func (s *Storage) GetFirstAndLastMessageTimes(userID int64, date string) (first, last time.Time, err error) {
	err = s.db.QueryRow(`
		SELECT MIN(created_at), MAX(created_at)
		FROM messages
		WHERE user_id = ? AND role = 'user'
			AND date(created_at) = ?`,
		userID, date,
	).Scan(&first, &last)
	if err == sql.ErrNoRows {
		return time.Time{}, time.Time{}, nil
	}
	return first, last, err
}

// GetLateNightMessageCount returns how many days in the last N days the user sent messages after the given hour.
func (s *Storage) GetLateNightMessageCount(userID int64, days int, afterHour int) (int, error) {
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT date(created_at))
		FROM messages
		WHERE user_id = ? AND role = 'user'
			AND date(created_at) >= ?
			AND CAST(strftime('%H', created_at) AS INTEGER) >= ?`,
		userID, since, afterHour,
	).Scan(&count)
	return count, err
}

// --- Reflections ---

// Reflection represents an evening reflection entry.
type Reflection struct {
	ID        int64
	UserID    int64
	Category  string
	Text      string
	CreatedAt time.Time
}

// SaveReflection stores an evening reflection entry.
func (s *Storage) SaveReflection(userID int64, category, text string) error {
	_, err := s.db.Exec(
		"INSERT INTO reflections (user_id, category, text) VALUES (?, ?, ?)",
		userID, category, text,
	)
	return err
}

// GetReflections returns reflections for a user from the last N days.
func (s *Storage) GetReflections(userID int64, days int) ([]Reflection, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, category, text, created_at
		FROM reflections
		WHERE user_id = ? AND created_at >= datetime('now', ? || ' days')
		ORDER BY created_at DESC`,
		userID, fmt.Sprintf("-%d", days),
	)
	if err != nil {
		return nil, fmt.Errorf("get reflections: %w", err)
	}
	defer rows.Close()

	var refs []Reflection
	for rows.Next() {
		var r Reflection
		if err := rows.Scan(&r.ID, &r.UserID, &r.Category, &r.Text, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan reflection: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, nil
}

// --- Weekly Challenges ---

// SaveWeeklyChallenge stores a weekly challenge for a user.
func (s *Storage) SaveWeeklyChallenge(userID int64, challenge, weekStart string) error {
	_, err := s.db.Exec(`
		INSERT INTO weekly_challenges (user_id, challenge, week_start)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, week_start) DO UPDATE SET challenge = excluded.challenge`,
		userID, challenge, weekStart,
	)
	return err
}

// GetCurrentChallenge returns the active weekly challenge (current week, Monday-based).
func (s *Storage) GetCurrentChallenge(userID int64) (challenge string, weekStart string, completedDays int, err error) {
	now := time.Now()
	// Calculate Monday of current week
	offset := int(now.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	monday := now.AddDate(0, 0, -offset)
	weekStart = monday.Format("2006-01-02")

	err = s.db.QueryRow(
		"SELECT challenge, week_start, completed_days FROM weekly_challenges WHERE user_id = ? AND week_start = ?",
		userID, weekStart,
	).Scan(&challenge, &weekStart, &completedDays)
	if err == sql.ErrNoRows {
		return "", weekStart, 0, nil
	}
	return challenge, weekStart, completedDays, err
}

// IncrementChallengeDays increments the completed_days counter for a weekly challenge.
func (s *Storage) IncrementChallengeDays(userID int64, weekStart string) error {
	_, err := s.db.Exec(
		"UPDATE weekly_challenges SET completed_days = completed_days + 1 WHERE user_id = ? AND week_start = ?",
		userID, weekStart,
	)
	return err
}

// --- Custom Easter Eggs ---

// EasterEggEntry represents a custom easter egg.
type EasterEggEntry struct {
	ID       int64
	Trigger  string
	Response string
}

// AddCustomEasterEgg adds a new custom easter egg.
func (s *Storage) AddCustomEasterEgg(trigger, response string) error {
	_, err := s.db.Exec(
		"INSERT INTO custom_easter_eggs (trigger_text, response) VALUES (?, ?)",
		trigger, response,
	)
	return err
}

// GetCustomEasterEggs returns all active custom easter eggs as a trigger->response map.
func (s *Storage) GetCustomEasterEggs() (map[string]string, error) {
	rows, err := s.db.Query("SELECT trigger_text, response FROM custom_easter_eggs WHERE active = 1")
	if err != nil {
		return nil, fmt.Errorf("get custom easter eggs: %w", err)
	}
	defer rows.Close()

	eggs := make(map[string]string)
	for rows.Next() {
		var trigger, response string
		if err := rows.Scan(&trigger, &response); err != nil {
			return nil, fmt.Errorf("scan custom easter egg: %w", err)
		}
		eggs[trigger] = response
	}
	return eggs, nil
}

// ListCustomEasterEggs returns all custom easter eggs with their IDs.
func (s *Storage) ListCustomEasterEggs() ([]EasterEggEntry, error) {
	rows, err := s.db.Query("SELECT id, trigger_text, response FROM custom_easter_eggs WHERE active = 1 ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list custom easter eggs: %w", err)
	}
	defer rows.Close()

	var eggs []EasterEggEntry
	for rows.Next() {
		var e EasterEggEntry
		if err := rows.Scan(&e.ID, &e.Trigger, &e.Response); err != nil {
			return nil, fmt.Errorf("scan custom easter egg: %w", err)
		}
		eggs = append(eggs, e)
	}
	return eggs, nil
}

// DeleteCustomEasterEgg soft-deletes a custom easter egg by ID.
func (s *Storage) DeleteCustomEasterEgg(id int64) error {
	_, err := s.db.Exec("UPDATE custom_easter_eggs SET active = 0 WHERE id = ?", id)
	return err
}

// --- Prompt Overrides ---

// GetPromptOverride returns the override value for a prompt name, or empty string if not found.
func (s *Storage) GetPromptOverride(name string) (string, error) {
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM prompt_overrides WHERE name = ?", name,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get prompt override %s: %w", name, err)
	}
	return value, nil
}

// SavePromptOverride upserts a prompt override value.
func (s *Storage) SavePromptOverride(name, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO prompt_overrides (name, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		name, value,
	)
	if err != nil {
		return fmt.Errorf("save prompt override %s: %w", name, err)
	}
	return nil
}

// GetAllPromptOverrides returns all prompt overrides as a map.
func (s *Storage) GetAllPromptOverrides() (map[string]string, error) {
	rows, err := s.db.Query("SELECT name, value FROM prompt_overrides")
	if err != nil {
		return nil, fmt.Errorf("get all prompt overrides: %w", err)
	}
	defer rows.Close()

	overrides := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, fmt.Errorf("scan prompt override: %w", err)
		}
		overrides[name] = value
	}
	return overrides, nil
}

// DeletePromptOverride removes a prompt override, restoring the default.
func (s *Storage) DeletePromptOverride(name string) error {
	_, err := s.db.Exec("DELETE FROM prompt_overrides WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("delete prompt override %s: %w", name, err)
	}
	return nil
}

// --- Analytics ---

// DayCount represents message count for a specific day.
type DayCount struct {
	Date  string
	Count int
}

// CommandCount represents usage count for a specific command or button text.
type CommandCount struct {
	Command string
	Count   int
}

// GetMessagesByDay returns daily message counts for a user over the last N days.
func (s *Storage) GetMessagesByDay(userID int64, days int) ([]DayCount, error) {
	rows, err := s.db.Query(`
		SELECT date(created_at) AS day, COUNT(*) AS cnt
		FROM messages
		WHERE user_id = ? AND role = 'user'
			AND created_at >= datetime('now', ? || ' days')
		GROUP BY day
		ORDER BY day ASC`,
		userID, fmt.Sprintf("-%d", days),
	)
	if err != nil {
		return nil, fmt.Errorf("get messages by day: %w", err)
	}
	defer rows.Close()

	var result []DayCount
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, fmt.Errorf("scan day count: %w", err)
		}
		result = append(result, d)
	}
	return result, nil
}

// GetTopCommands returns the most used commands (messages starting with / or matching button texts).
func (s *Storage) GetTopCommands(userID int64, limit int) ([]CommandCount, error) {
	rows, err := s.db.Query(`
		SELECT
			CASE
				WHEN text LIKE '/%' THEN LOWER(
					CASE WHEN INSTR(text, ' ') > 0
						THEN SUBSTR(text, 1, INSTR(text, ' ') - 1)
						ELSE text
					END
				)
				ELSE text
			END AS cmd,
			COUNT(*) AS cnt
		FROM messages
		WHERE user_id = ? AND role = 'user'
			AND (text LIKE '/%' OR text IN (
				'👁 Очнись', '🎲 Ебани куба', '🔮 Судьба',
				'🎱 Кинь кости', '🫁 Дыши', '🔥 Зажарь',
				'🧙 Мудрость', '⭐ Гороскоп', '📊 Настроение',
				'🪞 Зеркало', '[button]'
			))
		GROUP BY cmd
		ORDER BY cnt DESC
		LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get top commands: %w", err)
	}
	defer rows.Close()

	var result []CommandCount
	for rows.Next() {
		var c CommandCount
		if err := rows.Scan(&c.Command, &c.Count); err != nil {
			return nil, fmt.Errorf("scan command count: %w", err)
		}
		result = append(result, c)
	}
	return result, nil
}

// GetAverageWordCount returns the average number of words per user message.
func (s *Storage) GetAverageWordCount(userID int64) (float64, error) {
	var avg sql.NullFloat64
	err := s.db.QueryRow(`
		SELECT AVG(
			LENGTH(TRIM(text)) - LENGTH(REPLACE(TRIM(text), ' ', '')) + 1
		)
		FROM messages
		WHERE user_id = ? AND role = 'user'
			AND text NOT LIKE '[%'
			AND text NOT LIKE '/%'
			AND LENGTH(TRIM(text)) > 0`,
		userID,
	).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("get average word count: %w", err)
	}
	if !avg.Valid {
		return 0, nil
	}
	return avg.Float64, nil
}

// --- Custom Achievements ---

// CustomAchievement represents a user-defined achievement stored in DB.
type CustomAchievement struct {
	ID          int64
	Name        string
	Emoji       string
	Description string
	Event       string
	Threshold   int
	Active      bool
}

// AddCustomAchievement inserts a new custom achievement definition.
func (s *Storage) AddCustomAchievement(name, emoji, desc, event string, threshold int) error {
	_, err := s.db.Exec(
		"INSERT INTO custom_achievements (name, emoji, description, event, threshold) VALUES (?, ?, ?, ?, ?)",
		name, emoji, desc, event, threshold,
	)
	if err != nil {
		return fmt.Errorf("add custom achievement: %w", err)
	}
	return nil
}

// ListCustomAchievements returns all custom achievements (active and inactive).
func (s *Storage) ListCustomAchievements() ([]CustomAchievement, error) {
	rows, err := s.db.Query(
		"SELECT id, name, emoji, description, event, threshold, active FROM custom_achievements ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("list custom achievements: %w", err)
	}
	defer rows.Close()

	var result []CustomAchievement
	for rows.Next() {
		var a CustomAchievement
		var active int
		if err := rows.Scan(&a.ID, &a.Name, &a.Emoji, &a.Description, &a.Event, &a.Threshold, &active); err != nil {
			return nil, fmt.Errorf("scan custom achievement: %w", err)
		}
		a.Active = active == 1
		result = append(result, a)
	}
	return result, nil
}

// DeleteCustomAchievement soft-deletes a custom achievement by ID.
func (s *Storage) DeleteCustomAchievement(id int64) error {
	_, err := s.db.Exec("UPDATE custom_achievements SET active = 0 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete custom achievement: %w", err)
	}
	return nil
}

// GetActiveCustomAchievements returns all active custom achievements.
func (s *Storage) GetActiveCustomAchievements() ([]CustomAchievement, error) {
	rows, err := s.db.Query(
		"SELECT id, name, emoji, description, event, threshold, active FROM custom_achievements WHERE active = 1 ORDER BY id",
	)
	if err != nil {
		return nil, fmt.Errorf("get active custom achievements: %w", err)
	}
	defer rows.Close()

	var result []CustomAchievement
	for rows.Next() {
		var a CustomAchievement
		var active int
		if err := rows.Scan(&a.ID, &a.Name, &a.Emoji, &a.Description, &a.Event, &a.Threshold, &active); err != nil {
			return nil, fmt.Errorf("scan custom achievement: %w", err)
		}
		a.Active = active == 1
		result = append(result, a)
	}
	return result, nil
}

// --- Duels ---

// Duel represents a duel between two users in a group chat.
type Duel struct {
	ID               int64
	ChatID           int64
	ChallengerID     int64
	OpponentID       int64
	Question         string
	ChallengerAnswer string
	OpponentAnswer   string
	WinnerID         int64
	Status           string
	CreatedAt        time.Time
}

// CreateDuel creates a new duel in a group chat.
func (s *Storage) CreateDuel(chatID, challengerID, opponentID int64, question string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO duels (chat_id, challenger_id, opponent_id, question) VALUES (?, ?, ?, ?)",
		chatID, challengerID, opponentID, question,
	)
	if err != nil {
		return 0, fmt.Errorf("create duel: %w", err)
	}
	return res.LastInsertId()
}

// GetActiveDuel returns the active (pending) duel for a group chat.
func (s *Storage) GetActiveDuel(chatID int64) (*Duel, error) {
	var d Duel
	var challengerAnswer, opponentAnswer sql.NullString
	err := s.db.QueryRow(`
		SELECT id, chat_id, challenger_id, opponent_id, question,
			challenger_answer, opponent_answer, COALESCE(winner_id, 0), status, created_at
		FROM duels
		WHERE chat_id = ? AND status = 'pending'
		ORDER BY id DESC LIMIT 1`,
		chatID,
	).Scan(&d.ID, &d.ChatID, &d.ChallengerID, &d.OpponentID, &d.Question,
		&challengerAnswer, &opponentAnswer, &d.WinnerID, &d.Status, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active duel: %w", err)
	}
	if challengerAnswer.Valid {
		d.ChallengerAnswer = challengerAnswer.String
	}
	if opponentAnswer.Valid {
		d.OpponentAnswer = opponentAnswer.String
	}
	return &d, nil
}

// SubmitDuelAnswer saves a participant's answer in an active duel.
func (s *Storage) SubmitDuelAnswer(duelID, userID int64, answer string) error {
	// Determine which column to update based on userID
	var d Duel
	err := s.db.QueryRow("SELECT challenger_id, opponent_id FROM duels WHERE id = ?", duelID).
		Scan(&d.ChallengerID, &d.OpponentID)
	if err != nil {
		return fmt.Errorf("get duel participants: %w", err)
	}

	var column string
	if userID == d.ChallengerID {
		column = "challenger_answer"
	} else if userID == d.OpponentID {
		column = "opponent_answer"
	} else {
		return fmt.Errorf("user %d is not a participant of duel %d", userID, duelID)
	}

	_, err = s.db.Exec(
		fmt.Sprintf("UPDATE duels SET %s = ? WHERE id = ?", column),
		answer, duelID,
	)
	if err != nil {
		return fmt.Errorf("submit duel answer: %w", err)
	}
	return nil
}

// CompleteDuel marks a duel as completed with a winner.
func (s *Storage) CompleteDuel(duelID, winnerID int64) error {
	_, err := s.db.Exec(
		"UPDATE duels SET status = 'completed', winner_id = ? WHERE id = ?",
		winnerID, duelID,
	)
	if err != nil {
		return fmt.Errorf("complete duel: %w", err)
	}
	return nil
}

// --- Group Quests ---

// GroupQuest represents a quest dropped in a group chat.
type GroupQuest struct {
	ID         int64
	ChatID     int64
	Quest      string
	AnswerHint string
	WinnerID   int64
	Status     string
	CreatedAt  time.Time
}

// CreateGroupQuest creates a new quest in a group chat.
func (s *Storage) CreateGroupQuest(chatID int64, quest, hint string) (int64, error) {
	// Complete any previous active quest first
	_, _ = s.db.Exec("UPDATE group_quests SET status = 'expired' WHERE chat_id = ? AND status = 'active'", chatID)

	res, err := s.db.Exec(
		"INSERT INTO group_quests (chat_id, quest, answer_hint) VALUES (?, ?, ?)",
		chatID, quest, hint,
	)
	if err != nil {
		return 0, fmt.Errorf("create group quest: %w", err)
	}
	return res.LastInsertId()
}

// GetActiveQuest returns the active quest for a group chat.
func (s *Storage) GetActiveQuest(chatID int64) (*GroupQuest, error) {
	var q GroupQuest
	var hint sql.NullString
	err := s.db.QueryRow(`
		SELECT id, chat_id, quest, answer_hint, COALESCE(winner_id, 0), status, created_at
		FROM group_quests
		WHERE chat_id = ? AND status = 'active'
		ORDER BY id DESC LIMIT 1`,
		chatID,
	).Scan(&q.ID, &q.ChatID, &q.Quest, &hint, &q.WinnerID, &q.Status, &q.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active quest: %w", err)
	}
	if hint.Valid {
		q.AnswerHint = hint.String
	}
	return &q, nil
}

// CompleteQuest marks a quest as completed with a winner.
func (s *Storage) CompleteQuest(questID, winnerID int64) error {
	_, err := s.db.Exec(
		"UPDATE group_quests SET status = 'completed', winner_id = ? WHERE id = ?",
		winnerID, questID,
	)
	if err != nil {
		return fmt.Errorf("complete quest: %w", err)
	}
	return nil
}

// --- Linked Users ---

// LinkedUser represents a link between two users.
type LinkedUser struct {
	UserA     int64
	UserB     int64
	Status    string
	CreatedAt time.Time
}

// CreateLink creates a pending link request between two users.
func (s *Storage) CreateLink(userA, userB int64) error {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO linked_users (user_a, user_b, status) VALUES (?, ?, 'pending')",
		userA, userB,
	)
	if err != nil {
		return fmt.Errorf("create link: %w", err)
	}
	return nil
}

// AcceptLink confirms a link by updating status to 'active'.
func (s *Storage) AcceptLink(userA, userB int64) error {
	_, err := s.db.Exec(
		"UPDATE linked_users SET status = 'active' WHERE user_a = ? AND user_b = ?",
		userA, userB,
	)
	if err != nil {
		return fmt.Errorf("accept link: %w", err)
	}
	return nil
}

// GetLinkedUser returns the linked partner's user ID. Checks both directions.
func (s *Storage) GetLinkedUser(userID int64) (int64, error) {
	var partnerID int64
	err := s.db.QueryRow(`
		SELECT CASE WHEN user_a = ? THEN user_b ELSE user_a END
		FROM linked_users
		WHERE (user_a = ? OR user_b = ?) AND status = 'active'
		LIMIT 1`,
		userID, userID, userID,
	).Scan(&partnerID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get linked user: %w", err)
	}
	return partnerID, nil
}

// GetPendingLink checks if userA sent a pending link request to userB.
func (s *Storage) GetPendingLink(userA, userB int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM linked_users WHERE user_a = ? AND user_b = ? AND status = 'pending'",
		userA, userB,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("get pending link: %w", err)
	}
	return count > 0, nil
}

// DeleteLink removes a link between two users (both directions).
func (s *Storage) DeleteLink(userA, userB int64) error {
	_, err := s.db.Exec(
		"DELETE FROM linked_users WHERE (user_a = ? AND user_b = ?) OR (user_a = ? AND user_b = ?)",
		userA, userB, userB, userA,
	)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}
	return nil
}

// GetAllActiveLinks returns all active links (for morning check notifications).
func (s *Storage) GetAllActiveLinks() ([]LinkedUser, error) {
	rows, err := s.db.Query("SELECT user_a, user_b, status, created_at FROM linked_users WHERE status = 'active'")
	if err != nil {
		return nil, fmt.Errorf("get all active links: %w", err)
	}
	defer rows.Close()

	var links []LinkedUser
	for rows.Next() {
		var l LinkedUser
		if err := rows.Scan(&l.UserA, &l.UserB, &l.Status, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan linked user: %w", err)
		}
		links = append(links, l)
	}
	return links, nil
}

// GetUserProfile returns the stored profile info for a user.
func (s *Storage) GetUserProfile(userID int64) (username, firstName, lastName string, err error) {
	err = s.db.QueryRow(
		"SELECT username, first_name, last_name FROM user_profiles WHERE user_id = ?",
		userID,
	).Scan(&username, &firstName, &lastName)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("get user profile: %w", err)
	}
	return username, firstName, lastName, nil
}

// FindUserByUsername looks up a user ID by their Telegram username.
func (s *Storage) FindUserByUsername(username string, userID *int64) error {
	err := s.db.QueryRow(
		"SELECT user_id FROM user_profiles WHERE LOWER(username) = LOWER(?)",
		username,
	).Scan(userID)
	if err == sql.ErrNoRows {
		*userID = 0
		return nil
	}
	if err != nil {
		return fmt.Errorf("find user by username %s: %w", username, err)
	}
	return nil
}

// GetLatestMoodScore returns the most recent mood score for a user (from today).
func (s *Storage) GetLatestMoodScore(userID int64) (int, error) {
	var score int
	err := s.db.QueryRow(`
		SELECT CAST(SUBSTR(text, 7, LENGTH(text) - 7) AS INTEGER)
		FROM messages
		WHERE user_id = ? AND role = 'user' AND text LIKE '[mood:%]'
			AND date(created_at) = date('now')
		ORDER BY id DESC LIMIT 1`,
		userID,
	).Scan(&score)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get latest mood score: %w", err)
	}
	return score, nil
}

// --- Runtime Config ---

// GetRuntimeConfig returns the value for a runtime config key.
// Returns empty string if not found.
func (s *Storage) GetRuntimeConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM runtime_config WHERE key = ?", key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get runtime config %s: %w", key, err)
	}
	return value, nil
}

// SetRuntimeConfig upserts a runtime config key/value pair.
func (s *Storage) SetRuntimeConfig(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO runtime_config (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set runtime config %s: %w", key, err)
	}
	return nil
}

// GetAllRuntimeConfig returns all runtime config entries as a map.
func (s *Storage) GetAllRuntimeConfig() (map[string]string, error) {
	rows, err := s.db.Query("SELECT key, value FROM runtime_config ORDER BY key")
	if err != nil {
		return nil, fmt.Errorf("get all runtime config: %w", err)
	}
	defer rows.Close()

	config := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan runtime config: %w", err)
		}
		config[key] = value
	}
	return config, nil
}

// DeleteRuntimeConfig removes a runtime config entry.
func (s *Storage) DeleteRuntimeConfig(key string) error {
	_, err := s.db.Exec("DELETE FROM runtime_config WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("delete runtime config %s: %w", key, err)
	}
	return nil
}

// GetRecentAutoMoods returns the last N auto_mood scores for a user (newest first).
// Auto mood scores are stored as "[auto_mood:N]" bot messages by the sentiment analyzer.
func (s *Storage) GetRecentAutoMoods(userID int64, limit int) ([]int, error) {
	rows, err := s.db.Query(`
		SELECT CAST(SUBSTR(text, 12, LENGTH(text) - 12) AS INTEGER) AS score
		FROM messages
		WHERE user_id = ? AND role = 'bot' AND text LIKE '[auto_mood:%]'
		ORDER BY id DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent auto moods: %w", err)
	}
	defer rows.Close()

	var scores []int
	for rows.Next() {
		var score int
		if err := rows.Scan(&score); err != nil {
			return nil, fmt.Errorf("scan auto mood: %w", err)
		}
		scores = append(scores, score)
	}
	return scores, nil
}

// GetRuntimeConfigInt returns the integer value for a runtime config key.
// Returns the provided defaultVal if the key is not found or cannot be parsed.
func (s *Storage) GetRuntimeConfigInt(key string, defaultVal int) int {
	val, err := s.GetRuntimeConfig(key)
	if err != nil || val == "" {
		return defaultVal
	}
	var result int
	if _, err := fmt.Sscanf(val, "%d", &result); err != nil {
		return defaultVal
	}
	return result
}
