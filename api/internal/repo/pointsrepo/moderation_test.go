package pointsrepo

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func TestSetStatusMovesOnlyPendingPoints(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	approved, err := repo.Create(t.Context(),
		flatPoint(&user, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	rejected, err := repo.Create(t.Context(),
		flatPoint(&user, entity.SourceUserLink, "https://avito.ru/2", entity.PointPending))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	changed, err := repo.SetStatus(t.Context(), approved.ID, entity.PointActive, nil)
	if err != nil || !changed {
		t.Fatalf("SetStatus(active) = %v, %v, want true, nil", changed, err)
	}

	changed, err = repo.SetStatus(t.Context(), approved.ID, entity.PointClosed, pointer.To("rejected"))
	if err != nil || changed {
		t.Errorf("SetStatus() of an approved point = %v, %v, want false, nil", changed, err)
	}
	got, err := repo.ByID(t.Context(), approved.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.Status != entity.PointActive || got.CloseReason != nil {
		t.Errorf("approved point = %s / %v, want active with no close reason", got.Status, got.CloseReason)
	}

	changed, err = repo.SetStatus(t.Context(), rejected.ID, entity.PointClosed, pointer.To("rejected"))
	if err != nil || !changed {
		t.Fatalf("SetStatus(closed) = %v, %v, want true, nil", changed, err)
	}
	got, err = repo.ByID(t.Context(), rejected.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.Status != entity.PointClosed || got.CloseReason == nil || *got.CloseReason != "rejected" {
		t.Errorf("rejected point = %s / %v, want closed with reason rejected", got.Status, got.CloseReason)
	}

	if changed, err := repo.SetStatus(t.Context(), uuid.New(), entity.PointActive, nil); err != nil || changed {
		t.Errorf("SetStatus() of a missing point = %v, %v, want false, nil", changed, err)
	}
}

func TestPendingModerationListsBothForms(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	flat, err := repo.Create(t.Context(),
		flatPoint(&user, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(flat) failed: %v", err)
	}
	home := intentHome(user, entity.StageLive)
	home.Status = entity.PointPending
	pendingHome, err := repo.Create(t.Context(), home)
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}

	live, err := repo.Create(t.Context(),
		flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(active flat) failed: %v", err)
	}

	queue, err := repo.PendingModeration(t.Context())
	if err != nil {
		t.Fatalf("PendingModeration() failed: %v", err)
	}
	ids := make([]uuid.UUID, 0, len(queue))
	for _, p := range queue {
		ids = append(ids, p.ID)
	}
	if !slices.Contains(ids, flat.ID) || !slices.Contains(ids, pendingHome.ID) {
		t.Errorf("queue = %v, want both pending points (%s, %s)", ids, flat.ID, pendingHome.ID)
	}
	if slices.Contains(ids, live.ID) {
		t.Errorf("queue holds the active point %s", live.ID)
	}
}

func TestCardByIDMatchesTheFeed(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	mate := seedPerson(t, pool, 2, "male", "any")
	author := seedPerson(t, pool, 3, "male", "any")

	flat, err := repo.Create(t.Context(),
		flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(flat) failed: %v", err)
	}
	enterPool(t, repo, viewer, flat.ID)
	enterPool(t, repo, mate, flat.ID)

	card, err := repo.CardByID(t.Context(), viewer, flat.ID)
	if err != nil {
		t.Fatalf("CardByID() failed: %v", err)
	}
	if card.PoolCount != 1 || !card.InPool || card.Author != nil {
		t.Errorf("card = %+v, want pool 1, in_pool true and no author", card)
	}
	if card.GoneVotes != 0 || card.MyVote != nil {
		t.Errorf("card = %+v, want gone_votes 0 and no vote of my own", card)
	}

	pending, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending) failed: %v", err)
	}

	stranger, err := repo.CardByID(t.Context(), viewer, pending.ID)
	if err != nil || stranger.Point.ID != pending.ID || stranger.Point.Status != entity.PointPending {
		t.Errorf("CardByID() of a pending point = %+v, %v; want the row itself — гейт §5 в сервисе",
			stranger.Point, err)
	}

	if _, err := repo.CardByID(t.Context(), viewer, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("CardByID() of a missing point = %v, want ErrNotFound", err)
	}
}

func TestCardByIDHidesUnreachableRooms(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	hidden := seedPerson(t, pool, 2, "female", "same")
	matched := seedPerson(t, pool, 3, "male", "any")
	seedMatch(t, pool, viewer, matched)

	hiddenHome, err := repo.Create(t.Context(), intentHome(hidden, entity.StageLive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	matchedHome, err := repo.Create(t.Context(), intentHome(matched, entity.StageLive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	own := intentHome(viewer, entity.StageLive)
	own.Status = entity.PointPending
	ownHome, err := repo.Create(t.Context(), own)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if _, err := repo.CardByID(t.Context(), viewer, hiddenHome.ID); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("CardByID(invisible author) = %v, want ErrNotFound", err)
	}
	if _, err := repo.CardByID(t.Context(), viewer, matchedHome.ID); err != nil {
		t.Errorf("CardByID(matched author) = %v, want the room to stay visible after a match", err)
	}

	card, err := repo.CardByID(t.Context(), viewer, ownHome.ID)
	if err != nil {
		t.Fatalf("CardByID() of own room failed: %v", err)
	}
	if card.Author == nil || card.Author.ID != viewer {
		t.Errorf("own card = %+v, want the author attached", card)
	}
}

func TestSetStatusConfirmsFreshnessOnApproval(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1)

	backdate := func(id uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(t.Context(),
			`UPDATE points SET last_seen_alive = now() - interval '10 days' WHERE id = $1`, id); err != nil {
			t.Fatalf("backdate last_seen_alive: %v", err)
		}
	}

	approved, err := repo.Create(t.Context(),
		flatPoint(&user, entity.SourceUserLink, "https://avito.ru/1", entity.PointPending))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	backdate(approved.ID)

	if changed, err := repo.SetStatus(t.Context(), approved.ID, entity.PointActive, nil); err != nil || !changed {
		t.Fatalf("SetStatus(active) = %v, %v, want true, nil", changed, err)
	}
	got, err := repo.ByID(t.Context(), approved.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if got.LastSeenAlive.Before(time.Now().Add(-time.Minute)) {
		t.Errorf("last_seen_alive = %s, want the moment of approval", got.LastSeenAlive)
	}

	rejected, err := repo.Create(t.Context(),
		flatPoint(&user, entity.SourceUserLink, "https://avito.ru/2", entity.PointPending))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	backdate(rejected.ID)

	if changed, err := repo.SetStatus(t.Context(), rejected.ID, entity.PointClosed, pointer.To("rejected")); err != nil || !changed {
		t.Fatalf("SetStatus(closed) = %v, %v, want true, nil", changed, err)
	}
	stale, err := repo.ByID(t.Context(), rejected.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if !stale.LastSeenAlive.Before(time.Now().Add(-24 * time.Hour)) {
		t.Errorf("last_seen_alive of a rejected point = %s, want the backdated value", stale.LastSeenAlive)
	}
}
