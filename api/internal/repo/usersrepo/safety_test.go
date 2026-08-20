package usersrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

const safetyTGBase int64 = -991000000

func newSafetyRepo(t *testing.T) (*pgxpool.Pool, *Repo) {
	t.Helper()

	pool := pgtest.NewPool(t)
	clean := func() {
		for _, q := range []string{
			`DELETE FROM blocks WHERE blocker_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)
			    OR blocked_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, safetyTGBase); err != nil {
				t.Errorf("clean up safety test rows: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)

	return pool, New(pool)
}

func seedSafetyUser(t *testing.T, pool *pgxpool.Pool, n int64) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', 'active', 'male', 'any') RETURNING id`, safetyTGBase-n).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestSetSourceIfEmptyKeepsTheFirstTouch(t *testing.T) {
	pool, repo := newSafetyRepo(t)
	user := seedSafetyUser(t, pool, 1)
	ctx := t.Context()

	if err := repo.SetSourceIfEmpty(ctx, user, "hsetndr"); err != nil {
		t.Fatalf("first SetSourceIfEmpty() failed: %v", err)
	}
	if err := repo.SetSourceIfEmpty(ctx, user, "vk"); err != nil {
		t.Fatalf("second SetSourceIfEmpty() failed: %v", err)
	}

	got, err := repo.ByID(ctx, user)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.Source != "hsetndr" {
		t.Errorf("Source = %q, want the first touch %q", got.Source, "hsetndr")
	}
}

func TestSetSourceIfEmptyIgnoresAMissingUser(t *testing.T) {
	_, repo := newSafetyRepo(t)

	if err := repo.SetSourceIfEmpty(t.Context(), uuid.New(), "hsetndr"); err != nil {
		t.Errorf("SetSourceIfEmpty() for nobody = %v, want it silent", err)
	}
}

func TestBlockHidesThePairBothWays(t *testing.T) {
	pool, repo := newSafetyRepo(t)
	blocker := seedSafetyUser(t, pool, 2)
	blocked := seedSafetyUser(t, pool, 3)
	ctx := t.Context()

	if _, err := repo.VisibleByID(ctx, blocker, blocked); err != nil {
		t.Fatalf("the pair was not visible before the block: %v", err)
	}

	if err := repo.Block(ctx, blocker, blocked); err != nil {
		t.Fatalf("Block() failed: %v", err)
	}

	if _, err := repo.VisibleByID(ctx, blocker, blocked); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("VisibleByID(blocker → blocked) = %v, want ErrNotFound", err)
	}
	if _, err := repo.VisibleByID(ctx, blocked, blocker); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("VisibleByID(blocked → blocker) = %v, want ErrNotFound", err)
	}
}

func TestBlockIsIdempotent(t *testing.T) {
	pool, repo := newSafetyRepo(t)
	blocker := seedSafetyUser(t, pool, 4)
	blocked := seedSafetyUser(t, pool, 5)
	ctx := t.Context()

	for i := range 2 {
		if err := repo.Block(ctx, blocker, blocked); err != nil {
			t.Fatalf("Block() call %d failed: %v", i+1, err)
		}
	}

	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, blocker, blocked).Scan(&count)
	if err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	if count != 1 {
		t.Errorf("found %d block rows, want 1", count)
	}
}
