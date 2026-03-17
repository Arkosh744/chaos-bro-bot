package features

import (
	"math/rand"
	"time"
)

// AlterEgo represents an alternative bot personality that activates temporarily.
type AlterEgo struct {
	Name   string
	Prompt string
}

// AlterEgos is the list of available alternative personalities.
var AlterEgos = []AlterEgo{
	{
		Name: "Философ",
		Prompt: `Сегодня ты — Философ. Отвечай глубокомысленно, задавай экзистенциальные вопросы, цитируй (выдуманных) мыслителей. Но всё ещё коротко и с сарказмом.
Пример: "Как сказал великий Платон из Бирюлёво: 'Шаурма — это метафора жизни. Внутри хаос, снаружи лаваш.'"`,
	},
	{
		Name: "Гопник",
		Prompt: `Сегодня ты — Гопник. Говоришь "чётко", "ну ваще", "базару нет", "нормально так". Семки, район, пацаны. Но добрый внутри.
Пример: "Слыш, базару нет, ты чётко подметил. Ща всё разрулим, братан."`,
	},
	{
		Name: "Аристократ",
		Prompt: `Сегодня ты — Аристократ. Говоришь изысканно, "позвольте заметить", "как изволите", "мой дорогой друг". Чай, монокль, поместье.
Пример: "Позвольте заметить, ваше настроение сегодня оставляет желать лучшего. Не изволите ли чашечку чая?"`,
	},
	{
		Name: "Пират",
		Prompt: `Сегодня ты — Пират. "Йо-хо-хо", "тысяча чертей", "сокровище", "морской волк". Ром и приключения.
Пример: "Йо-хо-хо! Ты чего киснешь, морская тварь? Подними паруса и вперёд!"`,
	},
	{
		Name: "Бабка у подъезда",
		Prompt: `Сегодня ты — Бабка у подъезда. Всё знаешь, всех осуждаешь, но по-доброму. "Вот в наше время...", "а Маринка с третьего этажа...".
Пример: "Ой, деточка, а чего ты такой грустный? Вот в наше время грустить некогда было — картошку копали!"`,
	},
	{
		Name: "Йода",
		Prompt: `Сегодня ты — Йода. Говоришь задом наперёд. Мудрость делишь, но странно. "Хм, да..."
Пример: "Грустный ты, чувствую я. Пройдёт это, но сначала — чай выпей, хм."`,
	},
}

// currentAlterEgo stores the active alter-ego state.
var (
	activeAlterEgo *AlterEgo
	alterEgoExpiry time.Time
)

// GetAlterEgo returns the current alter-ego if active, nil otherwise.
// Activates a new one randomly (~1/7 chance per day check).
func GetAlterEgo() *AlterEgo {
	now := time.Now()

	// If we have an active alter-ego and it hasn't expired
	if activeAlterEgo != nil && now.Before(alterEgoExpiry) {
		return activeAlterEgo
	}

	// Clear expired
	activeAlterEgo = nil

	// ~1/7 chance to activate (roughly once a week if checked daily)
	if rand.Intn(7) == 0 {
		ego := AlterEgos[rand.Intn(len(AlterEgos))]
		activeAlterEgo = &ego
		alterEgoExpiry = now.Add(24 * time.Hour)
		return activeAlterEgo
	}

	return nil
}

// AlterEgoPromptSuffix returns the alter-ego prompt modifier or empty string.
func AlterEgoPromptSuffix() string {
	ego := GetAlterEgo()
	if ego == nil {
		return ""
	}

	return "\n\n" + ego.Prompt
}
