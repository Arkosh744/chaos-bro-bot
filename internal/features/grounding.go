package features

import (
	"math/rand"

	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

func RandomGrounding() string {
	return models.GroundingTechniques[rand.Intn(len(models.GroundingTechniques))]
}
