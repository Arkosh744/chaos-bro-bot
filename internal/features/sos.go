package features

import "strings"

var sosKeywords = []string{
	"паника", "тревога", "не могу дышать", "плохо", "умираю",
	"хуёво", "пиздец", "сука жизнь", "не хочу жить", "всё плохо",
	"депрессия", "панич", "трясёт", "страшно",
}

// IsSOS returns true if the text contains any distress keywords.
func IsSOS(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range sosKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// SOSMessage is the emergency response sent when distress is detected.
const SOSMessage = "🆘 Стоп. Я вижу что тебе хреново.\n\n" +
	"1. *Дыши*: вдох 4с → задержка 4с → выдох 6с (3 раза)\n" +
	"2. *Заземлись*: назови 5 вещей которые видишь\n" +
	"3. *Вода*: выпей стакан воды прямо сейчас\n\n" +
	"Если совсем плохо — позвони на горячую линию: *8-800-2000-122* (бесплатно, 24/7)\n\n" +
	"Я рядом. Напиши когда отпустит."
