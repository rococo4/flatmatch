package peopleapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

type fakePeople struct {
	user entity.User
	err  error
}

type fakeLikes struct {
	betweenCalls int
}

type fakePhotos struct{}

type fakeDir struct{}

// fakeSearches answers the pool half of «мы увиделись»: met=true means the
// pair's active searches intersect.
type fakeSearches struct{ met bool }

// The two halves of authmw, so the handler runs behind the real middleware and
// finds a real viewer in the context.
type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

// TestProfileChecksVisibilityBeforeLikes pins the order of §4 against §2.3:
// Between has no visibility gate of its own, so it must not be asked anything
// about a person the viewer cannot see — the mere fact that a like exists is
// something only the addressee may learn.
func TestProfileChecksVisibilityBeforeLikes(t *testing.T) {
	likes := &fakeLikes{}
	router := newRouter(New(&fakePeople{err: entity.ErrNotFound}, likes, fakePhotos{}, fakeSearches{}, fakeDir{}))

	rec := get(router, "/people/"+uuid.NewString())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if likes.betweenCalls != 0 {
		t.Errorf("Between was called %d time(s) for an invisible person — like existence leaks",
			likes.betweenCalls)
	}
}

// TestProfileReadsLikesOfVisiblePerson is the other half of the pin: once the
// gate lets the viewer through, the like state IS read — otherwise the test
// above would pass on a handler that never asks at all.
func TestProfileReadsLikesOfVisiblePerson(t *testing.T) {
	likes := &fakeLikes{}
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakePeople{user: target}, likes, fakePhotos{}, fakeSearches{met: true}, fakeDir{}))

	rec := get(router, "/people/"+target.ID.String())

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if likes.betweenCalls != 1 {
		t.Errorf("Between calls = %d, want 1", likes.betweenCalls)
	}
}

// TestProfileHiddenWithoutMeeting pins the «мы увиделись» rule (§2.3): a
// visible person whose searches share no pool with the viewer — and with no
// likes between the two — answers the same 404 as one who does not exist.
func TestProfileHiddenWithoutMeeting(t *testing.T) {
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakePeople{user: target}, &fakeLikes{}, fakePhotos{}, fakeSearches{}, fakeDir{}))

	rec := get(router, "/people/"+target.ID.String())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d — стороннему человеку профиль не показываем", rec.Code, http.StatusNotFound)
	}
}

func newRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}))
	r.Get("/people/{id}", h.Profile)
	return r
}

func get(router chi.Router, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func (f *fakePeople) VisibleByID(context.Context, uuid.UUID, uuid.UUID) (entity.User, error) {
	return f.user, f.err
}

func (f *fakeLikes) Between(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
	f.betweenCalls++
	return false, false, nil
}

func (f *fakeLikes) Budget(context.Context, uuid.UUID) (entity.LikeBudget, error) {
	return entity.LikeBudget{Remaining: entity.MaxDailyLikes}, nil
}

func (fakePhotos) PhotoURL(entity.User) string { return "" }

func (fakeDir) Get(string) (zones.Zone, bool) { return zones.Zone{}, false }

func (fakeDir) Intersecting(id string) []string { return []string{id} }

func (f fakeSearches) ListByUser(context.Context, uuid.UUID) ([]entity.Search, error) {
	if !f.met {
		return nil, nil
	}
	return []entity.Search{{
		SearchParams: entity.SearchParams{ZoneID: "z"},
		Status:       entity.SearchActive,
	}}, nil
}

func (f fakeSearches) HasActiveIn(context.Context, uuid.UUID, []string) (bool, error) {
	return f.met, nil
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
