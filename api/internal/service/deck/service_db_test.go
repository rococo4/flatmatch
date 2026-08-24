package deck

import (
	"context"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/passesrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/pools"
)

const testTGBase int64 = -989000000

func cleanDeckUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	clean := func() {
		for _, q := range []string{
			`DELETE FROM passes WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_users WHERE from_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_users WHERE to_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM matches WHERE b_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up deck test rows: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

func seedDeckUser(t *testing.T, pool *pgxpool.Pool, n int64, gender, pref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(t.Context(), `
		INSERT INTO users (tg_id, name, status, age, gender, gender_pref, occupation,
			schedule, smoking, pets_own, pets_ok, noise, guests, cleaning, alcohol,
			expenses, about, about_status, photo_keys, photo_status)
		VALUES ($1, 'дек', 'active', 25, $2, $3, 'study', 'owl', 'no', 'none', 'ok',
			'normal', 'rare', 'schedule', 'none', 'split_equally', 'о себе', 'ok',
			ARRAY['photos/deck.jpg'], 'ok')
		RETURNING id`, testTGBase-n, gender, pref).Scan(&id)
	if err != nil {
		t.Fatalf("seed deck user: %v", err)
	}
	return id
}

func deckParams(zone string, min, max int) entity.SearchParams {
	return entity.SearchParams{
		ZoneID:       zone,
		PriceMin:     pointer.To(min),
		PriceMax:     max,
		MoveInMonths: []time.Time{sept},
	}
}

func TestDeckEndToEnd(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanDeckUsers(t, pool)

	searchesRepo := searchesrepo.New(pool)
	pointsRepo := pointsrepo.New(pool)
	passesRepo := passesrepo.New(pool)
	svc := New(searchesRepo, pointsRepo, passesRepo, usersrepo.New(pool),
		matchesrepo.New(pool), likesrepo.New(pool), testDir(t))

	viewer := seedDeckUser(t, pool, 1, "male", "any")
	mate := seedDeckUser(t, pool, 2, "female", "any")
	matched := seedDeckUser(t, pool, 3, "female", "any")
	host := seedDeckUser(t, pool, 4, "female", "any")

	for _, s := range []struct {
		user uuid.UUID
		p    entity.SearchParams
	}{
		{viewer, deckParams("zd-line", 35000, 45000)},
		{viewer, deckParams("zd-station-east", 35000, 45000)},
		{mate, deckParams("zd-district", 40000, 55000)},
		{mate, deckParams("zd-line", 40000, 55000)},
		{matched, deckParams("zd-district", 40000, 55000)},
	} {
		if _, err := searchesRepo.Create(t.Context(), s.user, s.p); err != nil {
			t.Fatalf("create search in %s: %v", s.p.ZoneID, err)
		}
	}

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO matches (a_id, b_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid))`,
		viewer, matched); err != nil {
		t.Fatalf("insert match: %v", err)
	}

	created, err := pointsRepo.Create(t.Context(), entity.Point{
		Type:       entity.PointIntentHome,
		Stage:      pointer.To(entity.StageLive),
		Status:     entity.PointActive,
		Source:     entity.SourceUserLink,
		CreatedBy:  &host,
		ZoneID:     "zd-station-east",
		PriceShare: pointer.To(42000),
		PhotoKeys:  []string{"points/room.jpg"},
	})
	if err != nil {
		t.Fatalf("create the room: %v", err)
	}

	got, err := svc.Deck(t.Context(), viewer, Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if got.NoSearch {
		t.Fatal("the viewer has two active searches, no_search must be false")
	}
	if len(got.Cards) != 2 || got.Total != 2 {
		t.Fatalf("deck = %+v, want one person card and one room", cardKinds(got.Cards))
	}

	person, roomCard := cardOf(t, got.Cards, KindPerson), cardOf(t, got.Cards, KindRoom)
	if person.Person.ID != mate {
		t.Errorf("person card = %s, want the mate %s — смэтченный скрыт", person.Person.ID, mate)
	}
	if person.Pair.Section != pools.SectionAll || len(person.Pair.Chips) == 0 {
		t.Errorf("pair = %+v, want a full match with chips", person.Pair)
	}
	if person.Person.Profile.About == "" || person.Person.Profile.Occupation != entity.OccupationStudy {
		t.Errorf("person card carries a thin profile: %+v", person.Person.Profile)
	}
	if roomCard.Room.Point.ID != created.ID || roomCard.Pair.Section != pools.SectionAll {
		t.Errorf("room card = %+v, want %s as a full match", roomCard.Pair, created.ID)
	}
	if roomCard.Room.Author == nil || roomCard.Room.Author.ID != host {
		t.Errorf("room card carries author %+v, want %s", roomCard.Room.Author, host)
	}

	if err := svc.PassPerson(t.Context(), viewer, mate); err != nil {
		t.Fatalf("PassPerson() failed: %v", err)
	}
	if err := svc.PassPoint(t.Context(), viewer, created.ID); err != nil {
		t.Fatalf("PassPoint() failed: %v", err)
	}

	after, err := svc.Deck(t.Context(), viewer, Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if len(after.Cards) != 0 || after.Total != 0 {
		t.Errorf("deck after two passes = %v, want empty", cardKinds(after.Cards))
	}

	restored, err := svc.ClearPasses(t.Context(), viewer)
	if err != nil {
		t.Fatalf("ClearPasses() failed: %v", err)
	}
	if restored != 2 {
		t.Errorf("ClearPasses() = %d, want 2", restored)
	}

	back, err := svc.Deck(t.Context(), viewer, Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if len(back.Cards) != 2 {
		t.Errorf("deck after «показать пропущенных» = %v, want both cards back", cardKinds(back.Cards))
	}
}

func TestDeckWithoutSearchesOnTheDatabase(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanDeckUsers(t, pool)

	svc := New(searchesrepo.New(pool), pointsrepo.New(pool), passesrepo.New(pool),
		usersrepo.New(pool), matchesrepo.New(pool), likesrepo.New(pool), testDir(t))
	viewer := seedDeckUser(t, pool, 1, "male", "any")

	got, err := svc.Deck(t.Context(), viewer, Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if !got.NoSearch || len(got.Cards) != 0 {
		t.Errorf("Deck() = %+v, want no_search", got)
	}
}

func cardKinds(cards []Card) []Kind {
	out := make([]Kind, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Kind)
	}
	return out
}

func cardOf(t *testing.T, cards []Card, kind Kind) Card {
	t.Helper()
	for _, c := range cards {
		if c.Kind == kind {
			return c
		}
	}
	t.Fatalf("no %s card in %v", kind, cardKinds(cards))
	return Card{}
}
