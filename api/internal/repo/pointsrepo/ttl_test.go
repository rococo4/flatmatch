package pointsrepo

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

var (
	pingEvery  = time.Duration(entity.PingEveryDays) * 24 * time.Hour
	pointTTL   = time.Duration(entity.PointTTLDays) * 24 * time.Hour
	pingAnswer = time.Duration(entity.PingAnswerDays) * 24 * time.Hour
)

func backdatePoint(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, column string, daysAgo int) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`UPDATE points SET `+column+` = now() - make_interval(days => $2) WHERE id = $1`, id, daysAgo)
	if err != nil {
		t.Fatalf("backdate %s of point %s: %v", column, id, err)
	}
}

func sentPing(t *testing.T, pool *pgxpool.Pool, userID, pointID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO notifications (user_id, type, actor_key, context_key, payload, sent_at)
		SELECT $1, 'point_ping', $2::text || ':' || extract(epoch FROM last_seen_alive)::bigint,
		       $2::text, '{}'::jsonb, ping_asked_at + interval '10 seconds'
		FROM points WHERE id = $2::uuid`, userID, pointID)
	if err != nil {
		t.Fatalf("seed the sent ping of point %s: %v", pointID, err)
	}
}

func askedIDs(t *testing.T, repo *Repo) map[uuid.UUID]bool {
	t.Helper()
	points, err := repo.PointsForPing(t.Context(), pingEvery)
	if err != nil {
		t.Fatalf("PointsForPing() failed: %v", err)
	}
	out := map[uuid.UUID]bool{}
	for _, p := range points {
		out[p.ID] = true
	}
	return out
}

func expiredIDs(t *testing.T, repo *Repo) map[uuid.UUID]bool {
	t.Helper()
	ids, err := repo.Expire(t.Context(), pointTTL, pingAnswer)
	if err != nil {
		t.Fatalf("Expire() failed: %v", err)
	}
	out := map[uuid.UUID]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func TestPointsForPingAsksOwnedLivePointsWeekly(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)

	today := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/today")
	mine := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/mine")
	hidden := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/hidden")
	market := activeFlat(t, repo, nil, entity.SourceTGChannel, "https://cian.ru/market")
	room, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}

	for _, id := range []uuid.UUID{mine.ID, hidden.ID, market.ID, room.ID} {
		backdatePoint(t, pool, id, "last_seen_alive", entity.PingEveryDays+1)
	}
	pausePoint(t, pool, hidden.ID)

	asked := askedIDs(t, repo)
	if !asked[mine.ID] || !asked[room.ID] {
		t.Errorf("PointsForPing() = %v, want the week-old flat and room in it", asked)
	}
	if asked[today.ID] {
		t.Error("PointsForPing() asks about a point confirmed today")
	}
	if asked[market.ID] {
		t.Error("PointsForPing() asks about a point without a creator")
	}
	if asked[hidden.ID] {
		t.Error("PointsForPing() asks about a point hidden by votes")
	}

	if err := repo.MarkPingAsked(t.Context(), mine.ID); err != nil {
		t.Fatalf("MarkPingAsked() failed: %v", err)
	}
	got := pointByID(t, repo, mine.ID)
	if got.PingAskedAt == nil || time.Since(*got.PingAskedAt) > time.Minute {
		t.Errorf("ping_asked_at = %v, want it stamped now", got.PingAskedAt)
	}
	if askedIDs(t, repo)[mine.ID] {
		t.Error("PointsForPing() asks the same point twice in a row")
	}

	backdatePoint(t, pool, mine.ID, "ping_asked_at", entity.PingEveryDays+1)
	if !askedIDs(t, repo)[mine.ID] {
		t.Error("PointsForPing() never asks again after a week of silence")
	}
}

func TestExpireArchivesStaleAndSilentPoints(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)
	other := seedUser(t, pool, 2)

	stale := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/stale")
	hidden := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/hidden")
	silent := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/silent")
	unsent := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/unsent")
	answered := activeFlat(t, repo, &other, entity.SourceUserLink, "https://avito.ru/answered")
	queued, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/queued", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending) failed: %v", err)
	}

	for _, id := range []uuid.UUID{stale.ID, hidden.ID, queued.ID} {
		backdatePoint(t, pool, id, "last_seen_alive", entity.PointTTLDays+1)
	}
	pausePoint(t, pool, hidden.ID)
	for _, id := range []uuid.UUID{silent.ID, unsent.ID} {
		backdatePoint(t, pool, id, "last_seen_alive", entity.PingEveryDays+1)
		backdatePoint(t, pool, id, "ping_asked_at", entity.PingAnswerDays+1)
	}
	sentPing(t, pool, author, silent.ID)
	backdatePoint(t, pool, answered.ID, "ping_asked_at", entity.PingAnswerDays+1)
	sentPing(t, pool, other, answered.ID)

	expired := expiredIDs(t, repo)
	if !expired[stale.ID] || !expired[hidden.ID] || !expired[silent.ID] {
		t.Errorf("Expire() = %v, want the stale, the hidden and the silent points archived", expired)
	}
	if expired[unsent.ID] {
		t.Error("Expire() archived a point whose ping never went out")
	}
	if expired[answered.ID] {
		t.Error("Expire() archived a point confirmed after the ping")
	}
	if expired[queued.ID] {
		t.Error("Expire() archived a point waiting for moderation")
	}

	for _, id := range []uuid.UUID{stale.ID, hidden.ID, silent.ID} {
		got := pointByID(t, repo, id)
		if got.Status != entity.PointClosed ||
			pointer.Get(got.CloseReason) != string(entity.CloseReasonExpired) {
			t.Errorf("archived point = %s / %v, want closed as expired",
				got.Status, pointer.Get(got.CloseReason))
		}
	}
	for _, id := range []uuid.UUID{unsent.ID, answered.ID} {
		if got := pointByID(t, repo, id); got.Status != entity.PointActive {
			t.Errorf("point %s = %s, want it still active", id, got.Status)
		}
	}
	if got := pointByID(t, repo, queued.ID); got.Status != entity.PointPending {
		t.Errorf("point on moderation = %s, want it left in the queue", got.Status)
	}

	if again := expiredIDs(t, repo); again[stale.ID] || again[hidden.ID] || again[silent.ID] {
		t.Errorf("Expire() = %v, want it to archive nothing twice", again)
	}
}

func TestFreshnessSkipsSwitchedOffFlats(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	author := seedUser(t, pool, 1)

	flat := activeFlat(t, repo, &author, entity.SourceUserLink, "https://avito.ru/switched-off")
	room, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	for _, id := range []uuid.UUID{flat.ID, room.ID} {
		backdatePoint(t, pool, id, "last_seen_alive", entity.PointTTLDays+1)
	}
	closePoint(t, pool, flat.ID)

	asked := askedIDs(t, repo)
	if asked[flat.ID] {
		t.Error("PointsForPing() pings a closed market flat")
	}
	if !asked[room.ID] {
		t.Errorf("PointsForPing() = %v, want the room still pinged", asked)
	}

	expired := expiredIDs(t, repo)
	if expired[flat.ID] {
		t.Error("Expire() touches a closed market flat")
	}
	if !expired[room.ID] {
		t.Errorf("Expire() = %v, want the stale room archived", expired)
	}
	if got := pointByID(t, repo, flat.ID); got.Status != entity.PointClosed ||
		pointer.Get(got.CloseReason) != string(entity.CloseReasonClosed) {
		t.Errorf("closed flat = %s / %v, want it left closed",
			got.Status, pointer.Get(got.CloseReason))
	}
}
