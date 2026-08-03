package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/api/http/usersapi"
	"flatmatch/api/internal/config"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/postgres"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/users"
	"flatmatch/api/internal/tgbot"
	"flatmatch/api/internal/tma"
	"flatmatch/api/migrations"
)

// shutdownGrace is how long in-flight requests get to finish after a signal.
const shutdownGrace = 15 * time.Second

func main() {
	if err := run(); err != nil {
		// The logger may not exist yet if configuration itself failed, so this
		// last-resort report goes straight to stderr.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := postgres.Migrate(ctx, cfg.DatabaseURL, migrations.FS, log); err != nil {
		return err
	}

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	userService := users.New(usersrepo.New(pool))
	validator := tma.NewValidator(cfg.BotToken, cfg.InitDataTTL)

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           router(log, pool, validator, userService),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	failed := make(chan error, 2)

	go func() {
		log.Info("http server listening", "addr", cfg.APIAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failed <- fmt.Errorf("http server: %w", err)
			return
		}
		failed <- nil
	}()

	if cfg.BotPollingEnabled {
		bot := tgbot.NewBot(tgbot.NewClient(cfg.BotToken), cfg.AppURL, log)
		go func() {
			if err := bot.Run(ctx); err != nil {
				failed <- fmt.Errorf("telegram bot: %w", err)
				return
			}
			failed <- nil
		}()
	} else {
		log.Warn("bot polling is disabled, /start will not answer")
	}

	// Wait for a signal or for a component to give up.
	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case runErr = <-failed:
		if runErr != nil {
			log.Error("component stopped", "error", runErr)
		}
		stop() // tell the other components to wind down too
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("http server did not shut down cleanly", "error", err)
	}
	log.Info("stopped")

	return runErr
}

func router(
	log *slog.Logger,
	pool httpx.Pinger,
	validator authmw.Validator,
	accounts authmw.Accounts,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(httpx.RequestLogger(log))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", httpx.Health(pool))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.New(validator, accounts))
		r.Mount("/", usersapi.New().Routes())
	})

	return r
}
