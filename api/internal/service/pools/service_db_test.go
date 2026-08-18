package pools

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/service/zones"
)

const testTGBase int64 = -997000000

const sharedByDefault = 7

var (
	moveSept = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	moveDec  = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
)

type staticPhotos struct{}

type testUser struct {
	n        int64
	gender   entity.Gender
	pref     entity.GenderPref
	schedule entity.Schedule
	smoking  entity.Smoking
	photoKey string
	photoOK  bool
}

func (staticPhotos) URL(key string) string {
	if key == "" {
		return ""
	}
	return "/media/" + key
}

func TestZonePoolEndToEnd(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo, photoKey: "viewer.jpg", photoOK: true})
	full := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo, photoKey: "full.jpg", photoOK: true})
	partial := seedUser(t, pool, testUser{n: 3, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleLark, smoking: entity.SmokingHome, photoKey: "partial.jpg", photoOK: true})

	none := seedUser(t, pool, testUser{n: 4, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleLark, smoking: entity.SmokingHome, photoKey: "none.jpg"})
	invisible := seedUser(t, pool, testUser{n: 5, gender: entity.GenderFemale, pref: entity.GenderPrefSame,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo, photoKey: "invisible.jpg", photoOK: true})

	mine := mustCreate(t, repo, viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept))
	mustCreate(t, repo, full.ID, zoneParams("pz-district", 40000, 55000, moveSept))

	mustCreate(t, repo, full.ID, zoneParams("pz-line-east", 90000, 120000, moveDec))

	mustCreate(t, repo, partial.ID, zoneParams("pz-line-west", 60000, 80000, moveSept))
	mustCreate(t, repo, none.ID, zoneParams("pz-station-east", 90000, 120000, moveDec))
	mustCreate(t, repo, invisible.ID, zoneParams("pz-district", 40000, 45000, moveSept))

	got, err := svc.ZonePool(t.Context(), viewer, "pz-district")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}

	if got.Zone.ID != "pz-district" {
		t.Errorf("pool zone = %q, want pz-district", got.Zone.ID)
	}
	if !got.Ranked {
		t.Fatal("viewer has a search here, the pool must be ranked")
	}
	if got.MySearch == nil || got.MySearch.ID != mine.ID {
		t.Fatalf("MySearch = %+v, want the search of this very zone (%s)", got.MySearch, mine.ID)
	}
	if len(got.People) != 0 {
		t.Errorf("ranked pool must not fill the flat list too: %+v", got.People)
	}
	wantOrder := []SectionID{SectionAll, SectionPartial, SectionNone}
	if len(got.Sections) != len(wantOrder) {
		t.Fatalf("got %d sections, want 3", len(got.Sections))
	}
	for i, id := range wantOrder {
		if got.Sections[i].ID != id {
			t.Errorf("section %d = %q, want %q", i, got.Sections[i].ID, id)
		}
	}

	fullCard := assertSectionHas(t, got, SectionAll, full.ID)

	if fullCard.Params.PriceMax != 55000 {
		t.Errorf("full match card shows params %+v, want the 40–55k search", fullCard.Params)
	}
	if len(fullCard.Chips) == 0 {
		t.Error("ranked card without verdict chips")
	}
	if fullCard.SharedAnswers != sharedByDefault+2 {
		t.Errorf("SharedAnswers = %d, want %d (same schedule and smoking)",
			fullCard.SharedAnswers, sharedByDefault+2)
	}
	if fullCard.PhotoURL != "/media/full.jpg" {
		t.Errorf("PhotoURL = %q, want /media/full.jpg", fullCard.PhotoURL)
	}

	partialCard := assertSectionHas(t, got, SectionPartial, partial.ID)
	if partialCard.SharedAnswers != sharedByDefault {
		t.Errorf("SharedAnswers of the partial match = %d, want %d",
			partialCard.SharedAnswers, sharedByDefault)
	}

	if partialCard.Zone.ID != "pz-line-west" || partialCard.Zone.NameIn == "" {
		t.Errorf("partial card zone = %+v, want pz-line-west with declensions", partialCard.Zone)
	}
	noneCard := assertSectionHas(t, got, SectionNone, none.ID)
	if noneCard.PhotoURL != "" {
		t.Errorf("unapproved photo leaked into the pool: %q", noneCard.PhotoURL)
	}

	assertNowhere(t, got, "gender_pref cut them off", invisible.ID)
	assertNowhere(t, got, "nobody is in their own pool", viewer.ID)
	if n := len(got.Sections[0].People) + len(got.Sections[1].People) + len(got.Sections[2].People); n != 3 {
		t.Errorf("pool holds %d cards, want 3 — one per person", n)
	}

	counts, err := svc.Counts(t.Context(), viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept), nil)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if counts.People != 3 || counts.FullMatch != 1 || counts.NewCount != 0 {
		t.Errorf("Counts = %+v, want people=3 full=1 new=0", counts)
	}

	wantAvatars := []string{"/media/partial.jpg", "/media/full.jpg"}
	if !slices.Equal(counts.Avatars, wantAvatars) {
		t.Errorf("Avatars = %v, want %v — approved only, freshest first",
			counts.Avatars, wantAvatars)
	}
}

func TestZonePoolHidesClosedAndPausedSearches(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	stays := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	quitter := seedUser(t, pool, testUser{n: 3, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	silent := seedUser(t, pool, testUser{n: 4, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})

	mustCreate(t, repo, viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept))
	mustCreate(t, repo, stays.ID, zoneParams("pz-district", 40000, 55000, moveSept))

	closed := mustCreate(t, repo, quitter.ID, zoneParams("pz-district", 40000, 55000, moveSept))
	if _, err := repo.Close(t.Context(), quitter.ID, closed.ID, entity.CloseFoundHere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	paused := mustCreate(t, repo, silent.ID, zoneParams("pz-district", 40000, 55000, moveSept))
	pause(t, pool, paused.ID)

	got, err := svc.ZonePool(t.Context(), viewer, "pz-district")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}
	assertSectionHas(t, got, SectionAll, stays.ID)
	assertNowhere(t, got, "their search is closed", quitter.ID)
	assertNowhere(t, got, "their search is paused by TTL", silent.ID)

	counts, err := svc.Counts(t.Context(), viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept), nil)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if counts.People != 1 || counts.FullMatch != 1 {
		t.Errorf("Counts = %+v, want people=1 full=1 — only the active search counts", counts)
	}
}

func TestZonePoolRanksFromAnIntersectingSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	other := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	mustCreate(t, repo, viewer.ID, zoneParams("pz-line-west", 35000, 45000, moveSept))
	mustCreate(t, repo, other.ID, zoneParams("pz-district", 40000, 55000, moveSept))

	got, err := svc.ZonePool(t.Context(), viewer, "pz-district")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}
	if !got.Ranked {
		t.Fatal("an intersecting search must still rank the pool")
	}
	if got.MySearch != nil {
		t.Errorf("MySearch = %+v, want nil — the search is in another zone", got.MySearch)
	}
	assertSectionHas(t, got, SectionAll, other.ID)
}

func TestZonePoolNeedsOwnSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	other := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	mustCreate(t, repo, other.ID, entity.SearchParams{ZoneID: "pz-district", PriceMax: 45000})

	if _, err := svc.ZonePool(t.Context(), viewer, "pz-district"); !errors.Is(err, entity.ErrNeedSearch) {
		t.Fatalf("ZonePool() = %v, want ErrNeedSearch — пул закрыт без своего поиска", err)
	}

	mustCreate(t, repo, viewer.ID, entity.SearchParams{ZoneID: "pz-station-west", PriceMax: 45000})
	if _, err := svc.ZonePool(t.Context(), viewer, "pz-district"); !errors.Is(err, entity.ErrNeedSearch) {
		t.Fatalf("ZonePool() = %v, want ErrNeedSearch — чужая зона пул не открывает", err)
	}
}

func TestPoolAndCountsHideMatchedPair(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	matched := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	stranger := seedUser(t, pool, testUser{n: 3, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleLark, smoking: entity.SmokingHome})

	params := zoneParams("pz-district", 35000, 45000, moveSept)
	mustCreate(t, repo, viewer.ID, params)
	mustCreate(t, repo, matched.ID, zoneParams("pz-district", 40000, 55000, moveSept))
	mustCreate(t, repo, stranger.ID, zoneParams("pz-district", 40000, 55000, moveSept))

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO matches (a_id, b_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid))`,
		viewer.ID, matched.ID); err != nil {
		t.Fatalf("insert match: %v", err)
	}

	got, err := svc.ZonePool(t.Context(), viewer, "pz-district")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}
	assertSectionHas(t, got, SectionAll, stranger.ID)
	assertNowhere(t, got, "смэтченный скрыт из пула", matched.ID)

	counts, err := svc.Counts(t.Context(), viewer.ID, params, nil)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if counts.People != 1 {
		t.Errorf("Counts.People = %d, want 1 — смэтченный не считается", counts.People)
	}
}

func TestPoolSeenResetsNewCount(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	newcomer := seedUser(t, pool, testUser{n: 2, gender: entity.GenderFemale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	mine := mustCreate(t, repo, viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept))
	mustCreate(t, repo, newcomer.ID, zoneParams("pz-district", 40000, 55000, moveSept))

	p := zoneParams("pz-district", 35000, 45000, moveSept)
	before, err := svc.Counts(t.Context(), viewer.ID, p, &mine.PoolSeenAt)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if before.NewCount != 1 {
		t.Errorf("NewCount before the visit = %d, want 1", before.NewCount)
	}

	if _, err := svc.ZonePool(t.Context(), viewer, "pz-district"); err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}

	seen, err := repo.ByID(t.Context(), viewer.ID, mine.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if !seen.PoolSeenAt.After(mine.PoolSeenAt) {
		t.Fatalf("looking at the pool did not move pool_seen_at: %v then %v",
			mine.PoolSeenAt, seen.PoolSeenAt)
	}
	after, err := svc.Counts(t.Context(), viewer.ID, p, &seen.PoolSeenAt)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if after.NewCount != 0 || after.People != 1 {
		t.Errorf("Counts after the visit = %+v, want people=1 new=0", after)
	}
}

func TestZonePoolUnknownZone(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	svc := New(searchesrepo.New(pool), testDir(t), staticPhotos{})
	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny})

	if _, err := svc.ZonePool(t.Context(), viewer, "pz-nowhere"); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ZonePool() on an unknown zone = %v, want ErrNotFound", err)
	}
	if _, err := svc.Counts(t.Context(), viewer.ID,
		entity.SearchParams{ZoneID: "pz-nowhere", PriceMax: 45000}, nil); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Counts() on an unknown zone = %v, want ErrNotFound", err)
	}
}

func TestNoneSectionAndAvatarsAreCapped(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{})

	viewer := seedUser(t, pool, testUser{n: 1, gender: entity.GenderMale, pref: entity.GenderPrefAny,
		schedule: entity.ScheduleOwl, smoking: entity.SmokingNo})
	mustCreate(t, repo, viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept))

	const crowd = noneSectionCap + 1
	for i := range int64(crowd) {
		other := seedUser(t, pool, testUser{n: 10 + i, gender: entity.GenderMale,
			pref: entity.GenderPrefAny, schedule: entity.ScheduleOwl,
			smoking: entity.SmokingNo, photoKey: "crowd.jpg", photoOK: true})

		mustCreate(t, repo, other.ID, zoneParams("pz-district", 90000, 120000, moveDec))
	}

	got, err := svc.ZonePool(t.Context(), viewer, "pz-district")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}
	none := got.Sections[2]
	if none.ID != SectionNone {
		t.Fatalf("third section is %q, want none", none.ID)
	}
	if len(none.People) != noneSectionCap {
		t.Errorf("«не совпали» holds %d cards, want the cap of %d", len(none.People), noneSectionCap)
	}
	if len(got.Sections[0].People) != 0 || len(got.Sections[1].People) != 0 {
		t.Errorf("nobody matches here, yet the upper sections are not empty: %+v", got.Sections)
	}

	counts, err := svc.Counts(t.Context(), viewer.ID, zoneParams("pz-district", 35000, 45000, moveSept), nil)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if counts.People != crowd || counts.FullMatch != 0 {
		t.Errorf("Counts = %+v, want the uncapped people=%d full=0", counts, crowd)
	}
	if len(counts.Avatars) != avatarCap {
		t.Errorf("Avatars = %d, want the cap of %d", len(counts.Avatars), avatarCap)
	}
}

func testDir(t *testing.T) *zones.Directory {
	t.Helper()
	raw, err := os.ReadFile("testdata/pool.json")
	if err != nil {
		t.Fatalf("read zones fixture: %v", err)
	}
	dir, err := zones.New(raw)
	if err != nil {
		t.Fatalf("load zones fixture: %v", err)
	}
	return dir
}

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	clean := func() {
		for _, q := range []string{
			`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM blocks WHERE blocker_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up test users: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, spec testUser) entity.User {
	t.Helper()

	photoStatus := entity.ModerationPending
	if spec.photoOK {
		photoStatus = entity.ModerationOK
	}
	profile := entity.Profile{
		Age:        25,
		Gender:     spec.gender,
		GenderPref: spec.pref,
		Schedule:   spec.schedule,
		Smoking:    spec.smoking,
		PetsOwn:    entity.PetsOwnNone,
		PetsOK:     entity.PetsOKFine,
		Noise:      entity.NoiseNormal,
		Guests:     entity.GuestsRare,
		Cleaning:   entity.CleaningSchedule,
		Alcohol:    entity.AlcoholNone,
		Expenses:   entity.ExpensesSplitEqually,
	}

	var id uuid.UUID
	err := pool.QueryRow(t.Context(), `
		INSERT INTO users (tg_id, name, status, age, gender, gender_pref,
			schedule, smoking, pets_own, pets_ok, noise, guests, cleaning,
			alcohol, expenses, photo_key, photo_status)
		VALUES ($1, 'test', 'active', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15)
		RETURNING id`,
		testTGBase-spec.n, profile.Age, string(profile.Gender), string(profile.GenderPref),
		nullable(string(profile.Schedule)), nullable(string(profile.Smoking)),
		string(profile.PetsOwn), string(profile.PetsOK), string(profile.Noise),
		string(profile.Guests), string(profile.Cleaning), string(profile.Alcohol),
		string(profile.Expenses), spec.photoKey, string(photoStatus)).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	return entity.User{
		ID:          id,
		Name:        "test",
		Profile:     profile,
		PhotoKey:    spec.photoKey,
		PhotoStatus: photoStatus,
		Status:      entity.StatusActive,
	}
}

func pause(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE searches SET status = 'paused' WHERE id = $1`, id); err != nil {
		t.Fatalf("pause search %s: %v", id, err)
	}
}

func mustCreate(t *testing.T, repo *searchesrepo.Repo, userID uuid.UUID, p entity.SearchParams) entity.Search {
	t.Helper()
	s, err := repo.Create(t.Context(), userID, p)
	if err != nil {
		t.Fatalf("create search in %s: %v", p.ZoneID, err)
	}
	return s
}

func zoneParams(zone string, min, max int, move time.Time) entity.SearchParams {
	return entity.SearchParams{
		ZoneID:   zone,
		PriceMin: pointer.To(min),
		PriceMax: max,
		MoveIn:   &move,
	}
}

func assertSectionHas(t *testing.T, got Pool, want SectionID, userID uuid.UUID) Person {
	t.Helper()
	for _, section := range got.Sections {
		for _, p := range section.People {
			if p.User.ID != userID {
				continue
			}
			if section.ID != want {
				t.Fatalf("user %s sits in section %q, want %q", userID, section.ID, want)
			}
			return p
		}
	}
	t.Fatalf("user %s is not in the pool at all, want section %q", userID, want)
	return Person{}
}

func assertNowhere(t *testing.T, got Pool, why string, userID uuid.UUID) {
	t.Helper()
	lists := [][]Person{got.People}
	for _, section := range got.Sections {
		lists = append(lists, section.People)
	}
	for _, list := range lists {
		for _, p := range list {
			if p.User.ID == userID {
				t.Errorf("user %s must not be in the pool: %s", userID, why)
				return
			}
		}
	}
}

func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
