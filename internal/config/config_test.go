package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/config"
)

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.local.yaml")
	content := `
telegram:
  token: "test-token-123"
  owner_id: 12345
claude:
  model: "haiku"
  timeout: 10s
scheduler:
  enabled: true
  min_hour: 10
  max_hour: 20
  pings_per_day: 3
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Telegram.Token != "test-token-123" {
		t.Errorf("token = %q, want test-token-123", cfg.Telegram.Token)
	}
	if cfg.Telegram.OwnerID != 12345 {
		t.Errorf("owner_id = %d, want 12345", cfg.Telegram.OwnerID)
	}
	if cfg.Claude.Model != "haiku" {
		t.Errorf("model = %q, want haiku", cfg.Claude.Model)
	}
	if cfg.Claude.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", cfg.Claude.Timeout)
	}
	if !cfg.Scheduler.Enabled {
		t.Error("scheduler should be enabled")
	}
	if cfg.Scheduler.MinHour != 10 {
		t.Errorf("min_hour = %d, want 10", cfg.Scheduler.MinHour)
	}
}

func TestLoad_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
telegram:
  token: ""
claude:
  model: ""
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("TELEGRAM_TOKEN", "env-token-456")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telegram.Token != "env-token-456" {
		t.Errorf("token = %q, want env-token-456", cfg.Telegram.Token)
	}
	if cfg.Claude.Model != "sonnet" {
		t.Errorf("model = %q, want sonnet (default)", cfg.Claude.Model)
	}
}

func TestLoad_NoConfig(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when no config exists")
	}
}

func TestLoad_NoToken(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
telegram:
  token: ""
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when token is empty")
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
telegram:
  token: "${MY_BOT_TOKEN}"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("MY_BOT_TOKEN", "expanded-token")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Telegram.Token != "expanded-token" {
		t.Errorf("token = %q, want expanded-token", cfg.Telegram.Token)
	}
}
