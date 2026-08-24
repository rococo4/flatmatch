package passesrepo

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/postgres/pgtest"
)

const testTGBase int64 = -991000000

const passWindow = 7 * 24 * time.Hour

func newTestRepo(t *testing.T) (*pgxpool.Pool, *Repo) {
	t.Helper()

	pool := pgtest.NewPool(t)
	clean := func() {
		for _, q := range []string{
			`DELETE FROM passes WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up pass test rows: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)

	return pool, New(pool)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, n int64) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status) VALUES ($1, 'test', 'active') RETURNING id`,
		testTGBase-n).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func age(t *testing.T, pool *pgxpool.Pool, userID, targetID uuid.UUID, d time.Duration) {
	t.Helper()

	_, err := pool.Exec(context.Background(), `
		UPDATE passes SET created_at = now() - make_interval(secs => $3)
		WHERE user_id = $1 AND target_id = $2`, userID, targetID, d.Seconds())
	if err != nil {
		t.Fatalf("age a pass: %v", err)
	}
}

func TestUpsertHidesTheTarget(t *testing.T) {
	pool, repo := newTestRepo(t)
	viewer := seedUser(t, pool, 1)
	target := seedUser(t, pool, 2)
	ctx := t.Context()

	if err := repo.Upsert(ctx, viewer, target); err != nil {
		t.Fatalf("Upsert() failed: %v", err)
	}

	got, err := repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if !slices.Equal(got, []uuid.UUID{target}) {
		t.Errorf("ActiveTargets() = %v, want [%s]", got, target)
	}
}

func TestUpsertRestartsTheSevenDays(t *testing.T) {
	pool, repo := newTestRepo(t)
	viewer := seedUser(t, pool, 3)
	target := seedUser(t, pool, 4)
	ctx := t.Context()

	if err := repo.Upsert(ctx, viewer, target); err != nil {
		t.Fatalf("Upsert() failed: %v", err)
	}
	age(t, pool, viewer, target, 8*24*time.Hour)

	got, err := repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ActiveTargets() = %v, want a stale pass ignored", got)
	}

	if err := repo.Upsert(ctx, viewer, target); err != nil {
		t.Fatalf("second Upsert() failed: %v", err)
	}

	got, err = repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if !slices.Equal(got, []uuid.UUID{target}) {
		t.Errorf("ActiveTargets() = %v, want the pass hidden again", got)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM passes WHERE user_id = $1 AND target_id = $2`,
		viewer, target).Scan(&count); err != nil {
		t.Fatalf("count passes: %v", err)
	}
	if count != 1 {
		t.Errorf("found %d rows for one pair, want 1", count)
	}
}

func TestActiveTargetsSeesOnlyItsOwnWindow(t *testing.T) {
	pool, repo := newTestRepo(t)
	viewer := seedUser(t, pool, 5)
	stranger := seedUser(t, pool, 6)
	fresh := seedUser(t, pool, 7)
	stale := seedUser(t, pool, 8)
	ctx := t.Context()

	for _, target := range []uuid.UUID{fresh, stale} {
		if err := repo.Upsert(ctx, viewer, target); err != nil {
			t.Fatalf("Upsert() failed: %v", err)
		}
	}
	if err := repo.Upsert(ctx, stranger, fresh); err != nil {
		t.Fatalf("Upsert() by the stranger failed: %v", err)
	}
	age(t, pool, viewer, stale, 7*24*time.Hour+time.Minute)

	got, err := repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if !slices.Equal(got, []uuid.UUID{fresh}) {
		t.Errorf("ActiveTargets() = %v, want only the fresh pass of this viewer", got)
	}
}

func TestClearBringsEveryoneBack(t *testing.T) {
	pool, repo := newTestRepo(t)
	viewer := seedUser(t, pool, 9)
	stranger := seedUser(t, pool, 10)
	first := seedUser(t, pool, 11)
	second := seedUser(t, pool, 12)
	ctx := t.Context()

	for _, target := range []uuid.UUID{first, second} {
		if err := repo.Upsert(ctx, viewer, target); err != nil {
			t.Fatalf("Upsert() failed: %v", err)
		}
	}
	if err := repo.Upsert(ctx, stranger, first); err != nil {
		t.Fatalf("Upsert() by the stranger failed: %v", err)
	}

	restored, err := repo.Clear(ctx, viewer)
	if err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}
	if restored != 2 {
		t.Errorf("Clear() = %d, want 2", restored)
	}

	got, err := repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ActiveTargets() = %v, want nothing hidden any more", got)
	}

	others, err := repo.ActiveTargets(ctx, stranger, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() of the stranger failed: %v", err)
	}
	if len(others) != 1 {
		t.Errorf("the stranger lost their passes: %v", others)
	}

	if restored, err := repo.Clear(ctx, viewer); err != nil || restored != 0 {
		t.Errorf("second Clear() = (%d, %v), want (0, nil)", restored, err)
	}
}

func TestPassCanPointAtAPoint(t *testing.T) {
	pool, repo := newTestRepo(t)
	viewer := seedUser(t, pool, 13)
	point := uuid.New()
	ctx := t.Context()

	if err := repo.Upsert(ctx, viewer, point); err != nil {
		t.Fatalf("Upsert() of a point failed: %v", err)
	}

	got, err := repo.ActiveTargets(ctx, viewer, passWindow)
	if err != nil {
		t.Fatalf("ActiveTargets() failed: %v", err)
	}
	if !slices.Equal(got, []uuid.UUID{point}) {
		t.Errorf("ActiveTargets() = %v, want the point id", got)
	}
}
