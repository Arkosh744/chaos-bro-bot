package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Arkosh744/chaos-bro-bot/internal/bot"
	"github.com/Arkosh744/chaos-bro-bot/internal/claude"
	"github.com/Arkosh744/chaos-bro-bot/internal/config"
	"github.com/Arkosh744/chaos-bro-bot/internal/groq"
	"github.com/Arkosh744/chaos-bro-bot/internal/scheduler"
	"github.com/Arkosh744/chaos-bro-bot/internal/storage"
	"github.com/Arkosh744/chaos-bro-bot/internal/web"
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

	var whisper *groq.WhisperClient
	if cfg.Groq.APIKey != "" {
		whisper = groq.NewWhisper(cfg.Groq.APIKey)
		log.Println("Groq Whisper enabled")
	}

	schedCfg := scheduler.Config{
		Enabled: cfg.Scheduler.Enabled,
		MinHour: cfg.Scheduler.MinHour,
		MaxHour: cfg.Scheduler.MaxHour,
		OwnerID: cfg.Telegram.OwnerID,
	}

	// Start web dashboard early — before Telegram connection, so it works even if TG API is down
	var webSrv *web.Server
	if cfg.Web.Enabled {
		webSrv = web.New(*cfg, store, nil) // scheduler not created yet, will be set later
		go webSrv.Start()
	}

	b, err := bot.New(cfg.Telegram.Token, cfg.Telegram.OwnerID, cl, whisper, store, schedCfg, *cfg, webSrv, cfg.Group.InterjectChance)
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
