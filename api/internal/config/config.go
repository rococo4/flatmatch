package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIAddr  string
	LogLevel slog.Level

	DatabaseURL string

	PhotoDir string

	BotToken string
	AppURL   string

	InitDataTTL time.Duration

	BotPollingEnabled bool

	ModerationChatID int64

	PushWorkerEnabled bool
	PushTick          time.Duration
}

var dotEnvCandidates = []string{".env", "../.env"}

func LoadDotEnv() error {
	for _, path := range dotEnvCandidates {
		if err := godotenv.Load(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
	return nil
}

func Load() (Config, error) {
	if err := LoadDotEnv(); err != nil {
		return Config{}, err
	}

	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	cfg := Config{
		APIAddr:     envOr("API_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		BotToken:    os.Getenv("BOT_TOKEN"),
		AppURL:      os.Getenv("APP_URL"),
		PhotoDir:    envOr("PHOTO_DIR", "./.data/photos"),
	}

	if cfg.DatabaseURL == "" {
		fail("DATABASE_URL is required")
	}
	if cfg.BotToken == "" {
		fail("BOT_TOKEN is required — get one from @BotFather (/newbot)")
	}

	level, err := parseLogLevel(envOr("LOG_LEVEL", "info"))
	if err != nil {
		fail("LOG_LEVEL: %w", err)
	}
	cfg.LogLevel = level

	ttl, err := time.ParseDuration(envOr("TMA_INITDATA_TTL", "24h"))
	switch {
	case err != nil:
		fail("TMA_INITDATA_TTL: %w", err)
	case ttl <= 0:
		fail("TMA_INITDATA_TTL must be positive, got %s", ttl)
	default:
		cfg.InitDataTTL = ttl
	}

	polling, err := parseBool(envOr("BOT_POLLING_ENABLED", "true"))
	if err != nil {
		fail("BOT_POLLING_ENABLED: %w", err)
	}
	cfg.BotPollingEnabled = polling

	pushes, err := parseBool(envOr("PUSH_WORKER_ENABLED", "true"))
	if err != nil {
		fail("PUSH_WORKER_ENABLED: %w", err)
	}
	cfg.PushWorkerEnabled = pushes

	if cfg.BotPollingEnabled || cfg.PushWorkerEnabled {
		if cfg.AppURL == "" {
			fail("APP_URL is required when BOT_POLLING_ENABLED or PUSH_WORKER_ENABLED is true")
		} else if !strings.HasPrefix(cfg.AppURL, "https://") {
			fail("APP_URL must be https:// for a Mini App button, got %q", cfg.AppURL)
		}
	}

	tick, err := time.ParseDuration(envOr("PUSH_TICK", "30s"))
	switch {
	case err != nil:
		fail("PUSH_TICK: %w", err)
	case tick <= 0:
		fail("PUSH_TICK must be positive, got %s", tick)
	default:
		cfg.PushTick = tick
	}

	if raw := os.Getenv("MODERATION_CHAT_ID"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fail("MODERATION_CHAT_ID: %w", err)
		}
		cfg.ModerationChatID = id
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(problems...))
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseLogLevel(raw string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw)); err != nil {
		return 0, fmt.Errorf("unknown level %q (want debug, info, warn or error)", raw)
	}
	return level, nil
}

func parseBool(raw string) (bool, error) {
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("want true or false, got %q", raw)
	}
	return v, nil
}
