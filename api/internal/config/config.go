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

	// DatabaseURL is the Postgres connection string.
	DatabaseURL string

	// PhotoDir is where profile photos are written until object storage is set
	// up. Temporary by design: the storage sits behind an interface, and moving
	// to S3 replaces the implementation, not this path's users.
	PhotoDir string

	BotToken string
	AppURL   string
	// InitDataTTL is the maximum accepted age of initData's auth_date.
	InitDataTTL time.Duration

	BotPollingEnabled bool
	// ModerationChatID is the founder's moderation chat (§5). Not used by the
	// skeleton; carried here so step 5 of §12 has somewhere to read it from.
	ModerationChatID int64
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

	if cfg.BotPollingEnabled {
		if cfg.AppURL == "" {
			fail("APP_URL is required when BOT_POLLING_ENABLED=true")
		} else if !strings.HasPrefix(cfg.AppURL, "https://") {
			fail("APP_URL must be https:// for a Mini App button, got %q", cfg.AppURL)
		}
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
