package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// EdgeTTS wraps the edge-tts Python CLI for text-to-speech synthesis.
type EdgeTTS struct {
	voice string
}

// New creates an EdgeTTS instance with the given voice.
// Falls back to "ru-RU-DmitryNeural" if voice is empty.
func New(voice string) *EdgeTTS {
	if voice == "" {
		voice = "ru-RU-DmitryNeural"
	}
	return &EdgeTTS{voice: voice}
}

// Available checks if edge-tts CLI is installed and reachable.
func (t *EdgeTTS) Available() bool {
	_, err := exec.LookPath("edge-tts")
	return err == nil
}

// Synthesize converts text to an .mp3 file. Returns path to a temp file.
// Caller is responsible for removing the file after use.
func (t *EdgeTTS) Synthesize(ctx context.Context, text string) (string, error) {
	tmpFile, err := os.CreateTemp("", "tts-*.mp3")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, "edge-tts",
		"--voice", t.voice,
		"--text", text,
		"--write-media", tmpFile.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("edge-tts: %w (output: %s)", err, string(out))
	}

	return tmpFile.Name(), nil
}
