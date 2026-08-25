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

const deckWindow = 7 * 24 * time.Hour

const otherZone = "s-test-points-far"

func roomIDs(list []FeedPoint) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(list))
	for _, p := range list {
		out = append(out, p.Point.ID)
	}
	return out
}

func hasRoom(list []FeedPoint, id uuid.UUID) bool {
	for _, p := range list {
		if p.Point.ID == id {
			return true
		}
	}
	return false
}

func pass(t *testing.T, pool *pgxpool.Pool, userID, targetID uuid.UUID, ago time.Duration) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO passes (user_id, target_id, created_at)
		VALUES ($1, $2, now() - make_interval(secs => $3))`, userID, targetID, ago.Seconds())
	if err != nil {
		t.Fatalf("insert pass: %v", err)
	}
}

func TestDeckRoomsShowsLiveRoomsOfVisibleStrangers(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	viewer := seedUser(t, pool, 1)
	stranger := seedUser(t, pool, 2)
	matched := seedUser(t, pool, 3)
	liked := seedUser(t, pool, 4)
	passed := seedUser(t, pool, 5)
	invisible := seedUser(t, pool, 6)
	waiting := seedUser(t, pool, 7)
	distant := seedUser(t, pool, 8)
	if _, err := pool.Exec(t.Context(),
		`UPDATE users SET gender = 'female', gender_pref = 'same' WHERE id = $1`, invisible); err != nil {
		t.Fatalf("hide the invisible author: %v", err)
	}

	room := mustCreate(t, repo, intentHome(stranger, entity.StageLive))
	matchedRoom := mustCreate(t, repo, intentHome(matched, entity.StageLive))
	likedRoom := mustCreate(t, repo, intentHome(liked, entity.StageLive))
	passedRoom := mustCreate(t, repo, intentHome(passed, entity.StageLive))
	hiddenRoom := mustCreate(t, repo, intentHome(invisible, entity.StageLive))
	mine := mustCreate(t, repo, intentHome(viewer, entity.StageLive))

	pending := intentHome(waiting, entity.StageMovingIn)
	pending.Status = entity.PointPending
	pendingRoom := mustCreate(t, repo, pending)

	far := intentHome(distant, entity.StageMovingIn)
	far.ZoneID = otherZone
	farRoom := mustCreate(t, repo, far)

	flat := mustCreate(t, repo, flatPoint(&stranger, entity.SourceUserLink, "https://avito.ru/deck", entity.PointActive))

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO matches (a_id, b_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid))`, viewer, matched); err != nil {
		t.Fatalf("insert match: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO likes_users (from_id, to_id, point_id, spent)
		VALUES ($1, $2, $3, true)`, viewer, liked, likedRoom.ID); err != nil {
		t.Fatalf("insert like: %v", err)
	}
	pass(t, pool, viewer, passedRoom.ID, time.Hour)

	got, err := repo.DeckRooms(t.Context(), viewer, []string{testZone}, entity.PeopleFilter{}, deckWindow)
	if err != nil {
		t.Fatalf("DeckRooms() failed: %v", err)
	}

	if !hasRoom(got, room.ID) {
		t.Fatalf("the live room of a stranger is missing: %v", roomIDs(got))
	}
	for _, id := range []uuid.UUID{matchedRoom.ID, likedRoom.ID, passedRoom.ID, hiddenRoom.ID,
		mine.ID, pendingRoom.ID, farRoom.ID, flat.ID} {
		if hasRoom(got, id) {
			t.Errorf("point %s must not be in the deck: %v", id, roomIDs(got))
		}
	}
	if len(got) != 1 {
		t.Fatalf("deck holds %d rooms, want 1", len(got))
	}
	if got[0].Author == nil || got[0].Author.ID != stranger {
		t.Errorf("room carries author %+v, want %s", got[0].Author, stranger)
	}

	pass(t, pool, viewer, room.ID, 8*24*time.Hour)
	back, err := repo.DeckRooms(t.Context(), viewer, []string{testZone}, entity.PeopleFilter{}, deckWindow)
	if err != nil {
		t.Fatalf("DeckRooms() failed: %v", err)
	}
	if !hasRoom(back, room.ID) {
		t.Error("a pass older than the window must expire on its own")
	}
}

func TestDeckRoomsHidesRoomsOfPassedAuthors(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	viewer := seedUser(t, pool, 1)
	passed := seedUser(t, pool, 2)
	longAgoPassed := seedUser(t, pool, 3)
	stranger := seedUser(t, pool, 4)

	passedRoom := mustCreate(t, repo, intentHome(passed, entity.StageLive))
	longAgoRoom := mustCreate(t, repo, intentHome(longAgoPassed, entity.StageLive))
	strangerRoom := mustCreate(t, repo, intentHome(stranger, entity.StageLive))

	pass(t, pool, viewer, passed, time.Hour)
	pass(t, pool, viewer, longAgoPassed, 8*24*time.Hour)

	got, err := repo.DeckRooms(t.Context(), viewer, []string{testZone}, entity.PeopleFilter{}, deckWindow)
	if err != nil {
		t.Fatalf("DeckRooms() failed: %v", err)
	}
	if hasRoom(got, passedRoom.ID) {
		t.Errorf("the room of a passed author must be gone too: %v", roomIDs(got))
	}
	if !hasRoom(got, longAgoRoom.ID) {
		t.Errorf("a pass older than the window must expire on its own: %v", roomIDs(got))
	}
	if !hasRoom(got, strangerRoom.ID) {
		t.Errorf("the room of an untouched author is missing: %v", roomIDs(got))
	}
}

func TestDeckRoomsFiltersByAuthorProfile(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	viewer := seedUser(t, pool, 1)
	owl := seedUser(t, pool, 2)
	lark := seedUser(t, pool, 3)
	if _, err := pool.Exec(t.Context(), `
		UPDATE users SET age = 20, schedule = 'owl', smoking = 'no', pets_own = 'none'
		WHERE id = $1`, owl); err != nil {
		t.Fatalf("set profile: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE users SET age = 40, schedule = 'lark', smoking = 'home', pets_own = 'cat'
		WHERE id = $1`, lark); err != nil {
		t.Fatalf("set profile: %v", err)
	}

	owlRoom := mustCreate(t, repo, intentHome(owl, entity.StageLive))
	larkRoom := mustCreate(t, repo, intentHome(lark, entity.StageLive))

	got, err := repo.DeckRooms(t.Context(), viewer, []string{testZone},
		entity.PeopleFilter{Schedule: []string{"owl"}, AgeMax: pointer.To(30)}, deckWindow)
	if err != nil {
		t.Fatalf("DeckRooms() failed: %v", err)
	}
	if !hasRoom(got, owlRoom.ID) || hasRoom(got, larkRoom.ID) {
		t.Errorf("filtered deck = %v, want only the owl room %s", roomIDs(got), owlRoom.ID)
	}
}

func mustCreate(t *testing.T, repo *Repo, p entity.Point) entity.Point {
	t.Helper()
	created, err := repo.Create(t.Context(), p)
	if err != nil {
		t.Fatalf("create %s point: %v", p.Type, err)
	}
	return created
}
