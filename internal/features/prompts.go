package features

import (
	"math/rand"
	"time"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

// Aliases for backward compatibility within features package and external callers.
var (
	SleepReplies    = models.SleepReplies
	EasterEggs      = models.EasterEggs
	OffendedReplies = models.OffendedReplies
	Bargains        = models.Bargains
)

const (
	TricksterSystemPrompt  = models.TricksterSystemPrompt
	RandomizerSystemPrompt = models.RandomizerSystemPrompt
	PredictionPrompt       = models.PredictionPrompt
	ChaosGeneratorPrompt   = models.ChaosGeneratorPrompt
	SummaryUpdatePrompt    = models.SummaryUpdatePrompt
	QuotesSystemPrompt     = models.QuotesSystemPrompt
	DailyQuestPrompt       = models.DailyQuestPrompt
	ProfileExtractPrompt   = models.ProfileExtractPrompt
	LieGeneratorPrompt     = models.LieGeneratorPrompt
	MorningRitualPrompt    = models.MorningRitualPrompt
	SilencePrompt          = models.SilencePrompt
	MirrorPrompt           = models.MirrorPrompt
	RoastPrompt            = models.RoastPrompt
	WisdomPrompt           = models.WisdomPrompt
	AntiHoroscopePrompt    = models.AntiHoroscopePrompt
	TruthPrompt            = models.TruthPrompt
	DarePrompt             = models.DarePrompt
	DanetkiPrompt          = models.DanetkiPrompt
	DanetkiJudgePrompt     = models.DanetkiJudgePrompt
	TriviaPrompt           = models.TriviaPrompt
	PlaylistPrompt         = models.PlaylistPrompt
	FutureLetterPrompt     = models.FutureLetterPrompt
	WeeklyChallengePrompt  = models.WeeklyChallengePrompt
	StoryStartPrompt       = models.StoryStartPrompt
	StoryContinuePrompt    = models.StoryContinuePrompt
	DuelQuestionPrompt     = models.DuelQuestionPrompt
	DuelJudgePrompt        = models.DuelJudgePrompt
	GroupQuestPrompt       = models.GroupQuestPrompt
	GroupQuestJudgePrompt  = models.GroupQuestJudgePrompt
)

// IsSleepTime returns true if current hour is between 23:00 and 09:00.
func IsSleepTime() bool {
	hour := time.Now().Hour()
	return hour >= 23 || hour < 9
}

// TimeOfDayMood returns a mood suffix for the system prompt based on current hour.
func TimeOfDayMood() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 6 && hour < 12:
		return models.MoodMorning
	case hour >= 12 && hour < 18:
		return models.MoodDay
	case hour >= 18 && hour < 23:
		return models.MoodEvening
	default:
		return models.MoodNight
	}
}

func RandomLoot() string {
	return models.LootPool[rand.Intn(len(models.LootPool))]
}

// DayOfWeekMood returns a mood suffix for the system prompt based on current day of week.
func DayOfWeekMood() string {
	switch time.Now().Weekday() {
	case time.Monday:
		return models.MoodMonday
	case time.Tuesday:
		return models.MoodTuesday
	case time.Wednesday:
		return models.MoodWednesday
	case time.Thursday:
		return models.MoodThursday
	case time.Friday:
		return models.MoodFriday
	case time.Saturday:
		return models.MoodSaturday
	case time.Sunday:
		return models.MoodSunday
	default:
		return ""
	}
}
