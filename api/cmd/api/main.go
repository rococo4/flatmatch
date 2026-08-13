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
	"flatmatch/api/internal/api/http/likesapi"
	"flatmatch/api/internal/api/http/matchesapi"
	"flatmatch/api/internal/api/http/peopleapi"
	"flatmatch/api/internal/api/http/pointsapi"
	"flatmatch/api/internal/api/http/poolsapi"
	"flatmatch/api/internal/api/http/searchesapi"
	"flatmatch/api/internal/api/http/usersapi"
	"flatmatch/api/internal/api/http/zonesapi"
	"flatmatch/api/internal/config"
	"flatmatch/api/internal/httpx"
	"flatmatch/api/internal/photostore"
	"flatmatch/api/internal/postgres"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/likes"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/moderation"
	"flatmatch/api/internal/service/points"
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

// deps are the wired components the routes are built from — a struct rather
// than a dozen positional parameters.
type deps struct {
	accounts *users.Service
	searches *searches.Service
	pools    *pools.Service
	likes    *likes.Service
	matches  *matches.Service
	points   *points.Service

	// people, pairLikes and pairSearches are storage read directly: the profile
	// screen only asks for facts — is this person visible to the viewer, what
	// have the two already done to each other, do their searches intersect,
	// how many slots are left — and there is no policy in between for a
	// service to hold.
	people       *usersrepo.Repo
	pairLikes    *likesrepo.Repo
	pairSearches *searchesrepo.Repo
	// pairPoints is the point half of the same question: does a point put these
	// two in one place (§2.3).
	pairPoints *pointsrepo.Repo

	photos *photostore.Dir
	dir    *zones.Directory
	// bot is asked for one thing only: its own @name, which the Mini App needs
	// to build share links back into the same bot.
	bot *tgbot.Bot
}

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

	usersRepo := usersrepo.New(pool)
	searchesRepo := searchesrepo.New(pool)
	likesRepo := likesrepo.New(pool)
	pointsRepo := pointsrepo.New(pool)

	// The bot client is built before the services because moderation sends
	// through it, and the polling bot in turn routes taps back into moderation.
	botClient := tgbot.NewClient(cfg.BotToken)
	moderator := moderation.New(tgbot.NewModerationSender(botClient),
		cfg.ModerationChatID, usersRepo, pointsRepo, photos, dir)

	// The bot is built even with polling off: /me hands its @name to the Mini
	// App for share links, and without a poll that name is simply empty.
	bot := tgbot.NewBot(botClient, cfg.AppURL, log, moderator, cfg.ModerationChatID)

	wired := deps{
		accounts: users.New(usersRepo, photos, moderator),
		searches: searches.New(searchesRepo, dir),
		pools:    pools.New(searchesRepo, dir, photos),
		likes:    likes.New(likesRepo, dir),
		matches:  matches.New(matchesrepo.New(pool), searchesRepo, usersRepo, pointsRepo, dir),
		// Модератор обязателен: точка создаётся в pending и без сообщения в чат
		// осталась бы ждать вечно (§5).
		points:       points.New(pointsRepo, searchesRepo, photos, dir, moderator),
		people:       usersRepo,
		pairLikes:    likesRepo,
		pairSearches: searchesRepo,
		pairPoints:   pointsRepo,
		photos:       photos,
		dir:          dir,
		bot:          bot,
	}
	validator := tma.NewValidator(cfg.BotToken, cfg.InitDataTTL)

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           router(log, pool, validator, wired),
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
	d deps,
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
		http.FileServer(http.Dir(d.photos.Root()))))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.New(validator, d.accounts))
		r.Mount("/", usersapi.New(d.accounts, d.bot).Routes())
		r.Method(http.MethodGet, "/zones", zonesapi.New(d.dir))
		r.Mount("/searches", searchesapi.New(d.searches, d.pools, d.points, d.dir).Routes())
		r.Mount("/likes", likesapi.New(d.likes, d.accounts, d.pairPoints, d.dir).Routes())
		r.Mount("/matches", matchesapi.New(d.matches, d.accounts, d.dir).Routes())
		r.Mount("/points", pointsapi.New(d.points, d.photos, d.dir).Routes())

		// chi allows only one Mount per pattern, so the routes that would need a
		// second Mount("/", …) are registered directly.
		poolsHandler := poolsapi.New(d.pools, d.searches, d.points, d.photos, d.dir)
		r.Get("/feed", poolsHandler.Feed)
		r.Get("/pools/zone/{zoneID}", poolsHandler.ZonePool)
		r.Get("/people/{id}", peopleapi.New(
			d.people, d.pairLikes, d.accounts, d.pairSearches, d.pairPoints, d.dir).Profile)
	})

	return r
}
