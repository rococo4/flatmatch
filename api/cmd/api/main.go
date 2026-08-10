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
	"flatmatch/api/internal/api/http/poolsapi"
	"flatmatch/api/internal/api/http/searchesapi"
	"flatmatch/api/internal/api/http/usersapi"
	"flatmatch/api/internal/api/http/zonesapi"
	"flatmatch/api/internal/config"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/photostore"
	"flatmatch/api/internal/postgres"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/searches"
	"flatmatch/api/internal/service/users"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tgbot"
	"flatmatch/api/internal/tma"
	"flatmatch/api/migrations"
)

// shutdownGrace is how long in-flight requests get to finish after a signal.
const shutdownGrace = 15 * time.Second

// mediaPrefix is where uploaded photos are addressed, both in the URL the API
// hands out and in the route that serves them.
const mediaPrefix = "/media"

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

	// The one line that changes the day photos move to object storage.
	photos, err := photostore.NewDir(cfg.PhotoDir, mediaPrefix)
	if err != nil {
		return err
	}

	// A broken directory has to kill the process here, not a request at noon.
	dir, err := zones.Load()
	if err != nil {
		return err
	}

	searchesRepo := searchesrepo.New(pool)
	userService := users.New(usersrepo.New(pool), photos)
	searchService := searches.New(searchesRepo, dir)
	poolService := pools.New(searchesRepo, dir, photos)
	validator := tma.NewValidator(cfg.BotToken, cfg.InitDataTTL)

	server := &http.Server{
		Addr: cfg.APIAddr,
		Handler: router(log, pool, validator,
			userService, searchService, poolService, dir, photos.Root()),
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
	pinger httpx.Pinger,
	validator authmw.Validator,
	accounts *users.Service,
	searchService *searches.Service,
	poolService *pools.Service,
	dir *zones.Directory,
	mediaRoot string,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(httpx.RequestLogger(log))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", httpx.Health(pinger))

	// Photos are served without authentication, the same way an object storage
	// bucket will serve them later: the key is a random uuid, so the address is
	// unguessable, and a face in a profile is meant to be seen.
	r.Handle(mediaPrefix+"/*", http.StripPrefix(mediaPrefix+"/",
		http.FileServer(http.Dir(mediaRoot))))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.New(validator, accounts))
		r.Mount("/", usersapi.New(accounts).Routes())
		r.Method(http.MethodGet, "/zones", zonesapi.New(dir))
		r.Mount("/searches", searchesapi.New(searchService, poolService, dir).Routes())

		// chi allows only one Mount per pattern, so the two pool routes are
		// registered directly instead of via a second Mount("/", …).
		poolsHandler := poolsapi.New(poolService, searchService, dir)
		r.Get("/feed", poolsHandler.Feed)
		r.Get("/pools/zone/{zoneID}", poolsHandler.ZonePool)
	})

	return r
}
