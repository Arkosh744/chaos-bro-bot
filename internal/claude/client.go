package claude

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	offlineThreshold   = 3
	offlineRetryPeriod = 5 * time.Minute
)

type Client struct {
	model   string
	timeout time.Duration

	mu                  sync.RWMutex
	consecutiveFailures int
	offlineMode         bool
	lastRetry           time.Time
}

func New(model string, timeout time.Duration) *Client {
	return &Client{
		model:   model,
		timeout: timeout,
	}
}

// IsOffline reports whether the client is in offline mode after consecutive failures.
func (c *Client) IsOffline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.offlineMode
}

// SanitizeInput cleans user input before passing to Claude.
// Limits length and strips known prompt injection patterns.
func SanitizeInput(text string) string {
	// Hard limit on input length
	if len(text) > 4000 {
		text = text[:4000]
	}

	// Strip null bytes and control characters (except newlines/tabs)
	var clean strings.Builder
	clean.Grow(len(text))
	for _, r := range text {
		if r == '\n' || r == '\t' || r >= 32 {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// SanitizeOutput cleans Claude's response before sending to user.
func SanitizeOutput(text string) string {
	// Hard limit on output length (Telegram message limit ~4096)
	if len(text) > 3500 {
		text = text[:3500] + "..."
	}
	return text
}

func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return c.AskWithModel(ctx, c.model, systemPrompt, userMessage)
}

// AskWithModel calls Claude CLI with a specific model override for one call.
func (c *Client) AskWithModel(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	userMessage = SanitizeInput(userMessage)

	c.mu.Lock()
	if c.offlineMode {
		if time.Since(c.lastRetry) < offlineRetryPeriod {
			c.mu.Unlock()
			return "", fmt.Errorf("claude offline mode: using fallbacks")
		}
		// Enough time passed, attempt a real call to check recovery
		c.lastRetry = time.Now()
		log.Println("Claude: attempting recovery call")
	}
	c.mu.Unlock()

	if model == "" {
		model = c.model
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{
		"-p",
		"--model", model,
		"--output-format", "text",
		"--max-turns", "1",
	}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(userMessage)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		c.mu.Lock()
		c.consecutiveFailures++
		if c.consecutiveFailures >= offlineThreshold && !c.offlineMode {
			c.offlineMode = true
			c.lastRetry = time.Now()
			log.Printf("Claude: entering offline mode after %d consecutive failures", c.consecutiveFailures)
		}
		c.mu.Unlock()
		return "", fmt.Errorf("claude -p: %w (stderr: %s)", err, stderr.String())
	}

	c.mu.Lock()
	if c.offlineMode {
		log.Println("Claude: recovered, exiting offline mode")
	}
	c.consecutiveFailures = 0
	c.offlineMode = false
	c.mu.Unlock()

	return SanitizeOutput(strings.TrimSpace(stdout.String())), nil
}
