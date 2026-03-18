package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram struct {
		Token       string `yaml:"token"`
		OwnerID     int64  `yaml:"owner_id"`
		UseWebhook  bool   `yaml:"use_webhook"`
		WebhookURL  string `yaml:"webhook_url"`
		WebhookPort int    `yaml:"webhook_port"`
	} `yaml:"telegram"`
	Claude struct {
		Model      string        `yaml:"model"`
		FastModel  string        `yaml:"fast_model"`
		SmartModel string        `yaml:"smart_model"`
		Timeout    time.Duration `yaml:"timeout"`
	} `yaml:"claude"`
	Scheduler struct {
		Enabled bool `yaml:"enabled"`
		MinHour int  `yaml:"min_hour"`
		MaxHour int  `yaml:"max_hour"`
	} `yaml:"scheduler"`
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	Groq struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"groq"`
	Web struct {
		Enabled   bool   `yaml:"enabled"`
		Port      int    `yaml:"port"`
		AuthToken string `yaml:"auth_token"`
	} `yaml:"web"`
	Group struct {
		InterjectChance int `yaml:"interject_chance"` // percentage 0-100, default 10
	} `yaml:"group"`
}

func Load() (*Config, error) {
	// Try config.local.yaml first, then config.yaml
	paths := []string{"config.local.yaml", "config.yaml"}

	var data []byte
	var err error
	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("no config file found: %w", err)
	}

	// Expand env vars in token
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Telegram.Token == "" {
		// Fallback to env
		cfg.Telegram.Token = os.Getenv("TELEGRAM_TOKEN")
	}
	if cfg.Telegram.Token == "" {
		return nil, fmt.Errorf("telegram token is required (config or TELEGRAM_TOKEN env)")
	}

	// Defaults
	if cfg.Claude.Model == "" {
		cfg.Claude.Model = "sonnet"
	}
	if cfg.Claude.FastModel == "" {
		cfg.Claude.FastModel = cfg.Claude.Model
	}
	if cfg.Claude.SmartModel == "" {
		cfg.Claude.SmartModel = "sonnet"
	}
	if cfg.Claude.Timeout == 0 {
		cfg.Claude.Timeout = 30 * time.Second
	}
	if cfg.Scheduler.MaxHour == 0 {
		cfg.Scheduler.MaxHour = 22
	}
	if cfg.Storage.DBPath == "" {
		cfg.Storage.DBPath = "data/trickster.db"
	}
	if cfg.Web.Port == 0 {
		cfg.Web.Port = 8080
	}
	if cfg.Group.InterjectChance == 0 {
		cfg.Group.InterjectChance = 10
	}
	if cfg.Telegram.WebhookPort == 0 {
		cfg.Telegram.WebhookPort = 8443
	}

	return &cfg, nil
}
