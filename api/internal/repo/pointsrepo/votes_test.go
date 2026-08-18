package pointsrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func activeFlat(t *testing.T, repo *Repo, createdBy *uuid.UUID, source entity.PointSource, url string) entity.Point {
	t.Helper()
	p, err := repo.Create(t.Context(), flatPoint(createdBy, source, url, entity.PointActive))
	if err != nil {
		t.Fatalf("create point %s: %v", url, err)
	}
	return p
}

func flatWithPool(t *testing.T, pool *pgxpool.Pool, repo *Repo) (point entity.Point, author uuid.UUID, voters [2]uuid.UUID) {
	t.Helper()
	author = seedUser(t, pool, 1)
	point = activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/pooled")
	voters = [2]uuid.UUID{seedUser(t, pool, 2), seedUser(t, pool, 3)}
	for _, v := range voters {
		enterPool(t, repo, v, point.ID)
	}
	return point, author, voters
}

func pointByID(t *testing.T, repo *Repo, id uuid.UUID) entity.Point {
	t.Helper()
	got, err := repo.ByID(t.Context(), id)
	if err != nil {
		t.Fatalf("ByID(%s) failed: %v", id, err)
	}
	return got
}

func stalePoint(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, daysAgo int) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE points SET last_seen_alive = now() - make_interval(days => $2), ping_asked_at = now()
		WHERE id = $1`, id, daysAgo)
	if err != nil {
		t.Fatalf("stale point: %v", err)
	}
}

func pausePoint(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE points SET status = 'paused', gone_count = 1 WHERE id = $1`, id)
	if err != nil {
		t.Fatalf("pause point: %v", err)
	}
}

func fresh(p entity.Point) bool {
	return p.PingAskedAt == nil && p.LastSeenAlive.After(time.Now().Add(-time.Hour))
}

func pointIDs(points []FeedPoint) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, p := range points {
		out[p.Point.ID] = true
	}
	return out
}

func TestVoteUpsertsAndRefreshesAlive(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	voter := seedUser(t, pool, 1)

	flat := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/alive")
	stalePoint(t, pool, flat.ID, 10)

	out, err := repo.Vote(t.Context(), voter, flat.ID, true)
	if err != nil {
		t.Fatalf("Vote(alive) failed: %v", err)
	}
	if out != (VoteOutcome{}) {
		t.Errorf("Vote(alive) = %+v, want a quiet outcome", out)
	}
	got := pointByID(t, repo, flat.ID)
	if got.Status != entity.PointActive || !fresh(got) {
		t.Errorf("point after an alive vote = %s / %v / %v, want active and fresh",
			got.Status, got.LastSeenAlive, got.PingAskedAt)
	}

	gone, err := repo.Vote(t.Context(), voter, flat.ID, false)
	if err != nil {
		t.Fatalf("Vote(gone) failed: %v", err)
	}
	if gone.GoneVotes != 1 || gone.Hidden || gone.Closed {
		t.Errorf("Vote(gone) = %+v, want a single vote below the threshold", gone)
	}

	again, err := repo.Vote(t.Context(), voter, flat.ID, true)
	if err != nil {
		t.Fatalf("Vote(alive) again failed: %v", err)
	}
	if again.GoneVotes != 0 {
		t.Errorf("Vote(alive) again = %+v, want the gone vote flipped back", again)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM point_votes WHERE point_id = $1`, flat.ID); n != 1 {
		t.Errorf("three votes of one person left %d rows, want 1", n)
	}

	if _, err := repo.Vote(t.Context(), voter, uuid.New(), true); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Vote() on a missing point = %v, want ErrNotFound", err)
	}
}

func TestVotesHideAUserLinkFlatOnce(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	flat, author, voters := flatWithPool(t, pool, repo)
	outsider := seedUser(t, pool, 4)

	members, err := repo.AllPoolMemberIDs(t.Context(), flat.ID)
	if err != nil {
		t.Fatalf("AllPoolMemberIDs() failed: %v", err)
	}
	if len(members) != 3 || members[0] != author || members[1] != voters[0] || members[2] != voters[1] {
		t.Fatalf("AllPoolMemberIDs() = %v, want the creator and both members in entry order", members)
	}

	first, err := repo.Vote(t.Context(), voters[0], flat.ID, false)
	if err != nil {
		t.Fatalf("Vote() failed: %v", err)
	}
	if first.GoneVotes != 1 || first.Hidden || first.Closed {
		t.Errorf("first gone vote = %+v, want it below the threshold", first)
	}
	if got := pointByID(t, repo, flat.ID); got.Status != entity.PointActive || got.GoneCount != 0 {
		t.Errorf("point after one vote = %s / %d, want active with gone_count 0", got.Status, got.GoneCount)
	}

	second, err := repo.Vote(t.Context(), voters[1], flat.ID, false)
	if err != nil {
		t.Fatalf("Vote() failed: %v", err)
	}
	if second.GoneVotes != 2 || !second.Hidden || second.Closed {
		t.Errorf("second gone vote = %+v, want a first hiding", second)
	}
	got := pointByID(t, repo, flat.ID)
	if got.Status != entity.PointPaused || got.GoneCount != 1 || got.CloseReason != nil {
		t.Errorf("hidden point = %s / %d / %v, want paused with gone_count 1",
			got.Status, got.GoneCount, pointer.Get(got.CloseReason))
	}

	if _, err := repo.Vote(t.Context(), outsider, flat.ID, false); !errors.Is(err, entity.ErrPointNotActive) {
		t.Errorf("Vote() on a hidden point = %v, want ErrPointNotActive", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM point_votes WHERE point_id = $1`, flat.ID); n != 2 {
		t.Errorf("point holds %d votes, want the two that hid it", n)
	}
}

func TestSecondHidingIsFinal(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	flat, author, voters := flatWithPool(t, pool, repo)

	for _, v := range voters {
		if _, err := repo.Vote(t.Context(), v, flat.ID, false); err != nil {
			t.Fatalf("Vote() failed: %v", err)
		}
	}
	stalePoint(t, pool, flat.ID, 10)

	if err := repo.Restore(t.Context(), author, flat.ID); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}
	got := pointByID(t, repo, flat.ID)
	if got.Status != entity.PointActive || got.GoneCount != 1 || !fresh(got) {
		t.Errorf("restored point = %s / %d / %v, want active, gone_count 1 and fresh",
			got.Status, got.GoneCount, got.PingAskedAt)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM point_votes WHERE point_id = $1`, flat.ID); n != 0 {
		t.Errorf("restore left %d votes, want none", n)
	}

	first, err := repo.Vote(t.Context(), voters[0], flat.ID, false)
	if err != nil {
		t.Fatalf("Vote() failed: %v", err)
	}
	if first.GoneVotes != 1 || first.Hidden {
		t.Errorf("first gone vote after restore = %+v, want it below the threshold", first)
	}
	second, err := repo.Vote(t.Context(), voters[1], flat.ID, false)
	if err != nil {
		t.Fatalf("Vote() failed: %v", err)
	}
	if second.GoneVotes != 2 || !second.Hidden || !second.Closed {
		t.Errorf("second hiding = %+v, want it final", second)
	}
	got = pointByID(t, repo, flat.ID)
	if got.Status != entity.PointClosed || got.GoneCount != 2 ||
		got.CloseReason == nil || *got.CloseReason != string(entity.CloseReasonGone) {
		t.Errorf("finally hidden point = %s / %d / %v, want closed as gone with gone_count 2",
			got.Status, got.GoneCount, pointer.Get(got.CloseReason))
	}
}

func TestChannelFlatClosesAtOnce(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	flat := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/channel")
	first, second := seedUser(t, pool, 1), seedUser(t, pool, 2)
	enterPool(t, repo, first, flat.ID)
	enterPool(t, repo, second, flat.ID)

	if out, err := repo.Vote(t.Context(), first, flat.ID, false); err != nil || out.Hidden {
		t.Fatalf("first gone vote = %+v, %v, want it below the threshold", out, err)
	}
	out, err := repo.Vote(t.Context(), second, flat.ID, false)
	if err != nil {
		t.Fatalf("Vote() failed: %v", err)
	}
	if out.GoneVotes != 2 || !out.Hidden || !out.Closed {
		t.Errorf("second gone vote = %+v, want a point without a creator closed at once", out)
	}
	got := pointByID(t, repo, flat.ID)
	if got.Status != entity.PointClosed || got.GoneCount != 1 ||
		got.CloseReason == nil || *got.CloseReason != string(entity.CloseReasonGone) {
		t.Errorf("channel point = %s / %d / %v, want closed as gone", got.Status, got.GoneCount, pointer.Get(got.CloseReason))
	}
	if _, err := repo.Vote(t.Context(), first, flat.ID, false); !errors.Is(err, entity.ErrPointNotActive) {
		t.Errorf("Vote() on a closed point = %v, want ErrPointNotActive", err)
	}
}

func TestRestoreNeedsPaused(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	stranger := seedUser(t, pool, 2)

	flat := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/mine")
	market := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/market")
	pausePoint(t, pool, market.ID)

	if err := repo.Restore(t.Context(), author, flat.ID); !errors.Is(err, entity.ErrPointNotPaused) {
		t.Errorf("Restore() of an active point = %v, want ErrPointNotPaused", err)
	}
	if err := repo.Restore(t.Context(), stranger, flat.ID); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Restore() by a stranger = %v, want ErrNotFound", err)
	}
	if err := repo.Restore(t.Context(), author, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Restore() of a missing point = %v, want ErrNotFound", err)
	}
	if err := repo.Restore(t.Context(), author, market.ID); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Restore() of a point without a creator = %v, want ErrNotFound", err)
	}

	pausePoint(t, pool, flat.ID)
	stalePoint(t, pool, flat.ID, 10)
	if err := repo.Restore(t.Context(), author, flat.ID); err != nil {
		t.Fatalf("Restore() of a paused point failed: %v", err)
	}
	if got := pointByID(t, repo, flat.ID); got.Status != entity.PointActive || !fresh(got) {
		t.Errorf("restored point = %s / %v, want active and fresh", got.Status, got.PingAskedAt)
	}
}

func TestCloseWithReasonOnlyLive(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)

	live := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/live")
	closed, err := repo.CloseWithReason(t.Context(), live.ID, entity.CloseReasonExpired)
	if err != nil || !closed {
		t.Fatalf("CloseWithReason() of an active point = %v, %v, want true, nil", closed, err)
	}
	got := pointByID(t, repo, live.ID)
	if got.Status != entity.PointClosed || got.CloseReason == nil ||
		*got.CloseReason != string(entity.CloseReasonExpired) {
		t.Errorf("closed point = %s / %v, want closed as expired", got.Status, pointer.Get(got.CloseReason))
	}
	if closed, err := repo.CloseWithReason(t.Context(), live.ID, entity.CloseReasonGone); err != nil || closed {
		t.Errorf("CloseWithReason() of a closed point = %v, %v, want false, nil", closed, err)
	}
	if got := pointByID(t, repo, live.ID); got.CloseReason == nil ||
		*got.CloseReason != string(entity.CloseReasonExpired) {
		t.Errorf("close_reason = %v, want the first one kept", pointer.Get(got.CloseReason))
	}

	paused := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/paused")
	pausePoint(t, pool, paused.ID)
	if closed, err := repo.CloseWithReason(t.Context(), paused.ID, entity.CloseReasonGone); err != nil || !closed {
		t.Errorf("CloseWithReason() of a paused point = %v, %v, want true, nil", closed, err)
	}

	pending, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/pending", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending) failed: %v", err)
	}
	if closed, err := repo.CloseWithReason(t.Context(), pending.ID, entity.CloseReasonExpired); err != nil || closed {
		t.Errorf("CloseWithReason() of a pending point = %v, %v, want false, nil", closed, err)
	}
	if got := pointByID(t, repo, pending.ID); got.Status != entity.PointPending {
		t.Errorf("pending point = %s, want it left in the moderation queue", got.Status)
	}
	if closed, err := repo.CloseWithReason(t.Context(), uuid.New(), entity.CloseReasonExpired); err != nil || closed {
		t.Errorf("CloseWithReason() of a missing point = %v, %v, want false, nil", closed, err)
	}
}

func TestMineKeepsPausedAndSystemClosed(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	guest := seedUser(t, pool, 2)

	paused := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/paused")
	expired := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/expired")
	byHand := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/by-hand")
	enterPool(t, repo, guest, paused.ID)
	enterPool(t, repo, guest, expired.ID)

	pausePoint(t, pool, paused.ID)
	if _, err := repo.CloseWithReason(t.Context(), expired.ID, entity.CloseReasonExpired); err != nil {
		t.Fatalf("CloseWithReason() failed: %v", err)
	}
	if err := repo.Close(t.Context(), author, byHand.ID); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	created, _, err := repo.Mine(t.Context(), author)
	if err != nil {
		t.Fatalf("Mine(author) failed: %v", err)
	}
	mine := pointIDs(created)
	if !mine[paused.ID] || !mine[expired.ID] {
		t.Errorf("created list = %v, want the paused and the expired points", mine)
	}
	if mine[byHand.ID] {
		t.Error("created list shows a point the creator hid by hand")
	}

	_, entered, err := repo.Mine(t.Context(), guest)
	if err != nil {
		t.Fatalf("Mine(guest) failed: %v", err)
	}
	pools := pointIDs(entered)
	if !pools[paused.ID] {
		t.Errorf("entered list = %v, want the paused point still there", pools)
	}
	if pools[expired.ID] {
		t.Error("entered list shows a closed point")
	}

	if err := repo.Close(t.Context(), author, expired.ID); err != nil {
		t.Fatalf("Close() of a point closed by the system failed: %v", err)
	}
	created, _, err = repo.Mine(t.Context(), author)
	if err != nil {
		t.Fatalf("Mine(author) after hiding failed: %v", err)
	}
	if pointIDs(created)[expired.ID] {
		t.Error("created list still shows the hidden point")
	}
	if got := pointByID(t, repo, expired.ID); got.CloseReason == nil ||
		*got.CloseReason != string(entity.CloseReasonClosed) {
		t.Errorf("close_reason = %v, want it overwritten by the creator", pointer.Get(got.CloseReason))
	}
}

func TestTouchAliveClearsPing(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	live := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/live")
	stalePoint(t, pool, live.ID, 10)
	if err := repo.TouchAlive(t.Context(), live.ID); err != nil {
		t.Fatalf("TouchAlive() failed: %v", err)
	}
	if got := pointByID(t, repo, live.ID); !fresh(got) {
		t.Errorf("touched point = %v / %v, want the ping cleared and the point refreshed",
			got.LastSeenAlive, got.PingAskedAt)
	}

	closed := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/closed")
	stalePoint(t, pool, closed.ID, 10)
	if _, err := repo.CloseWithReason(t.Context(), closed.ID, entity.CloseReasonGone); err != nil {
		t.Fatalf("CloseWithReason() failed: %v", err)
	}
	if err := repo.TouchAlive(t.Context(), closed.ID); err != nil {
		t.Errorf("TouchAlive() of a closed point = %v, want nil", err)
	}
	if got := pointByID(t, repo, closed.ID); fresh(got) {
		t.Errorf("closed point = %v / %v, want it left untouched", got.LastSeenAlive, got.PingAskedAt)
	}
	if err := repo.TouchAlive(t.Context(), uuid.New()); err != nil {
		t.Errorf("TouchAlive() of a missing point = %v, want nil", err)
	}
}
