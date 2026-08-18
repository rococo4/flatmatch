package matches

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
	"flatmatch/api/internal/service/zones"
)

var (
	_ Repository = (*matchesrepo.Repo)(nil)
	_ Searches   = (*searchesrepo.Repo)(nil)
	_ Users      = (*usersrepo.Repo)(nil)
	_ Points     = (*pointsrepo.Repo)(nil)
)

type fakeRepo struct {
	match matchesrepo.MatchWithPeer
	err   error

	listCalls int
}

type fakeSearches map[uuid.UUID][]entity.Search

type fakeUsers map[uuid.UUID]entity.User

type fakePoints map[uuid.UUID]entity.Point

type fixture struct {
	svc      *Service
	repo     *fakeRepo
	searches fakeSearches
	users    fakeUsers
	points   fakePoints
	viewer   entity.User
	peer     entity.User
}

func (f *fakeRepo) ListByUser(context.Context, uuid.UUID) ([]matchesrepo.MatchWithPeer, error) {
	f.listCalls++
	if f.err != nil {
		return nil, f.err
	}
	return []matchesrepo.MatchWithPeer{f.match}, nil
}

func (f *fakeRepo) ByID(context.Context, uuid.UUID, uuid.UUID) (matchesrepo.MatchWithPeer, error) {
	if f.err != nil {
		return matchesrepo.MatchWithPeer{}, f.err
	}
	return f.match, nil
}

func (f fakeSearches) ListByUser(_ context.Context, userID uuid.UUID) ([]entity.Search, error) {
	return f[userID], nil
}

func (f fakeUsers) ByID(_ context.Context, id uuid.UUID) (entity.User, error) {
	u, ok := f[id]
	if !ok {
		return entity.User{}, entity.ErrNotFound
	}
	return u, nil
}

func (f fakePoints) ByID(_ context.Context, id uuid.UUID) (entity.Point, error) {
	p, ok := f[id]
	if !ok {
		return entity.Point{}, entity.ErrNotFound
	}
	return p, nil
}

func testDir(t *testing.T) *zones.Directory {
	t.Helper()
	raw, err := os.ReadFile("../zones/testdata/mini.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := zones.New(raw)
	if err != nil {
		t.Fatalf("zones.New: %v", err)
	}
	return d
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	viewer := entity.User{
		ID:      uuid.New(),
		Name:    "Аня",
		Status:  entity.StatusActive,
		Profile: entity.Profile{Schedule: entity.ScheduleOwl, Smoking: entity.SmokingNo},
	}
	peer := entity.User{
		ID:         uuid.New(),
		Name:       "Катя",
		Username:   "kate",
		TelegramID: 4242,
		Status:     entity.StatusActive,
		Profile:    entity.Profile{Schedule: entity.ScheduleOwl, Smoking: entity.SmokingHome},
	}

	f := &fixture{
		repo: &fakeRepo{match: matchesrepo.MatchWithPeer{
			Match: entity.Match{ID: uuid.New(), AID: viewer.ID, BID: peer.ID, ContextZone: "d-khamovniki"},
			Peer:  peer,
		}},
		searches: fakeSearches{},
		users:    fakeUsers{viewer.ID: viewer, peer.ID: peer},
		points:   fakePoints{},
		viewer:   viewer,
		peer:     peer,
	}
	f.svc = New(f.repo, f.searches, f.users, f.points, testDir(t))
	return f
}

func (f *fixture) atPoint(p entity.Point) uuid.UUID {
	p.ID = uuid.New()
	f.points[p.ID] = p
	f.repo.match.Match.ContextZone = ""
	f.repo.match.Match.ContextPoint = &p.ID
	return p.ID
}

func (f *fixture) detail(t *testing.T) (matchesrepo.MatchWithPeer, Summary) {
	t.Helper()
	match, summary, err := f.svc.Detail(t.Context(), f.viewer.ID, f.repo.match.Match.ID)
	if err != nil {
		t.Fatalf("Detail() failed: %v", err)
	}
	return match, summary
}

func TestDetailHidesContactsOfUnreachablePeer(t *testing.T) {
	cases := map[string]struct {
		status   entity.Status
		blocked  bool
		contacts bool
	}{
		"active":  {entity.StatusActive, false, true},
		"banned":  {entity.StatusBanned, false, false},
		"pending": {entity.StatusPending, false, false},
		"blocked": {entity.StatusActive, true, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.repo.match.Peer.Status = tc.status
			f.repo.match.Blocked = tc.blocked

			got, _ := f.detail(t)

			hasContacts := got.Peer.Username != "" || got.Peer.TelegramID != 0
			if hasContacts != tc.contacts {
				t.Errorf("contacts present = %v (username %q, tg_id %d), want %v",
					hasContacts, got.Peer.Username, got.Peer.TelegramID, tc.contacts)
			}
			if got.Match.ID != f.repo.match.Match.ID || got.Peer.Name != f.peer.Name {
				t.Error("the match itself must survive — only the contacts go")
			}
		})
	}
}

func TestDetailPropagatesRepoError(t *testing.T) {
	f := newFixture(t)
	f.repo.err = entity.ErrNotFound

	if _, _, err := f.svc.Detail(t.Context(), f.viewer.ID, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Detail() = %v, want ErrNotFound", err)
	}
}

func TestListReachesRepo(t *testing.T) {
	f := newFixture(t)

	got, err := f.svc.List(t.Context(), f.viewer.ID)
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if f.repo.listCalls != 1 || len(got) != 1 {
		t.Errorf("listCalls = %d, matches = %d, want 1 and 1", f.repo.listCalls, len(got))
	}
}
