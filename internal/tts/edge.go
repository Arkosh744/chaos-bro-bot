package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type EdgeTTS struct {
	voice string
}

func New(voice string) *EdgeTTS {
	if voice == "" {
		voice = "ru-RU-DmitryNeural"
	}
	return &EdgeTTS{voice: voice}
}

func (t *EdgeTTS) Available() bool {
	_, err := exec.LookPath("edge-tts")
	return err == nil
}

// Caller is responsible for removing the returned temp file.
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
