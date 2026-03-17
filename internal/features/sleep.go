package features

import (
	"fmt"
	"strings"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

// SleepDay represents estimated sleep data for one day.
type SleepDay struct {
	Date       string
	WakeUp     string  // first message time HH:MM
	LastActive string  // last message time HH:MM (previous day)
	SleepHours float64
}

var weekdayNames = map[time.Weekday]string{
	time.Monday:    "Пн",
	time.Tuesday:   "Вт",
	time.Wednesday: "Ср",
	time.Thursday:  "Чт",
	time.Friday:    "Пт",
	time.Saturday:  "Сб",
	time.Sunday:    "Вс",
}

// AnalyzeSleep looks at first and last message times to estimate sleep patterns.
func AnalyzeSleep(store *storage.Storage, userID int64, days int) []SleepDay {
	now := time.Now()
	var result []SleepDay

	for i := 1; i <= days; i++ {
		day := now.AddDate(0, 0, -i)
		prevDay := day.AddDate(0, 0, -1)

		dateStr := day.Format("2006-01-02")
		prevDateStr := prevDay.Format("2006-01-02")

		// First message of the day = wake up proxy
		firstMsg, _, err := store.GetFirstAndLastMessageTimes(userID, dateStr)
		if err != nil || firstMsg.IsZero() {
			continue
		}

		// Last message of the previous day = sleep proxy
		_, lastMsg, err := store.GetFirstAndLastMessageTimes(userID, prevDateStr)
		if err != nil || lastMsg.IsZero() {
			continue
		}

		sleepHours := firstMsg.Sub(lastMsg).Hours()
		if sleepHours <= 0 || sleepHours > 18 {
			// Unrealistic data, skip
			continue
		}

		result = append(result, SleepDay{
			Date:       dateStr,
			WakeUp:     firstMsg.Format("15:04"),
			LastActive: lastMsg.Format("15:04"),
			SleepHours: sleepHours,
		})
	}

	return result
}

func sleepEmoji(hours float64) string {
	switch {
	case hours >= 8.5:
		return "\U0001F634" // sleeping
	case hours >= 7.5:
		return "\U0001F44D" // thumbs up
	case hours >= 6.5:
		return "\U0001F610" // neutral
	default:
		return "\U0001F629" // weary
	}
}

// FormatSleepReport builds a text report of sleep patterns.
func FormatSleepReport(days []SleepDay) string {
	if len(days) == 0 {
		return "Недостаточно данных для анализа сна. Пиши чаще, тогда будет что анализировать."
	}

	var sb strings.Builder
	sb.WriteString("\U0001F634 Твой сон за последние дни:\n\n")

	var totalHours float64
	latestSleep := ""
	latestSleepDay := ""

	for _, d := range days {
		date, _ := time.Parse("2006-01-02", d.Date)
		dayName := weekdayNames[date.Weekday()]
		emoji := sleepEmoji(d.SleepHours)

		sb.WriteString(fmt.Sprintf("%s: %s → %s (%.1fч) %s\n",
			dayName, d.LastActive, d.WakeUp, d.SleepHours, emoji))

		totalHours += d.SleepHours

		if latestSleep == "" || d.LastActive > latestSleep {
			latestSleep = d.LastActive
			latestSleepDay = dayName
		}
	}

	avg := totalHours / float64(len(days))
	sb.WriteString(fmt.Sprintf("\nСреднее: %.1fч\n", avg))

	if latestSleepDay != "" {
		sb.WriteString(fmt.Sprintf("Позже всего ложишься: %s (%s). Кабан бы не одобрил.", latestSleepDay, latestSleep))
	}

	return sb.String()
}
