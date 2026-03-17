package features_test

import (
	"strings"
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func TestFormatProfile_Empty(t *testing.T) {
	result := features.FormatProfile(nil)
	if !strings.Contains(result, "Пока ничего не знаю") {
		t.Errorf("expected 'Пока ничего не знаю' for nil facts, got: %s", result)
	}

	result = features.FormatProfile([]storage.UserFact{})
	if !strings.Contains(result, "Пока ничего не знаю") {
		t.Errorf("expected 'Пока ничего не знаю' for empty facts, got: %s", result)
	}
}

func TestFormatProfile_WithFacts(t *testing.T) {
	facts := []storage.UserFact{
		{Category: "name", Fact: "Иван"},
		{Category: "city", Fact: "Москва"},
		{Category: "job", Fact: "Go разработчик"},
	}
	result := features.FormatProfile(facts)

	if !strings.Contains(result, "Твой профиль") {
		t.Errorf("expected profile header, got: %s", result)
	}
	if !strings.Contains(result, "Иван") {
		t.Errorf("expected name fact, got: %s", result)
	}
	if !strings.Contains(result, "Москва") {
		t.Errorf("expected city fact, got: %s", result)
	}
	if !strings.Contains(result, "Go разработчик") {
		t.Errorf("expected job fact, got: %s", result)
	}
	// Should have emoji labels
	if !strings.Contains(result, "👤") {
		t.Errorf("expected name emoji label, got: %s", result)
	}
	if !strings.Contains(result, "📍") {
		t.Errorf("expected city emoji label, got: %s", result)
	}
	if !strings.Contains(result, "💼") {
		t.Errorf("expected job emoji label, got: %s", result)
	}
	if !strings.Contains(result, "Собрано из наших разговоров") {
		t.Errorf("expected footer, got: %s", result)
	}
}

func TestFormatProfile_UnknownCategory(t *testing.T) {
	facts := []storage.UserFact{
		{Category: "unknown_category", Fact: "some fact"},
	}
	result := features.FormatProfile(facts)
	// Unknown category should fallback to category name as label
	if !strings.Contains(result, "unknown_category") {
		t.Errorf("expected unknown category as fallback label, got: %s", result)
	}
	if !strings.Contains(result, "some fact") {
		t.Errorf("expected fact text, got: %s", result)
	}
}

func TestCategoryLabels(t *testing.T) {
	expectedCategories := []string{
		"name", "age", "city", "job", "hobbies", "music",
		"games", "food", "mood_pattern", "relationships", "pets", "goals", "quirks",
	}

	for _, cat := range expectedCategories {
		facts := []storage.UserFact{{Category: cat, Fact: "test"}}
		result := features.FormatProfile(facts)
		// Should NOT contain the raw category key as the label (means it was mapped)
		// except if we can't verify the exact label. Instead verify it formats without error.
		if result == "" {
			t.Errorf("empty result for category %q", cat)
		}
		// Verify it does not fall back to raw category name
		if strings.Contains(result, cat+": test") {
			t.Errorf("category %q was not mapped to a label (raw key used)", cat)
		}
	}
}
