package features_test

import (
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
)

func TestIsSleepTime(t *testing.T) {
	// Can't test time-dependent, but verify function doesn't panic
	_ = features.IsSleepTime()
}

func TestTimeOfDayMood(t *testing.T) {
	mood := features.TimeOfDayMood()
	if mood == "" {
		t.Error("expected non-empty mood")
	}
}

func TestRandomFallback(t *testing.T) {
	f := features.RandomFallback()
	if f == "" {
		t.Error("expected non-empty fallback")
	}
}

func TestRandomLoot(t *testing.T) {
	l := features.RandomLoot()
	if l == "" {
		t.Error("expected non-empty loot")
	}
}

func TestEasterEggs(t *testing.T) {
	if len(features.EasterEggs) == 0 {
		t.Error("expected non-empty easter eggs")
	}
	if _, ok := features.EasterEggs["зуг-зуг"]; !ok {
		t.Error("expected зуг-зуг easter egg")
	}
	if _, ok := features.EasterEggs["42"]; !ok {
		t.Error("expected 42 easter egg")
	}
}

func TestAlterEgo(t *testing.T) {
	// AlterEgoPromptSuffix should not panic
	_ = features.AlterEgoPromptSuffix()

	// AlterEgos should have entries
	if len(features.AlterEgos) == 0 {
		t.Error("expected non-empty alter egos")
	}
}

func TestBreathingCycle(t *testing.T) {
	if len(features.BreathingCycle) == 0 {
		t.Error("expected non-empty breathing cycle")
	}
	if features.BreathingRounds < 1 {
		t.Error("expected at least 1 round")
	}
}
