package searchesrepo

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

func setProfile(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, age int,
	occupation, schedule, smoking, petsOwn, petsOK string,
) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		UPDATE users SET age = $2, occupation = $3, schedule = $4, smoking = $5,
			pets_own = $6, pets_ok = $7, about = 'сова и кот', about_status = 'ok'
		WHERE id = $1`, id, age, occupation, schedule, smoking, petsOwn, petsOK)
	if err != nil {
		t.Fatalf("set profile of %s: %v", id, err)
	}
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

func candidateIDs(list []DeckCandidate) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(list))
	for _, c := range list {
		out = append(out, c.User.ID)
	}
	return out
}

func hasCandidate(list []DeckCandidate, id uuid.UUID) (DeckCandidate, bool) {
	for _, c := range list {
		if c.User.ID == id {
			return c, true
		}
	}
	return DeckCandidate{}, false
}

func TestDeckCandidatesGroupsSearchesAndCutsTheExcluded(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	viewer := seedUser(t, pool, 1, "active", "male", "any")
	twoSearches := seedUser(t, pool, 2, "active", "female", "any")
	matched := seedUser(t, pool, 3, "active", "female", "any")
	liked := seedUser(t, pool, 4, "active", "female", "any")
	passed := seedUser(t, pool, 5, "active", "female", "any")
	longAgoPassed := seedUser(t, pool, 6, "active", "female", "any")
	invisible := seedUser(t, pool, 7, "active", "female", "same")
	elsewhere := seedUser(t, pool, 8, "active", "female", "any")
	for _, id := range []uuid.UUID{viewer, twoSearches, matched, liked, passed,
		longAgoPassed, invisible, elsewhere} {
		setProfile(t, pool, id, 25, "study", "owl", "no", "none", "ok")
	}

	mustSeed(t, repo, viewer, "z-deck-a")
	first := mustSeed(t, repo, twoSearches, "z-deck-a")
	second := mustSeed(t, repo, twoSearches, "z-deck-b")
	closed := mustSeed(t, repo, twoSearches, "z-deck-c")
	if _, err := repo.Close(t.Context(), twoSearches, closed.ID, entity.CloseFoundHere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	for _, id := range []uuid.UUID{matched, liked, passed, longAgoPassed, invisible} {
		mustSeed(t, repo, id, "z-deck-a")
	}
	mustSeed(t, repo, elsewhere, "z-deck-c")

	seedMatch(t, pool, viewer, matched, time.Hour)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO likes_users (from_id, to_id, zone_id, spent)
		VALUES ($1, $2, 'z-deck-a', true)`, viewer, liked); err != nil {
		t.Fatalf("insert like: %v", err)
	}
	pass(t, pool, viewer, passed, time.Hour)
	pass(t, pool, viewer, longAgoPassed, 8*24*time.Hour)

	got, err := repo.DeckCandidates(t.Context(), viewer,
		[]string{"z-deck-a", "z-deck-b"}, entity.PeopleFilter{}, deckWindow)
	if err != nil {
		t.Fatalf("DeckCandidates() failed: %v", err)
	}

	for _, id := range []uuid.UUID{viewer, matched, liked, passed, invisible, elsewhere} {
		if _, ok := hasCandidate(got, id); ok {
			t.Errorf("user %s must not be in the deck: %v", id, candidateIDs(got))
		}
	}
	if _, ok := hasCandidate(got, longAgoPassed); !ok {
		t.Error("a pass older than the window must expire on its own")
	}

	cand, ok := hasCandidate(got, twoSearches)
	if !ok {
		t.Fatalf("the candidate with two searches is missing: %v", candidateIDs(got))
	}
	if len(cand.Searches) != 2 {
		t.Fatalf("candidate carries %d searches, want both active ones", len(cand.Searches))
	}
	ids := map[uuid.UUID]bool{cand.Searches[0].ID: true, cand.Searches[1].ID: true}
	if !ids[first.ID] || !ids[second.ID] {
		t.Errorf("searches = %v, want %s and %s", ids, first.ID, second.ID)
	}
	if cand.User.Profile.Occupation != entity.OccupationStudy || cand.User.Profile.About == "" {
		t.Errorf("candidate profile is thin: %+v", cand.User.Profile)
	}
}

func TestDeckCandidatesFilters(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	viewer := seedUser(t, pool, 1, "active", "male", "any")
	setProfile(t, pool, viewer, 25, "study", "owl", "no", "none", "ok")
	mustSeed(t, repo, viewer, "z-deck-a")

	student := seedUser(t, pool, 2, "active", "female", "any")
	setProfile(t, pool, student, 20, "study", "owl", "no", "none", "ok")
	worker := seedUser(t, pool, 3, "active", "female", "any")
	setProfile(t, pool, worker, 30, "hybrid", "lark", "outside", "cat", "against")
	idle := seedUser(t, pool, 4, "active", "female", "any")
	setProfile(t, pool, idle, 40, "none", "floating", "home", "dog", "ok")
	for _, id := range []uuid.UUID{student, worker, idle} {
		mustSeed(t, repo, id, "z-deck-a")
	}

	cases := map[string]struct {
		filter entity.PeopleFilter
		want   []uuid.UUID
	}{
		"без фильтров":        {entity.PeopleFilter{}, []uuid.UUID{student, worker, idle}},
		"возраст":             {entity.PeopleFilter{AgeMin: pointer.To(25), AgeMax: pointer.To(35)}, []uuid.UUID{worker}},
		"учится или работает": {entity.PeopleFilter{Occupation: []string{"study", "office", "hybrid", "remote", "shift"}}, []uuid.UUID{student, worker}},
		"не работает":         {entity.PeopleFilter{Occupation: []string{"none"}}, []uuid.UUID{idle}},
		"сова":                {entity.PeopleFilter{Schedule: []string{"owl"}}, []uuid.UUID{student}},
		"сова или жаворонок":  {entity.PeopleFilter{Schedule: []string{"owl", "lark"}}, []uuid.UUID{student, worker}},
		"не курит дома":       {entity.PeopleFilter{Smoking: []string{"no", "outside"}}, []uuid.UUID{student, worker}},
		"совсем не курит":     {entity.PeopleFilter{Smoking: []string{"no"}}, []uuid.UUID{student}},
		"без животных":        {entity.PeopleFilter{PetsOwnNone: true}, []uuid.UUID{student}},
		"не против моих":      {entity.PeopleFilter{PetsOKFine: true}, []uuid.UUID{student, idle}},
		"животные или-или":    {entity.PeopleFilter{PetsOwnNone: true, PetsOKFine: true}, []uuid.UUID{student, idle}},
		"пересечение и-и":     {entity.PeopleFilter{PetsOKFine: true, Schedule: []string{"floating"}}, []uuid.UUID{idle}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := repo.DeckCandidates(t.Context(), viewer, []string{"z-deck-a"}, tc.filter, deckWindow)
			if err != nil {
				t.Fatalf("DeckCandidates() failed: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", candidateIDs(got), tc.want)
			}
			for _, id := range tc.want {
				if _, ok := hasCandidate(got, id); !ok {
					t.Errorf("user %s is missing from %v", id, candidateIDs(got))
				}
			}
		})
	}
}

func TestActiveByUsers(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	one := seedUser(t, pool, 1, "active", "male", "any")
	two := seedUser(t, pool, 2, "active", "female", "any")
	stranger := seedUser(t, pool, 3, "active", "female", "any")

	live := mustSeed(t, repo, one, "z-deck-a")
	gone := mustSeed(t, repo, one, "z-deck-b")
	if _, err := repo.Close(t.Context(), one, gone.ID, entity.CloseChangedMind); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	theirs := mustSeed(t, repo, two, "z-deck-c")
	mustSeed(t, repo, stranger, "z-deck-a")

	got, err := repo.ActiveByUsers(t.Context(), []uuid.UUID{one, two})
	if err != nil {
		t.Fatalf("ActiveByUsers() failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d searches, want the two active ones", len(got))
	}
	ids := map[uuid.UUID]bool{got[0].ID: true, got[1].ID: true}
	if !ids[live.ID] || !ids[theirs.ID] {
		t.Errorf("searches = %v, want %s and %s", ids, live.ID, theirs.ID)
	}

	empty, err := repo.ActiveByUsers(t.Context(), nil)
	if err != nil {
		t.Fatalf("ActiveByUsers(nil) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ActiveByUsers(nil) = %v, want nothing", empty)
	}
}

func mustSeed(t *testing.T, repo *Repo, userID uuid.UUID, zone string) entity.Search {
	t.Helper()
	s, err := repo.Create(t.Context(), userID, testParams(zone))
	if err != nil {
		t.Fatalf("create search in %s: %v", zone, err)
	}
	return s
}
