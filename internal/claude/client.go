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

// dangerousPatterns are phrases that indicate prompt injection attempts
// to make Claude execute system commands or access files.
var dangerousPatterns = []string{
	"выполни команду",
	"execute command",
	"run command",
	"запусти команду",
	"system(",
	"os.exec",
	"subprocess",
	"exec(",
	"/bin/sh",
	"/bin/bash",
	"rm -rf",
	"rm -f",
	"sudo ",
	"chmod ",
	"curl ",
	"wget ",
	"cat /etc",
	"DROP TABLE",
	"DELETE FROM",
	"; rm ",
	"&& rm",
	"| rm",
	"`rm",
	"$(rm",
	"используй bash",
	"use bash",
	"use the terminal",
	"open a shell",
	"access the file",
	"read the file",
	"write to file",
	"create a file",
	"удали файл",
	"прочитай файл",
	"запиши в файл",
	"открой терминал",
}

// IsDangerousInput checks if user text contains prompt injection patterns
// attempting to make Claude execute commands or access the filesystem.
func IsDangerousInput(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// SanitizeInput cleans user input before passing to Claude.
// Limits length and strips control characters.
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

// outputDangerousPatterns are patterns in Claude output that suggest
// it tried to execute commands or leak system info.
var outputDangerousPatterns = []string{
	"```bash",
	"```shell",
	"```sh",
	"$ rm ",
	"$ sudo",
	"TELEGRAM_TOKEN",
	"api_key:",
	"auth_token:",
	"gsk_",           // Groq API key prefix
	"AAE",            // Telegram bot token prefix pattern
	"BEGIN RSA",
	"BEGIN PRIVATE",
}

// SanitizeOutput cleans Claude's response before sending to user.
func SanitizeOutput(text string) string {
	// Hard limit on output length (Telegram message limit ~4096)
	if len(text) > 3500 {
		text = text[:3500] + "..."
	}

	// Check for leaked secrets or command output in response
	lower := strings.ToLower(text)
	for _, p := range outputDangerousPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			log.Printf("Claude: sanitized dangerous output pattern: %s", p)
			return "Хм, что-то пошло не так. Попробуй ещё раз."
		}
	}

	return text
}

func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return c.AskWithModel(ctx, c.model, systemPrompt, userMessage)
}

// AskWithModel calls Claude CLI with a specific model override for one call.
func (c *Client) AskWithModel(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	userMessage = SanitizeInput(userMessage)

	// Block dangerous prompt injection attempts before they reach Claude
	if IsDangerousInput(userMessage) {
		log.Printf("Claude: blocked dangerous input: %.100s", userMessage)
		return "", fmt.Errorf("blocked: dangerous input detected")
	}

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
		"--tools", "",               // disable ALL tools (Bash, Edit, Read, Write etc.)
		"--permission-mode", "plan", // read-only mode as extra safety layer
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
