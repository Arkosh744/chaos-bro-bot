package features_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func skipIfNoClaudeCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not found, skipping integration test")
	}
}

func hasRussian(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// --- Grounding ---

func TestRandomGrounding_ReturnsNonEmpty(t *testing.T) {
	g := features.RandomGrounding()
	if g == "" {
		t.Fatal("expected non-empty grounding")
	}
	if !hasRussian(g) {
		t.Errorf("expected Russian text, got: %s", g)
	}
}

func TestRandomGrounding_Varies(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		seen[features.RandomGrounding()] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected variety, got only %d unique results", len(seen))
	}
}

// --- Chaos ---

func TestRandomChaos_ReturnsNonEmpty(t *testing.T) {
	c := features.RandomChaos()
	if c == "" {
		t.Fatal("expected non-empty chaos task")
	}
	if !hasRussian(c) {
		t.Errorf("expected Russian text, got: %s", c)
	}
}

func TestRandomChaos_Varies(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		seen[features.RandomChaos()] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected variety, got only %d unique results", len(seen))
	}
}

func TestGenerateChaos_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	task, err := features.GenerateChaos(context.Background(), cl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task == "" {
		t.Fatal("expected non-empty chaos task")
	}
	if len(task) < 5 {
		t.Errorf("chaos task too short: %q", task)
	}
	t.Logf("chaos: %s", task)
}

// --- Trickster ---

func TestTricksterReply_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := features.TricksterReply(context.Background(), cl, "Привет", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("expected non-empty trickster reply")
	}
	if !hasRussian(reply) {
		t.Errorf("expected Russian reply, got: %s", reply)
	}
	t.Logf("trickster: %s", reply)
}

func TestTricksterReply_NotTherapist(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := features.TricksterReply(context.Background(), cl, "Мне плохо, устал от всего", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	banned := []string{"я тебя понимаю", "всё будет хорошо", "ты справишься"}
	lower := strings.ToLower(reply)
	for _, phrase := range banned {
		if strings.Contains(lower, phrase) {
			t.Errorf("reply contains banned therapist phrase %q: %s", phrase, reply)
		}
	}
	t.Logf("complaint reply: %s", reply)
}

func TestTricksterReply_Short(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := features.TricksterReply(context.Background(), cl, "Как дела?", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be 1-3 sentences, roughly under 500 chars
	if len(reply) > 500 {
		t.Errorf("reply too long (%d chars), expected short answer: %s", len(reply), reply)
	}
	t.Logf("short reply (%d chars): %s", len(reply), reply)
}

// --- Randomizer ---

func TestDecide_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := features.Decide(context.Background(), cl, "Пойти в зал или остаться дома?", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("expected non-empty decision")
	}
	if !hasRussian(reply) {
		t.Errorf("expected Russian reply, got: %s", reply)
	}

	// Should be decisive, not wishy-washy
	wishy := []string{"зависит от", "с одной стороны", "it depends"}
	lower := strings.ToLower(reply)
	for _, phrase := range wishy {
		if strings.Contains(lower, phrase) {
			t.Errorf("reply is wishy-washy (%q found): %s", phrase, reply)
		}
	}
	t.Logf("decision: %s", reply)
}

func TestDecide_Short(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := features.Decide(context.Background(), cl, "Что поесть?", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reply) > 300 {
		t.Errorf("decision too long (%d chars): %s", len(reply), reply)
	}
	t.Logf("short decision (%d chars): %s", len(reply), reply)
}

// --- Memory & Quotes ---

func TestUpdateSummary_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed 25 messages to trigger summary update
	for i := range 25 {
		role := "user"
		if i%2 == 1 {
			role = "bot"
		}
		msgs := []string{
			"Устал на работе", "Закрой ноут", "Хочу в зал", "Ну сходи",
			"Опять дождь", "Возьми зонт", "Может шаурму?", "Дерзай",
			"Лень что-то делать", "Ну и сиди", "Как дела?", "Нормально",
			"Скучно", "Почитай книгу", "Хочу кота", "Заведи",
			"Опять работа", "Терпи", "Сходил в зал!", "Красава",
			"Устал", "Поспи", "Что поесть?", "Шаурму",
			"Спасибо", "Не за что", "Пока", "Удачи",
		}
		text := msgs[i%len(msgs)]
		if _, err := store.SaveMessage(123, role, text); err != nil {
			t.Fatal(err)
		}
	}

	cl := claude.New("sonnet", 120*time.Second)
	if err := features.UpdateSummary(context.Background(), cl, store, 123); err != nil {
		t.Fatalf("UpdateSummary error: %v", err)
	}

	summary, lastID, err := store.GetSummary(123)
	if err != nil {
		t.Fatal(err)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if lastID == 0 {
		t.Error("expected lastID > 0")
	}
	if !hasRussian(summary) {
		t.Errorf("expected Russian summary, got: %s", summary)
	}
	t.Logf("summary (%d chars): %s", len(summary), summary)
}

func TestGenerateQuote_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	quote, err := features.GenerateQuote(context.Background(), cl, nil)
	if err != nil {
		t.Fatalf("GenerateQuote error: %v", err)
	}
	if quote == "" {
		t.Fatal("expected non-empty quote")
	}
	// Quote should contain dash separator (quote — character, game)
	if !strings.Contains(quote, "—") && !strings.Contains(quote, "-") {
		t.Errorf("expected quote with dash separator, got: %s", quote)
	}
	t.Logf("quote: %s", quote)
}

func TestGenerateQuote_AvoidsRepeats(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	recent := []string{
		`"Работа-работа..." — Пеон, Warcraft III`,
		`"My life for Aiur!" — Zealot, StarCraft`,
	}
	quote, err := features.GenerateQuote(context.Background(), cl, recent)
	if err != nil {
		t.Fatalf("GenerateQuote error: %v", err)
	}
	for _, r := range recent {
		if quote == r {
			t.Errorf("quote repeated a recent one: %s", quote)
		}
	}
	t.Logf("non-repeated quote: %s", quote)
}

func TestContextAffectsReply_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)

	// Reply without context
	reply1, err := features.TricksterReply(context.Background(), cl, "Привет", "")
	if err != nil {
		t.Fatalf("reply without context: %v", err)
	}

	// Reply WITH context about cats
	catCtx := "## What you know about this person\nПользователь обожает котов. У него кот по имени Барсик. Постоянно скидывает фотки кота."
	reply2, err := features.TricksterReply(context.Background(), cl, "Привет", catCtx)
	if err != nil {
		t.Fatalf("reply with context: %v", err)
	}

	t.Logf("without context: %s", reply1)
	t.Logf("with cat context: %s", reply2)
	// We can't deterministically assert the reply mentions cats,
	// but we verify both work without error
}

func TestDetectPatterns_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed messages with a pattern (repeated complaints about work)
	for i := range 20 {
		role := "user"
		if i%2 == 1 {
			role = "bot"
		}
		texts := []string{
			"Устал на работе", "Бывает", "Опять работа", "Терпи",
			"Достала эта работа", "Уволься", "Ненавижу работу", "Ну ты и нытик",
			"Работа задолбала", "Кофе попей", "Заебала работа", "Понял",
		}
		store.SaveMessage(123, role, texts[i%len(texts)])
	}

	// Update summary first
	cl := claude.New("sonnet", 120*time.Second)
	if err := features.UpdateSummary(context.Background(), cl, store, 123); err != nil {
		t.Fatalf("update summary: %v", err)
	}

	pattern, err := features.DetectPatterns(context.Background(), cl, store, 123)
	if err != nil {
		t.Fatalf("detect patterns: %v", err)
	}
	// Should detect work complaints pattern
	t.Logf("pattern result: %q", pattern)
	// Pattern can be empty or non-empty, but should not error
}

func TestGenerateDigest_Integration(t *testing.T) {
	skipIfNoClaudeCLI(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for i := range 15 {
		role := "user"
		if i%2 == 1 {
			role = "bot"
		}
		store.SaveMessage(123, role, fmt.Sprintf("message %d", i))
	}

	cl := claude.New("sonnet", 120*time.Second)
	features.UpdateSummary(context.Background(), cl, store, 123)

	digest, err := features.GenerateDigest(context.Background(), cl, store, 123)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}
	if digest == "" {
		t.Fatal("expected non-empty digest")
	}
	t.Logf("digest: %s", digest)
}

func TestDayOfWeekMood(t *testing.T) {
	mood := features.DayOfWeekMood()
	if mood == "" {
		t.Error("expected non-empty day of week mood")
	}
	if !hasRussian(mood) {
		t.Errorf("expected Russian text, got: %s", mood)
	}
}

func TestEasterEggs_EmojiKeys(t *testing.T) {
	emojiKeys := []string{
		"\u2764\uFE0F", "\U0001F480", "\U0001F44D", "\U0001F525",
		"\U0001F602", "\U0001F914", "\U0001F62D", "\U0001F921",
		"\U0001F4A9", "\U0001F440", "\U0001F64F",
	}
	for _, key := range emojiKeys {
		if _, ok := features.EasterEggs[key]; !ok {
			t.Errorf("missing emoji easter egg key: %q", key)
		}
	}
}

func TestRandomLoot_Varies(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 30; i++ {
		seen[features.RandomLoot()] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected variety in loot, got only %d unique results", len(seen))
	}
}

func TestRandomFallback_Varies(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 30; i++ {
		seen[features.RandomFallback()] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected variety in fallbacks, got only %d unique results", len(seen))
	}
}
