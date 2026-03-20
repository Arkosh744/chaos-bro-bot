package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// sanitizeTTSText strips shell metacharacters and limits length
// to prevent command injection via edge-tts arguments.
func sanitizeTTSText(text string) string {
	if len(text) > 500 {
		text = text[:500]
	}
	replacer := strings.NewReplacer(
		"`", "", "$", "", "\\", "",
		"(", "", ")", "", "{", "", "}", "",
		";", ",", "&", "", "|", "",
		"<", "", ">", "",
	)
	return replacer.Replace(text)
}

// Caller is responsible for removing the returned temp file.
func (t *EdgeTTS) Synthesize(ctx context.Context, text string) (string, error) {
	text = sanitizeTTSText(text)
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
