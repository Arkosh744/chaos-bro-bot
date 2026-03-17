package claude

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	model   string
	timeout time.Duration
}

func New(model string, timeout time.Duration) *Client {
	return &Client{
		model:   model,
		timeout: timeout,
	}
}

func (c *Client) Ask(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{
		"-p",
		"--model", c.model,
		"--output-format", "text",
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
		return "", fmt.Errorf("claude -p: %w (stderr: %s)", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
