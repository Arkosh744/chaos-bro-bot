package bot

import (
	"fmt"
	"time"
)

const (
	defaultRateLimit = 30 // default max Claude calls per hour per user
)

// getRateLimit reads the rate limit from runtime config, falling back to default.
func (b *Bot) getRateLimit() int {
	return b.store.GetRuntimeConfigInt("rate_limit_per_hour", defaultRateLimit)
}

// checkRateLimit returns true if the user is within the rate limit for Claude calls.
func (b *Bot) checkRateLimit(userID int64) bool {
	hourKey := fmt.Sprintf("rate_%d_%s", userID, time.Now().Format("2006010215"))
	count, _ := b.store.GetCounter(userID, hourKey)
	return count < b.getRateLimit()
}

// incrementRateLimit increments the Claude call counter for the current hour.
func (b *Bot) incrementRateLimit(userID int64) {
	hourKey := fmt.Sprintf("rate_%d_%s", userID, time.Now().Format("2006010215"))
	b.store.IncrementCounter(userID, hourKey)
}
