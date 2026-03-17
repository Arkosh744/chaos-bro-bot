package features_test

import (
	"strings"
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func TestBuildContext_Empty(t *testing.T) {
	ctx := features.BuildContext("", nil)
	if ctx != "" {
		t.Errorf("expected empty, got: %q", ctx)
	}
}

func TestBuildContext_WithSummary(t *testing.T) {
	ctx := features.BuildContext("user likes cats", nil)
	if !strings.Contains(ctx, "user likes cats") {
		t.Errorf("missing summary in context: %q", ctx)
	}
	if !strings.Contains(ctx, "What you know") {
		t.Errorf("missing header in context: %q", ctx)
	}
}

func TestBuildContext_WithMessages(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "hello"},
		{Role: "bot", Text: "yo"},
	}
	ctx := features.BuildContext("", msgs)
	if !strings.Contains(ctx, "User: hello") || !strings.Contains(ctx, "Bot: yo") {
		t.Errorf("missing messages in context: %q", ctx)
	}
}

func TestBuildContext_Full(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "test"},
	}
	ctx := features.BuildContext("summary here", msgs)
	if !strings.Contains(ctx, "summary here") || !strings.Contains(ctx, "User: test") {
		t.Errorf("context missing parts: %q", ctx)
	}
}
