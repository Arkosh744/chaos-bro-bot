package features_test

import (
	"strings"
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
)

func TestGetLevel(t *testing.T) {
	tests := []struct {
		name     string
		msgCount int
		wantLvl  int
		wantName string
	}{
		{"zero messages", 0, 1, "Незнакомец"},
		{"20 messages", 20, 1, "Незнакомец"},
		{"50 messages", 50, 2, "Знакомый"},
		{"150 messages", 150, 3, "Кореш"},
		{"300 messages", 300, 4, "Бро"},
		{"500 messages", 500, 5, "Кабан"},
		{"1000 messages", 1000, 6, "Легенда"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := features.GetLevel(tt.msgCount)
			if level.Level != tt.wantLvl {
				t.Errorf("GetLevel(%d).Level = %d, want %d", tt.msgCount, level.Level, tt.wantLvl)
			}
			if level.Name != tt.wantName {
				t.Errorf("GetLevel(%d).Name = %q, want %q", tt.msgCount, level.Name, tt.wantName)
			}
		})
	}
}

func TestGetLevel_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		msgCount int
		wantLvl  int
	}{
		{"exactly 21 -> level 2", 21, 2},
		{"exactly 51 -> level 3", 51, 3},
		{"exactly 151 -> level 4", 151, 4},
		{"exactly 301 -> level 5", 301, 5},
		{"exactly 501 -> level 6", 501, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := features.GetLevel(tt.msgCount)
			if level.Level != tt.wantLvl {
				t.Errorf("GetLevel(%d).Level = %d, want %d", tt.msgCount, level.Level, tt.wantLvl)
			}
		})
	}
}

func TestGetLevel_BelowBoundary(t *testing.T) {
	tests := []struct {
		msgCount int
		wantLvl  int
	}{
		{20, 1},  // just below 21
		{50, 2},  // just below 51
		{150, 3}, // just below 151
		{300, 4}, // just below 301
		{500, 5}, // just below 501
	}

	for _, tt := range tests {
		level := features.GetLevel(tt.msgCount)
		if level.Level != tt.wantLvl {
			t.Errorf("GetLevel(%d).Level = %d, want %d", tt.msgCount, level.Level, tt.wantLvl)
		}
	}
}

func TestGetNextLevel(t *testing.T) {
	lvl1 := features.GetLevel(0)
	next := features.GetNextLevel(lvl1)
	if next == nil {
		t.Fatal("expected next level for level 1")
	}
	if next.Level != 2 {
		t.Errorf("expected next level 2, got %d", next.Level)
	}

	// Max level should return nil
	maxLvl := features.GetLevel(1000)
	if maxLvl.Level != 6 {
		t.Fatalf("expected max level 6, got %d", maxLvl.Level)
	}
	nextMax := features.GetNextLevel(maxLvl)
	if nextMax != nil {
		t.Errorf("expected nil for max level next, got level %d", nextMax.Level)
	}
}

func TestLevelUpMessage(t *testing.T) {
	for lvl := 2; lvl <= 6; lvl++ {
		level := features.GetLevel(levelThreshold(lvl))
		msg := features.LevelUpMessage(level)
		if msg == "" {
			t.Errorf("LevelUpMessage for level %d returned empty string", lvl)
		}
		if !strings.Contains(msg, level.Name) {
			t.Errorf("LevelUpMessage for level %d does not contain level name %q: %s", lvl, level.Name, msg)
		}
		if !strings.Contains(msg, level.Emoji) {
			t.Errorf("LevelUpMessage for level %d does not contain emoji %q: %s", lvl, level.Emoji, msg)
		}
	}
}

func TestLevelUpMessage_Level1_Empty(t *testing.T) {
	level := features.GetLevel(0)
	msg := features.LevelUpMessage(level)
	if msg != "" {
		t.Errorf("expected empty LevelUpMessage for level 1, got: %s", msg)
	}
}

func TestLevelPromptSuffix(t *testing.T) {
	for lvl := 1; lvl <= 6; lvl++ {
		level := features.GetLevel(levelThreshold(lvl))
		suffix := features.LevelPromptSuffix(level)
		if suffix == "" {
			t.Errorf("LevelPromptSuffix for level %d returned empty string", lvl)
		}
		if !strings.Contains(suffix, level.Name) {
			t.Errorf("LevelPromptSuffix for level %d missing level name: %s", lvl, suffix)
		}
		if !strings.Contains(suffix, level.Emoji) {
			t.Errorf("LevelPromptSuffix for level %d missing emoji: %s", lvl, suffix)
		}
		if !strings.Contains(suffix, level.Suffix) {
			t.Errorf("LevelPromptSuffix for level %d missing suffix text", lvl)
		}
	}
}

func TestFormatLevelStatus(t *testing.T) {
	status := features.FormatLevelStatus(100)
	if status == "" {
		t.Fatal("expected non-empty status")
	}
	if !strings.Contains(status, "Кореш") {
		t.Errorf("expected Кореш in status for 100 msgs, got: %s", status)
	}
	if !strings.Contains(status, "100") {
		t.Errorf("expected message count in status: %s", status)
	}
}

func TestFormatLevelStatus_MaxLevel(t *testing.T) {
	status := features.FormatLevelStatus(1000)
	if !strings.Contains(status, "Максимальный уровень") {
		t.Errorf("expected max level message, got: %s", status)
	}
}

// levelThreshold returns a message count that maps to the given level.
func levelThreshold(lvl int) int {
	thresholds := map[int]int{1: 0, 2: 21, 3: 51, 4: 151, 5: 301, 6: 501}
	return thresholds[lvl]
}
