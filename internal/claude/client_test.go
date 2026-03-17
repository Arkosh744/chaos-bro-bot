package claude_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
)

func skipIfNoClaudeCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not found, skipping integration test")
	}
}

func TestClient_Ask_ReturnsNonEmpty(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 120*time.Second)
	reply, err := cl.Ask(context.Background(), "Reply in one word only. No explanation.", "Say hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("expected non-empty reply")
	}
	t.Logf("reply: %s", reply)
}

func TestClient_Ask_SystemPromptWorks(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 60*time.Second)
	reply, err := cl.Ask(context.Background(),
		"You are a pirate. Always reply with 'Arrr!'",
		"Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply == "" {
		t.Fatal("expected non-empty reply")
	}
	t.Logf("pirate reply: %s", reply)
}

func TestClient_Ask_TimeoutCancels(t *testing.T) {
	skipIfNoClaudeCLI(t)

	cl := claude.New("sonnet", 1*time.Millisecond)
	_, err := cl.Ask(context.Background(), "", "Hello")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	t.Logf("timeout error (expected): %v", err)
}
