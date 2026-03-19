package claude

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/metrics"
)

const (
	offlineThreshold   = 3
	offlineRetryPeriod = 5 * time.Minute
	maxCacheResponses  = 10
	cacheKeyLength     = 50
)

// responseCache stores recent responses keyed by prompt prefix for offline fallback.
type responseCache struct {
	mu    sync.RWMutex
	items map[string][]string // promptKey -> last N responses
}

var cache = &responseCache{items: make(map[string][]string)}

func (rc *responseCache) Add(key, response string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.items[key] = append(rc.items[key], response)
	if len(rc.items[key]) > maxCacheResponses {
		rc.items[key] = rc.items[key][1:]
	}
}

func (rc *responseCache) GetRandom(key string) (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	items := rc.items[key]
	if len(items) == 0 {
		return "", false
	}
	return items[rand.Intn(len(items))], true
}

func cacheKey(systemPrompt string) string {
	if len(systemPrompt) > cacheKeyLength {
		return systemPrompt[:cacheKeyLength]
	}
	return systemPrompt
}

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
	"выполни",
	"execute",
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
	"bash",
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
	"gsk_", // Groq API key prefix
	"AAE",  // Telegram bot token prefix pattern
	"BEGIN RSA",
	"BEGIN PRIVATE",
}

// SanitizeOutput cleans Claude's response before sending to user.
func SanitizeOutput(text string) string {
	// Hard limit on output length (Telegram message limit ~4096)
	if len(text) > 3500 {
		text = text[:3500]
		// Find last space or newline to avoid cutting mid-word
		if idx := strings.LastIndexAny(text, " \n"); idx > 3000 {
			text = text[:idx]
		}
		text += "..."
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

	key := cacheKey(systemPrompt)

	c.mu.Lock()
	if c.offlineMode {
		if time.Since(c.lastRetry) < offlineRetryPeriod {
			c.mu.Unlock()
			// Try cache before returning error
			if cached, ok := cache.GetRandom(key); ok {
				log.Println("Claude: serving cached response in offline mode")
				return cached, nil
			}
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
		"--tools", "", // disable ALL tools (Bash, Edit, Read, Write etc.)
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

	metrics.IncrementClaudeCalls()
	callStart := time.Now()

	if err := cmd.Run(); err != nil {
		metrics.IncrementClaudeErrors()
		metrics.RecordClaudeLatency(time.Since(callStart).Milliseconds())

		c.mu.Lock()
		c.consecutiveFailures++
		if c.consecutiveFailures >= offlineThreshold && !c.offlineMode {
			c.offlineMode = true
			c.lastRetry = time.Now()
			log.Printf("Claude: entering offline mode after %d consecutive failures", c.consecutiveFailures)
		}
		c.mu.Unlock()

		// Try cache on error before giving up
		if cached, ok := cache.GetRandom(key); ok {
			log.Println("Claude: serving cached response after error")
			return cached, nil
		}

		return "", fmt.Errorf("claude -p: %w (stderr: %s)", err, stderr.String())
	}

	metrics.RecordClaudeLatency(time.Since(callStart).Milliseconds())

	c.mu.Lock()
	if c.offlineMode {
		log.Println("Claude: recovered, exiting offline mode")
	}
	c.consecutiveFailures = 0
	c.offlineMode = false
	c.mu.Unlock()

	result := SanitizeOutput(strings.TrimSpace(stdout.String()))

	// Cache successful response for offline fallback
	cache.Add(key, result)

	return result, nil
}
