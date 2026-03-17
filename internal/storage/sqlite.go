package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID        int64
	UserID    int64
	Role      string // "user" or "bot"
	Text      string
	CreatedAt time.Time
}

type Storage struct {
	db *sql.DB
}

func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) migrate() error {
	_, err := s.db.Exec(`
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
	`)
	return err
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

// UserInfo represents a user with aggregate message stats.
type UserInfo struct {
	UserID       int64
	MessageCount int
	LastMessage  time.Time
}

// GetAllUsers returns all users who have sent or received messages, ordered by last activity.
func (s *Storage) GetAllUsers() ([]UserInfo, error) {
	rows, err := s.db.Query(`
		SELECT user_id, COUNT(*) AS msg_count, COALESCE(MAX(created_at), '') AS last_msg
		FROM messages
		GROUP BY user_id
		ORDER BY last_msg DESC`)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
	}
	defer rows.Close()

	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		var lastMsg string
		if err := rows.Scan(&u.UserID, &u.MessageCount, &lastMsg); err != nil {
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
