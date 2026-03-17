package storage

import (
	"database/sql"
	"fmt"
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
