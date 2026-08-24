package deck

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/passesrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

var (
	_ Searches = (*searchesrepo.Repo)(nil)
	_ Points   = (*pointsrepo.Repo)(nil)
	_ Passes   = (*passesrepo.Repo)(nil)
	_ People   = (*usersrepo.Repo)(nil)
	_ Matches  = (*matchesrepo.Repo)(nil)
	_ Likes    = (*likesrepo.Repo)(nil)
)

var (
	sept = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	dec  = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
)

type fakeSearches struct {
	mine       []entity.Search
	candidates []searchesrepo.DeckCandidate
	active     map[uuid.UUID][]entity.Search

	zones  []string
	filter entity.PeopleFilter
	within time.Duration
	calls  int
}

type fakePoints struct {
	rooms  []pointsrepo.FeedPoint
	byID   map[uuid.UUID]entity.Point
	zones  []string
	filter entity.PeopleFilter
	calls  int
}

type fakePasses struct {
	active  []uuid.UUID
	passed  []uuid.UUID
	cleared int
	within  time.Duration
}

type fakePeople map[uuid.UUID]entity.User

type fakeMatches map[uuid.UUID]bool

type fakeLikes map[uuid.UUID]bool

func testDir(t *testing.T) *zones.Directory {
	t.Helper()
	raw, err := os.ReadFile("testdata/deck.json")
	if err != nil {
		t.Fatalf("read zones fixture: %v", err)
	}
	dir, err := zones.New(raw)
	if err != nil {
		t.Fatalf("load zones fixture: %v", err)
	}
	return dir
}

func testSearch(zone string, min, max int, created time.Time, months ...time.Time) entity.Search {
	return entity.Search{
		ID:     uuid.New(),
		Status: entity.SearchActive,
		SearchParams: entity.SearchParams{
			ZoneID:       zone,
			PriceMin:     pointer.To(min),
			PriceMax:     max,
			MoveInMonths: months,
		},
		CreatedAt: created,
	}
}

func person(name string, lastSeen time.Time) entity.User {
	return entity.User{ID: uuid.New(), Name: name, LastSeen: lastSeen, Status: entity.StatusActive}
}

func room(author entity.User, zone string, share int, created time.Time) pointsrepo.FeedPoint {
	return pointsrepo.FeedPoint{
		Point: entity.Point{
			ID:         uuid.New(),
			Type:       entity.PointIntentHome,
			Stage:      pointer.To(entity.StageLive),
			Status:     entity.PointActive,
			CreatedBy:  &author.ID,
			ZoneID:     zone,
			PriceShare: pointer.To(share),
			CreatedAt:  created,
		},
		Author:    &author,
		Responses: 2,
	}
}

func newService(t *testing.T, s *fakeSearches, p *fakePoints, ps *fakePasses,
	people fakePeople, m fakeMatches, l fakeLikes,
) *Service {
	t.Helper()
	return New(s, p, ps, people, m, l, testDir(t))
}

func TestDeckWithoutAnActiveSearch(t *testing.T) {
	searches := &fakeSearches{mine: []entity.Search{closedSearch()}}
	svc := newService(t, searches, &fakePoints{}, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.Deck(t.Context(), uuid.New(), Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if !got.NoSearch || len(got.Cards) != 0 || got.Total != 0 {
		t.Errorf("Deck() = %+v, want an empty deck with no_search", got)
	}
	if searches.calls != 0 || len(got.Cards) != 0 {
		t.Error("without a search the deck must not touch the candidates query")
	}
}

func TestDeckSortsBySectionThenFreshness(t *testing.T) {
	now := time.Now()
	mine := testSearch("zd-district", 35000, 45000, now.Add(-time.Hour), sept)

	stale := person("Полное совпадение, но давно не заходил", now.Add(-72*time.Hour))
	fresh := person("Частичное совпадение, зашёл только что", now)
	nobody := person("Не совпал", now.Add(-time.Minute))
	host := person("Хозяйка комнаты", now.Add(-time.Hour))

	searches := &fakeSearches{
		mine: []entity.Search{mine},
		candidates: []searchesrepo.DeckCandidate{
			{User: stale, Searches: []entity.Search{testSearch("zd-district", 40000, 55000, now, sept)}},
			{User: fresh, Searches: []entity.Search{testSearch("zd-line", 60000, 80000, now, sept)}},
			{User: nobody, Searches: []entity.Search{testSearch("zd-station-east", 90000, 120000, now, dec)}},
		},
	}
	points := &fakePoints{rooms: []pointsrepo.FeedPoint{room(host, "zd-station-east", 42000, now.Add(-30*time.Minute))}}
	svc := newService(t, searches, points, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.Deck(t.Context(), uuid.New(), Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if got.Total != 4 || len(got.Cards) != 4 {
		t.Fatalf("Deck() holds %d cards (total %d), want 4", len(got.Cards), got.Total)
	}

	wantSections := []pools.SectionID{pools.SectionAll, pools.SectionAll, pools.SectionPartial, pools.SectionNone}
	for i, want := range wantSections {
		if got.Cards[i].Pair.Section != want {
			t.Errorf("card %d is in section %q, want %q", i, got.Cards[i].Pair.Section, want)
		}
	}
	if got.Cards[0].Kind != KindRoom {
		t.Errorf("card 0 = %s, want the room — its author is fresher than the stale full match", got.Cards[0].Kind)
	}
	if got.Cards[1].Kind != KindPerson || got.Cards[1].Person.ID != stale.ID {
		t.Errorf("card 1 = %+v, want the full match %s", got.Cards[1], stale.ID)
	}
	if got.Cards[2].Person.ID != fresh.ID || got.Cards[3].Person.ID != nobody.ID {
		t.Errorf("tail = %s, %s; want %s then %s",
			got.Cards[2].Person.ID, got.Cards[3].Person.ID, fresh.ID, nobody.ID)
	}
	if got.Cards[1].Pair.Zone.ID != "zd-district" || !got.Cards[1].Pair.ZoneMatch {
		t.Errorf("pair zone = %+v, want zd-district marked as mine", got.Cards[1].Pair.Zone)
	}
	if got.Cards[1].Pair.Params == nil || got.Cards[1].Pair.Params.PriceMax != 55000 {
		t.Errorf("pair params = %+v, want the params of their best search", got.Cards[1].Pair.Params)
	}
	if len(got.Cards[0].Pair.Chips) != 0 || got.Cards[0].Pair.Params != nil {
		t.Error("a room card carries no chips and no search params")
	}
}

func TestDeckRoomSectionFollowsThePriceShare(t *testing.T) {
	now := time.Now()
	host := person("Хозяйка", now)
	cases := map[string]struct {
		share int
		zone  string
		want  pools.SectionID
		gone  bool
	}{
		"доля в вилке":        {42000, "zd-station-east", pools.SectionAll, false},
		"доля выше вилки":     {90000, "zd-station-east", pools.SectionPartial, false},
		"доля ниже вилки":     {10000, "zd-station-east", pools.SectionPartial, false},
		"зона не пересеклась": {42000, "zd-far", "", true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			searches := &fakeSearches{mine: []entity.Search{testSearch("zd-district", 35000, 45000, now, sept)}}
			points := &fakePoints{rooms: []pointsrepo.FeedPoint{room(host, tc.zone, tc.share, now)}}
			svc := newService(t, searches, points, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

			got, err := svc.Deck(t.Context(), uuid.New(), Query{Rooms: true})
			if err != nil {
				t.Fatalf("Deck() failed: %v", err)
			}
			if tc.gone {
				if len(got.Cards) != 0 {
					t.Fatalf("Deck() = %+v, want no card for a room outside my zones", got.Cards)
				}
				return
			}
			if len(got.Cards) != 1 {
				t.Fatalf("Deck() holds %d cards, want 1", len(got.Cards))
			}
			if got.Cards[0].Pair.Section != tc.want {
				t.Errorf("section = %q, want %q", got.Cards[0].Pair.Section, tc.want)
			}
		})
	}
}

func TestDeckAsksOnlyForWhatWasRequested(t *testing.T) {
	now := time.Now()
	searches := &fakeSearches{mine: []entity.Search{
		testSearch("zd-line", 35000, 45000, now, sept),
		testSearch("zd-station-east", 35000, 45000, now, sept),
	}}
	points := &fakePoints{}
	filter := entity.PeopleFilter{Schedule: []string{"owl"}}
	svc := newService(t, searches, points, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

	if _, err := svc.Deck(t.Context(), uuid.New(), Query{People: true, Filter: filter}); err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if searches.calls != 1 || points.calls != 0 {
		t.Errorf("who=people asked searches %d time(s) and points %d, want 1 and 0",
			searches.calls, points.calls)
	}
	want := map[string]bool{"zd-district": true, "zd-line": true, "zd-station-east": true, "zd-cross": true}
	if len(searches.zones) != len(want) {
		t.Errorf("zones = %v, want the union of intersections %v", searches.zones, want)
	}
	for _, id := range searches.zones {
		if !want[id] {
			t.Errorf("zone %q is not an intersection of my searches", id)
		}
	}
	if len(searches.filter.Schedule) != 1 || searches.filter.Schedule[0] != "owl" {
		t.Errorf("filter %+v did not reach the repo", searches.filter)
	}
	if searches.within != passWindow {
		t.Errorf("pass window = %v, want %v", searches.within, passWindow)
	}

	searches.calls, points.calls = 0, 0
	if _, err := svc.Deck(t.Context(), uuid.New(), Query{Rooms: true}); err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if searches.calls != 0 || points.calls != 1 {
		t.Errorf("who=rooms asked searches %d time(s) and points %d, want 0 and 1",
			searches.calls, points.calls)
	}
}

func TestDeckCapsTheListButCountsEverything(t *testing.T) {
	now := time.Now()
	searches := &fakeSearches{mine: []entity.Search{testSearch("zd-district", 35000, 45000, now, sept)}}
	for i := range cardCap + 5 {
		mate := person("сосед", now.Add(-time.Duration(i)*time.Minute))
		searches.candidates = append(searches.candidates, searchesrepo.DeckCandidate{
			User:     mate,
			Searches: []entity.Search{testSearch("zd-district", 40000, 55000, now, sept)},
		})
	}
	svc := newService(t, searches, &fakePoints{}, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.Deck(t.Context(), uuid.New(), Query{People: true, Rooms: true})
	if err != nil {
		t.Fatalf("Deck() failed: %v", err)
	}
	if len(got.Cards) != cardCap {
		t.Errorf("Deck() holds %d cards, want the cap of %d", len(got.Cards), cardCap)
	}
	if got.Total != cardCap+5 {
		t.Errorf("Total = %d, want the full %d", got.Total, cardCap+5)
	}
}

func TestPassPersonNeedsAReachableTarget(t *testing.T) {
	viewer, visible := uuid.New(), person("видимая", time.Now())
	matched, liker, stranger := uuid.New(), uuid.New(), uuid.New()

	passes := &fakePasses{}
	svc := newService(t, &fakeSearches{}, &fakePoints{}, passes,
		fakePeople{visible.ID: visible}, fakeMatches{matched: true}, fakeLikes{liker: true})

	for _, id := range []uuid.UUID{visible.ID, matched, liker} {
		if err := svc.PassPerson(t.Context(), viewer, id); err != nil {
			t.Errorf("PassPerson(%s) = %v, want the pass through", id, err)
		}
	}
	if len(passes.passed) != 3 {
		t.Errorf("passes = %v, want three rows", passes.passed)
	}

	if err := svc.PassPerson(t.Context(), viewer, stranger); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("PassPerson(stranger) = %v, want ErrNotFound", err)
	}
	if err := svc.PassPerson(t.Context(), viewer, viewer); !errors.Is(err, entity.ErrSelfTarget) {
		t.Errorf("PassPerson(self) = %v, want ErrSelfTarget", err)
	}
	if len(passes.passed) != 3 {
		t.Errorf("an unreachable target still landed in passes: %v", passes.passed)
	}
}

func TestPassPointTakesLiveRoomsOnly(t *testing.T) {
	live := entity.Point{ID: uuid.New(), Type: entity.PointIntentHome, Status: entity.PointActive}
	closed := entity.Point{ID: uuid.New(), Type: entity.PointIntentHome, Status: entity.PointClosed}
	flat := entity.Point{ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive}

	passes := &fakePasses{}
	points := &fakePoints{byID: map[uuid.UUID]entity.Point{
		live.ID: live, closed.ID: closed, flat.ID: flat,
	}}
	svc := newService(t, &fakeSearches{}, points, passes, fakePeople{}, fakeMatches{}, fakeLikes{})

	if err := svc.PassPoint(t.Context(), uuid.New(), live.ID); err != nil {
		t.Errorf("PassPoint(live room) = %v, want the pass through", err)
	}
	for _, id := range []uuid.UUID{closed.ID, flat.ID, uuid.New()} {
		if err := svc.PassPoint(t.Context(), uuid.New(), id); !errors.Is(err, entity.ErrNotFound) {
			t.Errorf("PassPoint(%s) = %v, want ErrNotFound", id, err)
		}
	}
	if len(passes.passed) != 1 || passes.passed[0] != live.ID {
		t.Errorf("passes = %v, want only the live room", passes.passed)
	}
}

func TestClearPassesCountsWhatCameBack(t *testing.T) {
	passes := &fakePasses{active: []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}}
	svc := newService(t, &fakeSearches{}, &fakePoints{}, passes, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.ClearPasses(t.Context(), uuid.New())
	if err != nil {
		t.Fatalf("ClearPasses() failed: %v", err)
	}
	if got != 3 || passes.cleared != 1 {
		t.Errorf("ClearPasses() = %d after %d call(s), want 3 after one", got, passes.cleared)
	}
}

func TestRankIncomingSortsAndDropsThePassed(t *testing.T) {
	now := time.Now()
	viewer := uuid.New()
	weak := person("слабое совпадение", now)
	strong := person("сильное совпадение", now)
	passed := person("отклонённый", now)

	searches := &fakeSearches{
		mine: []entity.Search{testSearch("zd-district", 35000, 45000, now, sept)},
		active: map[uuid.UUID][]entity.Search{
			weak.ID:   {testSearch("zd-line", 60000, 80000, now, sept)},
			strong.ID: {testSearch("zd-district", 40000, 55000, now, sept)},
			passed.ID: {testSearch("zd-district", 40000, 55000, now, sept)},
		},
	}
	passes := &fakePasses{active: []uuid.UUID{passed.ID}}
	svc := newService(t, searches, &fakePoints{}, passes, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.RankIncoming(t.Context(), viewer, []likesrepo.IncomingLike{
		{From: weak, CreatedAt: now},
		{From: passed, CreatedAt: now.Add(-time.Minute)},
		{From: strong, CreatedAt: now.Add(-2 * time.Minute)},
	})
	if err != nil {
		t.Fatalf("RankIncoming() failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("RankIncoming() = %d likes, want the two unpassed ones", len(got))
	}
	if got[0].Like.From.ID != strong.ID || got[1].Like.From.ID != weak.ID {
		t.Errorf("order = %s, %s; want the better pair first",
			got[0].Like.From.Name, got[1].Like.From.Name)
	}
	if got[0].Pair == nil || got[0].Pair.Section != pools.SectionAll {
		t.Errorf("pair of the top like = %+v, want the full match", got[0].Pair)
	}
	if passes.within != passWindow {
		t.Errorf("pass window = %v, want %v", passes.within, passWindow)
	}
}

func TestRankIncomingKeepsLikesWithoutAPair(t *testing.T) {
	now := time.Now()
	quiet := person("без поиска", now)
	searches := &fakeSearches{mine: []entity.Search{testSearch("zd-district", 35000, 45000, now, sept)}}
	svc := newService(t, searches, &fakePoints{}, &fakePasses{}, fakePeople{}, fakeMatches{}, fakeLikes{})

	got, err := svc.RankIncoming(t.Context(), uuid.New(), []likesrepo.IncomingLike{{From: quiet, CreatedAt: now}})
	if err != nil {
		t.Fatalf("RankIncoming() failed: %v", err)
	}
	if len(got) != 1 || got[0].Pair != nil {
		t.Errorf("RankIncoming() = %+v, want the like through without a pair", got)
	}
}

func closedSearch() entity.Search {
	s := testSearch("zd-district", 35000, 45000, time.Now(), sept)
	s.Status = entity.SearchClosed
	return s
}

func (f *fakeSearches) ListByUser(context.Context, uuid.UUID) ([]entity.Search, error) {
	return f.mine, nil
}

func (f *fakeSearches) DeckCandidates(_ context.Context, _ uuid.UUID, zoneIDs []string,
	filter entity.PeopleFilter, within time.Duration,
) ([]searchesrepo.DeckCandidate, error) {
	f.calls++
	f.zones, f.filter, f.within = zoneIDs, filter, within
	return f.candidates, nil
}

func (f *fakeSearches) ActiveByUsers(_ context.Context, userIDs []uuid.UUID) ([]entity.Search, error) {
	var out []entity.Search
	for _, id := range userIDs {
		for _, s := range f.active[id] {
			s.UserID = id
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakePoints) DeckRooms(_ context.Context, _ uuid.UUID, zoneIDs []string,
	filter entity.PeopleFilter, _ time.Duration,
) ([]pointsrepo.FeedPoint, error) {
	f.calls++
	f.zones, f.filter = zoneIDs, filter
	return f.rooms, nil
}

func (f *fakePoints) ByID(_ context.Context, id uuid.UUID) (entity.Point, error) {
	p, ok := f.byID[id]
	if !ok {
		return entity.Point{}, entity.ErrNotFound
	}
	return p, nil
}

func (f *fakePasses) Upsert(_ context.Context, _, targetID uuid.UUID) error {
	f.passed = append(f.passed, targetID)
	return nil
}

func (f *fakePasses) ActiveTargets(_ context.Context, _ uuid.UUID, within time.Duration) ([]uuid.UUID, error) {
	f.within = within
	return f.active, nil
}

func (f *fakePasses) Clear(context.Context, uuid.UUID) (int, error) {
	f.cleared++
	return len(f.active), nil
}

func (f fakePeople) VisibleByID(_ context.Context, _, id uuid.UUID) (entity.User, error) {
	u, ok := f[id]
	if !ok {
		return entity.User{}, entity.ErrNotFound
	}
	return u, nil
}

func (f fakeMatches) IsMatched(_ context.Context, _, b uuid.UUID) (bool, error) {
	return f[b], nil
}

func (f fakeLikes) Between(_ context.Context, _, b uuid.UUID) (bool, bool, error) {
	return false, f[b], nil
}
