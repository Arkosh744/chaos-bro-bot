package storage_test

import (
	"path/filepath"
	"testing"

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

func newTestDB(t *testing.T) *storage.Storage {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
