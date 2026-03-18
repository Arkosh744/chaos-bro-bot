package features

// PromptStore provides access to prompt overrides without importing storage directly.
type PromptStore interface {
	GetPromptOverride(name string) (string, error)
}

// GetPrompt returns the prompt value, checking overrides first.
// If the store has an override for the given name, it is returned.
// Otherwise, the defaultValue is returned.
func GetPrompt(store PromptStore, name, defaultValue string) string {
	if store != nil {
		if override, err := store.GetPromptOverride(name); err == nil && override != "" {
			return override
		}
	}
	return defaultValue
}

// PromptDefinition describes a prompt exposed in the admin panel.
type PromptDefinition struct {
	Name         string `json:"name"`
	DefaultValue string `json:"default_value"`
}

// AllPrompts returns the list of all editable prompts with their default values.
func AllPrompts() []PromptDefinition {
	return []PromptDefinition{
		{Name: "TricksterSystemPrompt", DefaultValue: TricksterSystemPrompt},
		{Name: "PredictionPrompt", DefaultValue: PredictionPrompt},
		{Name: "RandomizerSystemPrompt", DefaultValue: RandomizerSystemPrompt},
		{Name: "ChaosGeneratorPrompt", DefaultValue: ChaosGeneratorPrompt},
		{Name: "SummaryUpdatePrompt", DefaultValue: SummaryUpdatePrompt},
		{Name: "QuotesSystemPrompt", DefaultValue: QuotesSystemPrompt},
		{Name: "DailyQuestPrompt", DefaultValue: DailyQuestPrompt},
		{Name: "ProfileExtractPrompt", DefaultValue: ProfileExtractPrompt},
		{Name: "LieGeneratorPrompt", DefaultValue: LieGeneratorPrompt},
		{Name: "SilencePrompt", DefaultValue: SilencePrompt},
		{Name: "MirrorPrompt", DefaultValue: MirrorPrompt},
		{Name: "RoastPrompt", DefaultValue: RoastPrompt},
		{Name: "WisdomPrompt", DefaultValue: WisdomPrompt},
		{Name: "AntiHoroscopePrompt", DefaultValue: AntiHoroscopePrompt},
		{Name: "MorningRitualPrompt", DefaultValue: MorningRitualPrompt},
		{Name: "RecallPrompt", DefaultValue: RecallPrompt},
		{Name: "TruthPrompt", DefaultValue: TruthPrompt},
		{Name: "DarePrompt", DefaultValue: DarePrompt},
		{Name: "DanetkiPrompt", DefaultValue: DanetkiPrompt},
		{Name: "TriviaPrompt", DefaultValue: TriviaPrompt},
		{Name: "PlaylistPrompt", DefaultValue: PlaylistPrompt},
		{Name: "FutureLetterPrompt", DefaultValue: FutureLetterPrompt},
		{Name: "WeeklyChallengePrompt", DefaultValue: WeeklyChallengePrompt},
		{Name: "StoryStartPrompt", DefaultValue: StoryStartPrompt},
		{Name: "StoryContinuePrompt", DefaultValue: StoryContinuePrompt},
	}
}
