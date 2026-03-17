package features_test

import (
	"strings"
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
)

func TestParseLieResponse_Valid(t *testing.T) {
	input := "ЛОЖЬ|Кошки на самом деле умеют летать\nПРАВДА|Конечно нет, это бред"
	lie, truth, err := features.ParseLieResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lie != "Кошки на самом деле умеют летать" {
		t.Errorf("unexpected lie: %q", lie)
	}
	if truth != "Конечно нет, это бред" {
		t.Errorf("unexpected truth: %q", truth)
	}
}

func TestParseLieResponse_WithWhitespace(t *testing.T) {
	input := "  ЛОЖЬ|  Факт с пробелами  \n  ПРАВДА|  Объяснение с пробелами  "
	lie, truth, err := features.ParseLieResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lie != "Факт с пробелами" {
		t.Errorf("unexpected lie: %q", lie)
	}
	if truth != "Объяснение с пробелами" {
		t.Errorf("unexpected truth: %q", truth)
	}
}

func TestParseLieResponse_Invalid_Empty(t *testing.T) {
	_, _, err := features.ParseLieResponse("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParseLieResponse_Invalid_NoFormat(t *testing.T) {
	_, _, err := features.ParseLieResponse("just some random text without format")
	if err == nil {
		t.Error("expected error for unformatted input")
	}
}

func TestParseLieResponse_Invalid_OnlyLie(t *testing.T) {
	_, _, err := features.ParseLieResponse("ЛОЖЬ|something\nno truth here")
	if err == nil {
		t.Error("expected error when truth is missing")
	}
}

func TestParseLieResponse_Invalid_OnlyTruth(t *testing.T) {
	_, _, err := features.ParseLieResponse("no lie here\nПРАВДА|something")
	if err == nil {
		t.Error("expected error when lie is missing")
	}
}

func TestInjectLie(t *testing.T) {
	result := features.InjectLie("Основной ответ", "Кошки правят миром")
	if !strings.HasPrefix(result, "Основной ответ") {
		t.Errorf("expected reply to start with original text, got: %s", result)
	}
	if !strings.Contains(result, "Кстати, ") {
		t.Errorf("expected 'Кстати, ' prefix for lie, got: %s", result)
	}
	// Should lowercase the first letter
	if !strings.Contains(result, "Кстати, кошки правят миром") {
		t.Errorf("expected lowercased lie after 'Кстати, ', got: %s", result)
	}
}

func TestInjectLie_EnglishFirstLetter(t *testing.T) {
	result := features.InjectLie("Reply", "Something interesting")
	if !strings.Contains(result, "Кстати, something interesting") {
		t.Errorf("expected English first letter lowercased, got: %s", result)
	}
}

func TestInjectLie_AlreadyLowercase(t *testing.T) {
	result := features.InjectLie("Ответ", "уже маленькая буква")
	if !strings.Contains(result, "Кстати, уже маленькая буква") {
		t.Errorf("expected unchanged lowercase, got: %s", result)
	}
}
