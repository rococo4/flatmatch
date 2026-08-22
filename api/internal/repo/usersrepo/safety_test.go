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
			`DELETE FROM match_polls WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_users WHERE from_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
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

func seedMatchedPair(t *testing.T, pool *pgxpool.Pool, a, b uuid.UUID) uuid.UUID {
	t.Helper()

	var matchID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO matches (a_id, b_id, context_zone_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid), 'd-khamovniki')
		RETURNING id`, a, b).Scan(&matchID)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO likes_users (from_id, to_id, zone_id, spent)
		VALUES ($1, $2, 'd-khamovniki', true), ($2, $1, NULL, false)`, a, b)
	if err != nil {
		t.Fatalf("seed likes: %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		INSERT INTO match_polls (match_id, user_id, answer) VALUES ($1, $2, 'talking')`, matchID, a)
	if err != nil {
		t.Fatalf("seed poll answer: %v", err)
	}
	return matchID
}

func countSafetyRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestBlockErasesTheMatchAndBothLikes(t *testing.T) {
	pool, repo := newSafetyRepo(t)
	blocker := seedSafetyUser(t, pool, 6)
	blocked := seedSafetyUser(t, pool, 7)
	matchID := seedMatchedPair(t, pool, blocker, blocked)
	ctx := t.Context()

	if err := repo.Block(ctx, blocked, blocker); err != nil {
		t.Fatalf("Block() failed: %v", err)
	}

	if n := countSafetyRows(t, pool, `SELECT count(*) FROM matches WHERE id = $1`, matchID); n != 0 {
		t.Errorf("the match survived the block: %d rows", n)
	}
	if n := countSafetyRows(t, pool, `SELECT count(*) FROM likes_users
		WHERE (from_id, to_id) IN (($1, $2), ($2, $1))`, blocker, blocked); n != 0 {
		t.Errorf("%d likes of the pair survived the block, want none", n)
	}
	if n := countSafetyRows(t, pool, `SELECT count(*) FROM match_polls WHERE match_id = $1`, matchID); n != 0 {
		t.Errorf("%d poll answers survived the block, want none", n)
	}
	if n := countSafetyRows(t, pool,
		`SELECT count(*) FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, blocked, blocker); n != 1 {
		t.Errorf("found %d block rows, want 1", n)
	}
}

func TestBlockOfAStrangerLeavesOtherPairsAlone(t *testing.T) {
	pool, repo := newSafetyRepo(t)
	blocker := seedSafetyUser(t, pool, 8)
	blocked := seedSafetyUser(t, pool, 9)
	third := seedSafetyUser(t, pool, 10)
	matchID := seedMatchedPair(t, pool, blocker, third)
	ctx := t.Context()

	if err := repo.Block(ctx, blocker, blocked); err != nil {
		t.Fatalf("Block() failed: %v", err)
	}

	if n := countSafetyRows(t, pool, `SELECT count(*) FROM matches WHERE id = $1`, matchID); n != 1 {
		t.Errorf("the untouched match is gone: %d rows, want 1", n)
	}
	if n := countSafetyRows(t, pool, `SELECT count(*) FROM likes_users
		WHERE (from_id, to_id) IN (($1, $2), ($2, $1))`, blocker, third); n != 2 {
		t.Errorf("likes of the untouched pair = %d, want 2", n)
	}
}
