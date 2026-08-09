// Package pgtest connects DB tests to TEST_DATABASE_URL. Skipping without the
// variable keeps `go test ./...` runnable with nothing but a Go toolchain.
package pgtest

import (
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/postgres"
	"flatmatch/api/migrations"
)

func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set — see README, «Тесты, которым нужна база»")
	}

	ctx := t.Context()
	if err := postgres.Migrate(ctx, dsn, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
