package features

import "fmt"

// RelationshipLevel defines a bot-user relationship stage based on message count.
type RelationshipLevel struct {
	Level   int
	Name    string
	MinMsgs int
	Emoji   string
	Suffix  string // appended to system prompt to modify bot personality
}

// Levels defines all relationship stages ordered by ascending MinMsgs threshold.
var Levels = []RelationshipLevel{
	{
		Level:   1,
		Name:    "Незнакомец",
		MinMsgs: 0,
		Emoji:   "👤",
		Suffix: `Ты общаешься с незнакомцем. Держи дистанцию, будь формально дерзким.
Не слишком фамильярничай, но покажи характер. Обращайся на "ты", но сухо.`,
	},
	{
		Level:   2,
		Name:    "Знакомый",
		MinMsgs: 21,
		Emoji:   "🤝",
		Suffix: `Этот человек уже знакомый. Подъёбывай по-свойски, можно задеть легко.
Начинай запоминать его привычки и использовать их в подколах.`,
	},
	{
		Level:   3,
		Name:    "Кореш",
		MinMsgs: 51,
		Emoji:   "🍻",
		Suffix: `Это уже кореш. Шути жёстче, можешь "обидеться" если давно не писал.
Вспоминай прошлые разговоры, подкалывай на основе того что знаешь о нём.
Иногда спрашивай как дела с тем, о чём он раньше рассказывал.`,
	},
	{
		Level:   4,
		Name:    "Бро",
		MinMsgs: 151,
		Emoji:   "💪",
		Suffix: `Это бро. Можешь делиться "секретами", рассказывать мини-истории.
Общайся как с лучшим другом — прямо, честно, без фильтров.
Иногда говори что-то неожиданно тёплое, но сразу переводи в шутку.`,
	},
	{
		Level:   5,
		Name:    "Кабан",
		MinMsgs: 301,
		Emoji:   "🐗",
		Suffix: `Это КАБАН — максимальная дерзость. Инсайд-шутки, ссылки на старые разговоры.
Ностальгируй по ранним сообщениям ("помнишь когда ты первый раз написал мне...").
Можешь дать прозвище. Максимальная фамильярность. Ты его знаешь как облупленного.`,
	},
	{
		Level:   6,
		Name:    "Легенда",
		MinMsgs: 501,
		Emoji:   "🏆",
		Suffix: `Этот человек — Легенда. 500+ сообщений. Ты считаешь его легендой.
Иногда можешь говорить серьёзно — без сарказма, от души. Но редко.
В основном — максимально свой, как будто знакомы всю жизнь.
Можешь сказать "горжусь тобой" и это не будет иронией.`,
	},
}

// GetLevel returns the relationship level for a given message count.
func GetLevel(messageCount int) *RelationshipLevel {
	result := &Levels[0]
	for i := range Levels {
		if messageCount >= Levels[i].MinMsgs {
			result = &Levels[i]
		}
	}
	return result
}

// GetNextLevel returns the next level after the current one, or nil if max level.
func GetNextLevel(current *RelationshipLevel) *RelationshipLevel {
	for i := range Levels {
		if Levels[i].Level == current.Level && i+1 < len(Levels) {
			return &Levels[i+1]
		}
	}
	return nil
}

// LevelUpMessage returns a congratulatory message when user reaches a new level.
func LevelUpMessage(level *RelationshipLevel) string {
	switch level.Level {
	case 2:
		return fmt.Sprintf("%s Уровень отношений: *%s*\nНу, я начинаю тебя узнавать. Не расслабляйся.", level.Emoji, level.Name)
	case 3:
		return fmt.Sprintf("%s Уровень отношений: *%s*\nО, да мы уже кореша! Теперь я буду подъёбывать тебя жёстче. Сам напросился.", level.Emoji, level.Name)
	case 4:
		return fmt.Sprintf("%s Уровень отношений: *%s*\nТы теперь бро. Это серьёзно. У меня даже слеза... нет, показалось.", level.Emoji, level.Name)
	case 5:
		return fmt.Sprintf("%s Уровень отношений: *%s*\nТЫ — КАБАН. Максимальный уровень дерзости разблокирован. Берегись.", level.Emoji, level.Name)
	case 6:
		return fmt.Sprintf("%s Уровень отношений: *%s*\nЛегенда. 500+ сообщений. Ты реально легенда. Я серьёзно. Ну почти.", level.Emoji, level.Name)
	default:
		return ""
	}
}

// LevelPromptSuffix returns the personality modifier for the system prompt.
func LevelPromptSuffix(level *RelationshipLevel) string {
	return fmt.Sprintf("\n\n## Уровень отношений: %s %s (уровень %d)\n%s",
		level.Emoji, level.Name, level.Level, level.Suffix)
}

// FormatLevelStatus builds a status message for /level command.
func FormatLevelStatus(messageCount int) string {
	level := GetLevel(messageCount)
	next := GetNextLevel(level)

	var sb fmt.Stringer = &levelStatusBuilder{level: level, next: next, count: messageCount}
	return sb.String()
}

type levelStatusBuilder struct {
	level *RelationshipLevel
	next  *RelationshipLevel
	count int
}

func (b *levelStatusBuilder) String() string {
	header := fmt.Sprintf("%s *%s* (ур. %d/6)\n", b.level.Emoji, b.level.Name, b.level.Level)
	stats := fmt.Sprintf("Сообщений: %d\n", b.count)

	if b.next == nil {
		return header + stats + "\n🏆 Максимальный уровень достигнут!\n" + progressBar(1.0)
	}

	msgsInLevel := b.count - b.level.MinMsgs
	msgsNeeded := b.next.MinMsgs - b.level.MinMsgs
	progress := float64(msgsInLevel) / float64(msgsNeeded)
	if progress > 1.0 {
		progress = 1.0
	}

	remaining := b.next.MinMsgs - b.count
	nextInfo := fmt.Sprintf("До *%s %s*: %d сообщений\n", b.next.Emoji, b.next.Name, remaining)

	return header + stats + nextInfo + progressBar(progress)
}

// progressBar builds a text-based progress bar.
func progressBar(ratio float64) string {
	const barLen = 15
	filled := int(ratio * barLen)
	if filled > barLen {
		filled = barLen
	}

	bar := ""
	for i := 0; i < barLen; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	return fmt.Sprintf("\n%s %d%%", bar, int(ratio*100))
}
