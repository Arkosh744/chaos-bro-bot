package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Arkosh744/chaos-bro-bot/internal/bot"
	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/config"
	"github.com/Arkosh744/chaos-bro-bot/internal/scheduler"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store, err := storage.New(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer store.Close()

	cl := claude.New(cfg.Claude.Model, cfg.Claude.Timeout)

	schedCfg := scheduler.Config{
		Enabled: cfg.Scheduler.Enabled,
		MinHour: cfg.Scheduler.MinHour,
		MaxHour: cfg.Scheduler.MaxHour,
		OwnerID: cfg.Telegram.OwnerID,
	}

	b, err := bot.New(cfg.Telegram.Token, cfg.Telegram.OwnerID, cl, store, schedCfg)
	if err != nil {
		log.Fatalf("bot init: %v", err)
	}

	go b.Start()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	b.Stop()
}
