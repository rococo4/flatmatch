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
	"github.com/google/uuid"

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
	"flatmatch/api/internal/repo/notificationsrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/reportsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/likes"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/moderation"
	"flatmatch/api/internal/service/notifications"
	"flatmatch/api/internal/service/people"
	"flatmatch/api/internal/service/points"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/searches"
	"flatmatch/api/internal/service/users"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tgbot"
	"flatmatch/api/internal/tma"
	"flatmatch/api/migrations"
)

const shutdownGrace = 15 * time.Second

const mediaPrefix = "/media"

type visibility struct {
	users   *usersrepo.Repo
	matches *matchesrepo.Repo
}

type deps struct {
	accounts *users.Service
	searches *searches.Service
	pools    *pools.Service
	likes    *likes.Service
	matches  *matches.Service
	points   *points.Service
	persons  *people.Service

	people       *usersrepo.Repo
	pairLikes    *likesrepo.Repo
	pairSearches *searchesrepo.Repo

	pairPoints  *pointsrepo.Repo
	pairMatches *matchesrepo.Repo

	photos *photostore.Dir
	dir    *zones.Directory

	bot *tgbot.Bot
}

func (v visibility) VisibleBetween(ctx context.Context, a, b uuid.UUID) (bool, error) {
	return v.users.VisibleBetween(ctx, a, b)
}

func (v visibility) IsMatched(ctx context.Context, a, b uuid.UUID) (bool, error) {
	return v.matches.IsMatched(ctx, a, b)
}

func main() {
	if err := run(); err != nil {
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

	photos, err := photostore.NewDir(cfg.PhotoDir, mediaPrefix)
	if err != nil {
		return err
	}

	dir, err := zones.Load()
	if err != nil {
		return err
	}

	usersRepo := usersrepo.New(pool)
	searchesRepo := searchesrepo.New(pool)
	likesRepo := likesrepo.New(pool)
	pointsRepo := pointsrepo.New(pool)
	matchesRepo := matchesrepo.New(pool)
	reportsRepo := reportsrepo.New(pool)

	notificationsRepo := notificationsrepo.New(pool)
	pushes := notifications.New(notificationsRepo, searchesRepo, pointsRepo,
		visibility{users: usersRepo, matches: matchesRepo}, dir, log)

	botClient := tgbot.NewClient(cfg.BotToken)
	moderator := moderation.New(tgbot.NewModerationSender(botClient),
		cfg.ModerationChatID, usersRepo, pointsRepo, reportsRepo, photos, dir, pushes, notificationsRepo)

	pushWorker := notifications.NewWorker(notificationsRepo,
		tgbot.NewPushSender(botClient, cfg.AppURL), usersRepo, searchesRepo,
		matchesRepo, pointsRepo, dir, cfg.PushTick, log)
	if !cfg.BotPollingEnabled {
		pushWorker = pushWorker.WithoutAnswers()
	}
	pushCallbacks := notifications.NewCallbacks(searchesRepo, matchesRepo, usersRepo, pointsRepo)

	bot := tgbot.NewBot(botClient, cfg.AppURL, log, moderator, cfg.ModerationChatID, pushCallbacks)

	wired := deps{
		accounts: users.New(usersRepo, photos, moderator),
		searches: searches.New(searchesRepo, dir, pushes),
		pools:    pools.New(searchesRepo, dir, photos),
		likes:    likes.New(likesRepo, dir, pushes, pointsRepo),
		matches:  matches.New(matchesRepo, searchesRepo, usersRepo, pointsRepo, notificationsRepo, dir),

		points:       points.New(pointsRepo, searchesRepo, photos, dir, moderator, pushes),
		persons:      people.New(usersRepo, reportsRepo, matchesRepo, moderator),
		people:       usersRepo,
		pairLikes:    likesRepo,
		pairSearches: searchesRepo,
		pairPoints:   pointsRepo,
		pairMatches:  matchesRepo,
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

	if cfg.PushWorkerEnabled {
		go pushWorker.Run(ctx)
	} else {
		log.Warn("the push worker is disabled, notifications will only pile up in the log")
	}

	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case runErr = <-failed:
		if runErr != nil {
			log.Error("component stopped", "error", runErr)
		}
		stop()
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

	r.Handle(mediaPrefix+"/*", http.StripPrefix(mediaPrefix+"/",
		http.FileServer(http.Dir(d.photos.Root()))))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.New(validator, d.accounts, d.pairSearches))
		r.Mount("/", usersapi.New(d.accounts, d.bot).Routes())
		r.Mount("/zones", zonesapi.New(d.dir, d.pools).Routes())
		r.Mount("/searches", searchesapi.New(d.searches, d.pools, d.points, d.dir).Routes())
		r.Mount("/likes", likesapi.New(d.likes, d.accounts, d.pairPoints, d.dir).Routes())
		r.Mount("/matches", matchesapi.New(d.matches, d.accounts, d.dir).Routes())
		r.Mount("/points", pointsapi.New(d.points, d.photos, d.dir).Routes())

		poolsHandler := poolsapi.New(d.pools, d.searches, d.points, d.photos, d.dir)
		r.Get("/feed", poolsHandler.Feed)
		r.Get("/pools/zone/{zoneID}", poolsHandler.ZonePool)
		r.Mount("/people", peopleapi.New(d.persons, d.people, d.pairLikes, d.accounts,
			d.pairSearches, d.pairPoints, d.pairMatches, d.dir).Routes())
	})

	return r
}
