package features

import (
	"context"
	"math/rand"

	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/pkg/models"
)

func RandomChaos() string {
	return models.ChaosPool[rand.Intn(len(models.ChaosPool))]
}

func GenerateChaos(ctx context.Context, cl *claude.Client) (string, error) {
	// 50/50: pool or generated
	if rand.Intn(2) == 0 {
		return RandomChaos(), nil
	}
	return cl.Ask(ctx, ChaosGeneratorPrompt, "Придумай задание")
}
