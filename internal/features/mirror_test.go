package features_test

import (
	"strings"
	"testing"

	"github.com/Arkosh744/chaos-bro-bot/internal/features"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func TestAnalyzeStyle_Empty(t *testing.T) {
	result := features.AnalyzeStyle(nil)
	if result == "" {
		t.Fatal("expected non-empty result for nil messages")
	}
	if !strings.Contains(result, "Недостаточно данных") {
		t.Errorf("expected 'Недостаточно данных' message, got: %s", result)
	}

	// Empty slice
	result = features.AnalyzeStyle([]storage.Message{})
	if !strings.Contains(result, "Недостаточно данных") {
		t.Errorf("expected 'Недостаточно данных' for empty slice, got: %s", result)
	}
}

func TestAnalyzeStyle_OnlyBotMessages(t *testing.T) {
	msgs := []storage.Message{
		{Role: "bot", Text: "hello there"},
		{Role: "bot", Text: "another reply"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "Недостаточно данных") {
		t.Errorf("expected 'Недостаточно данных' when only bot messages, got: %s", result)
	}
}

func TestAnalyzeStyle_ShortMessages(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "да"},
		{Role: "user", Text: "нет"},
		{Role: "user", Text: "ок"},
		{Role: "user", Text: "хз"},
		{Role: "user", Text: "лол"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "ОЧЕНЬ коротко") {
		t.Errorf("expected 'ОЧЕНЬ коротко' for 2-char messages, got: %s", result)
	}
	if !strings.Contains(result, "Характеристики стиля") {
		t.Errorf("expected 'Характеристики стиля' header, got: %s", result)
	}
}

func TestAnalyzeStyle_LongMessages(t *testing.T) {
	longMsg := strings.Repeat("Длинное сообщение с большим количеством текста. ", 10)
	msgs := []storage.Message{
		{Role: "user", Text: longMsg},
		{Role: "user", Text: longMsg},
		{Role: "user", Text: longMsg},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "длинными сообщениями") {
		t.Errorf("expected 'длинными сообщениями' for long messages, got: %s", result)
	}
}

func TestAnalyzeStyle_MediumMessages(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "Сегодня ходил в магазин, купил продукты на неделю"},
		{Role: "user", Text: "Потом зашёл к другу, посидели поговорили немного"},
		{Role: "user", Text: "Вечером буду смотреть фильм какой-нибудь интересный"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "средними сообщениями") {
		t.Errorf("expected 'средними сообщениями', got: %s", result)
	}
}

func TestAnalyzeStyle_WithEmoji(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "Привет! \U0001F600\U0001F600\U0001F600"},
		{Role: "user", Text: "Как дела? \U0001F601\U0001F601"},
		{Role: "user", Text: "Круто! \U0001F602\U0001F602\U0001F602\U0001F602"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "эмоджи") {
		t.Errorf("expected emoji usage detection, got: %s", result)
	}
}

func TestAnalyzeStyle_NoEmoji(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "Просто текст без эмоджи"},
		{Role: "user", Text: "И ещё один обычный текст"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "Не использует эмоджи") {
		t.Errorf("expected 'Не использует эмоджи', got: %s", result)
	}
}

func TestAnalyzeStyle_WithCaps(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "ЭТО ВСЁ КАПСОМ"},
		{Role: "user", Text: "ОПЯТЬ КАПС ВЕЗДЕ"},
		{Role: "user", Text: "И ЕЩЁ РАЗ КАПС"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "КАПС") {
		t.Errorf("expected caps detection, got: %s", result)
	}
}

func TestAnalyzeStyle_NoPunctuation(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "привет как дела"},
		{Role: "user", Text: "норм а у тебя"},
		{Role: "user", Text: "тоже норм"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "Не ставит точки") {
		t.Errorf("expected 'Не ставит точки', got: %s", result)
	}
}

func TestAnalyzeStyle_WithExclamations(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "Круто! Класс!"},
		{Role: "user", Text: "Супер! Вау!"},
		{Role: "user", Text: "Офигеть!"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "восклицательные") {
		t.Errorf("expected exclamation detection, got: %s", result)
	}
}

func TestAnalyzeStyle_WithEllipsis(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "Ну не знаю..."},
		{Role: "user", Text: "Может быть..."},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "многоточие") {
		t.Errorf("expected ellipsis detection, got: %s", result)
	}
}

func TestAnalyzeStyle_SampleMessages(t *testing.T) {
	msgs := []storage.Message{
		{Role: "user", Text: "первое"},
		{Role: "user", Text: "второе"},
		{Role: "user", Text: "третье"},
	}
	result := features.AnalyzeStyle(msgs)
	if !strings.Contains(result, "Примеры сообщений") {
		t.Errorf("expected 'Примеры сообщений' section, got: %s", result)
	}
	if !strings.Contains(result, "третье") {
		t.Errorf("expected last message in samples, got: %s", result)
	}
}
