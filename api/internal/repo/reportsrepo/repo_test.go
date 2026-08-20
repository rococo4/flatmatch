package reportsrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

const testTGBase int64 = -990000000

func newTestRepo(t *testing.T) (*pgxpool.Pool, *Repo) {
	t.Helper()

	pool := pgtest.NewPool(t)
	clean := func() {
		for _, q := range []string{
			`DELETE FROM reports WHERE reporter_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)
			    OR target_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up report test rows: %v", err)
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

func TestInsertStoresTheComplaintOnce(t *testing.T) {
	pool, repo := newTestRepo(t)
	reporter := seedUser(t, pool, 1)
	target := seedUser(t, pool, 2)
	ctx := t.Context()

	stored, inserted, err := repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: target,
		Reason: entity.ReasonScam, ContextKey: "zone:d-khamovniki",
	})
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}
	if !inserted {
		t.Fatal("the first complaint was not inserted")
	}
	if stored.ID == uuid.Nil {
		t.Error("the stored complaint has no id — the moderation card has nothing to point at")
	}
	if stored.Reason != entity.ReasonScam || stored.ContextKey != "zone:d-khamovniki" {
		t.Errorf("stored complaint = %+v, want the reason and the context back", stored)
	}
	if stored.Resolved() {
		t.Error("a fresh complaint must not be resolved")
	}

	_, inserted, err = repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: target, Reason: entity.ReasonSpam,
	})
	if err != nil {
		t.Fatalf("second Insert() failed: %v", err)
	}
	if inserted {
		t.Error("the same pair filed twice — the moderator would get a second card")
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM reports WHERE reporter_id = $1 AND target_id = $2`,
		reporter, target).Scan(&count); err != nil {
		t.Fatalf("count reports: %v", err)
	}
	if count != 1 {
		t.Errorf("found %d rows for one pair, want 1", count)
	}
}

func TestByIDReadsTheComplaintBack(t *testing.T) {
	pool, repo := newTestRepo(t)
	reporter := seedUser(t, pool, 3)
	target := seedUser(t, pool, 4)
	ctx := t.Context()

	stored, _, err := repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: target, Reason: entity.ReasonAbuse,
	})
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	got, err := repo.ByID(ctx, stored.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.ID != stored.ID || got.ReporterID != reporter || got.TargetID != target {
		t.Errorf("ByID() = %+v, want the stored complaint", got)
	}
	if got.Reason != entity.ReasonAbuse {
		t.Errorf("Reason = %q, want %q", got.Reason, entity.ReasonAbuse)
	}

	if _, err := repo.ByID(ctx, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ByID() of nobody = %v, want ErrNotFound", err)
	}
}

func TestResolveIsTakenOnlyOnce(t *testing.T) {
	pool, repo := newTestRepo(t)
	reporter := seedUser(t, pool, 5)
	target := seedUser(t, pool, 6)
	ctx := t.Context()

	stored, _, err := repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: target, Reason: entity.ReasonOther,
	})
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	done, err := repo.Resolve(ctx, stored.ID, entity.ReportSkipped)
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}
	if !done {
		t.Fatal("the first verdict did not land")
	}

	done, err = repo.Resolve(ctx, stored.ID, entity.ReportBanned)
	if err != nil {
		t.Fatalf("second Resolve() failed: %v", err)
	}
	if done {
		t.Error("a stale button overwrote a verdict that was already given")
	}

	got, err := repo.ByID(ctx, stored.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if !got.Resolved() {
		t.Fatal("ResolvedAt is empty after a verdict")
	}
	if got.Verdict == nil || *got.Verdict != entity.ReportSkipped {
		t.Errorf("Verdict = %v, want %q", got.Verdict, entity.ReportSkipped)
	}

	if done, err := repo.Resolve(ctx, uuid.New(), entity.ReportSkipped); err != nil || done {
		t.Errorf("Resolve() of nobody = (%v, %v), want (false, nil)", done, err)
	}
}

func TestUnresolvedFeedsTheQueue(t *testing.T) {
	pool, repo := newTestRepo(t)
	reporter := seedUser(t, pool, 7)
	open := seedUser(t, pool, 8)
	closed := seedUser(t, pool, 9)
	ctx := t.Context()

	waiting, _, err := repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: open, Reason: entity.ReasonSpam,
	})
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}
	resolved, _, err := repo.Insert(ctx, entity.Report{
		ReporterID: reporter, TargetID: closed, Reason: entity.ReasonSpam,
	})
	if err != nil {
		t.Fatalf("second Insert() failed: %v", err)
	}
	if _, err := repo.Resolve(ctx, resolved.ID, entity.ReportBanned); err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	got, err := repo.Unresolved(ctx)
	if err != nil {
		t.Fatalf("Unresolved() failed: %v", err)
	}

	var seenWaiting, seenResolved bool
	for _, r := range got {
		switch r.ID {
		case waiting.ID:
			seenWaiting = true
		case resolved.ID:
			seenResolved = true
		}
	}
	if !seenWaiting {
		t.Error("an unanswered complaint is missing from the queue")
	}
	if seenResolved {
		t.Error("a complaint with a verdict came back into the queue")
	}
}
