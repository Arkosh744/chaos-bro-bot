package features

import (
	"math/rand"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

func RandomFallback() string {
	return models.Fallbacks[rand.Intn(len(models.Fallbacks))]
}
