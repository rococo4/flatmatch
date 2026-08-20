package peopleapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const station = "st-sokol"

type fakePeople struct {
	user entity.User
	err  error
}

type fakeLikes struct {
	betweenCalls int
}

type fakePhotos struct{}

type fakeDir struct{}

type fakeMatches struct{}

func (fakeMatches) MatchIDBetween(context.Context, uuid.UUID, uuid.UUID) (*uuid.UUID, error) {
	return nil, nil
}

type fakePoints struct {
	point entity.Point
	pool  map[uuid.UUID]bool
	calls *int
}

type fakeSearches struct{ met bool }

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestProfileChecksVisibilityBeforeLikes(t *testing.T) {
	likes := &fakeLikes{}
	pointCalls := 0
	router := newRouter(New(&fakeService{}, &fakePeople{err: entity.ErrNotFound}, likes, fakePhotos{},
		fakeSearches{}, fakePoints{calls: &pointCalls}, fakeMatches{}, fakeDir{}), uuid.New())

	rec := get(router, "/people/"+uuid.NewString()+"?point="+uuid.NewString())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if likes.betweenCalls != 0 {
		t.Errorf("Between was called %d time(s) for an invisible person — like existence leaks",
			likes.betweenCalls)
	}

	if pointCalls != 0 {
		t.Errorf("Points was asked %d time(s) about an invisible person", pointCalls)
	}
}

func TestProfileReadsLikesOfVisiblePerson(t *testing.T) {
	likes := &fakeLikes{}
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakeService{}, &fakePeople{user: target}, likes, fakePhotos{},
		fakeSearches{met: true}, fakePoints{}, fakeMatches{}, fakeDir{}), uuid.New())

	rec := get(router, "/people/"+target.ID.String())

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if likes.betweenCalls != 1 {
		t.Errorf("Between calls = %d, want 1", likes.betweenCalls)
	}
}

func TestProfileHiddenWithoutMeeting(t *testing.T) {
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakeService{}, &fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
		fakeSearches{}, fakePoints{}, fakeMatches{}, fakeDir{}), uuid.New())

	rec := get(router, "/people/"+target.ID.String())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d — стороннему человеку профиль не показываем", rec.Code, http.StatusNotFound)
	}
}

func TestProfileOpensToThePointRelation(t *testing.T) {
	viewer := uuid.New()
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	flat := entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station,
	}
	room := entity.Point{
		ID: uuid.New(), Type: entity.PointIntentHome, Status: entity.PointActive,
		ZoneID: station, CreatedBy: &target.ID,
	}
	pending := flat
	pending.Status = entity.PointPending
	stranger := uuid.New()
	strangersRoom := room
	strangersRoom.CreatedBy = &stranger

	cases := map[string]struct {
		points fakePoints
		want   int
		label  string
	}{
		"со-пульник квартиры": {
			points: fakePoints{point: flat, pool: map[uuid.UUID]bool{viewer: true, target.ID: true}},
			want:   http.StatusOK,
			label:  "квартира у м. Сокол",
		},
		"автор «к себе» открыт откликнувшемуся": {
			points: fakePoints{point: room},
			want:   http.StatusOK,
			label:  "на комнату Кати · м. Сокол",
		},

		"«к себе» чужого автора": {
			points: fakePoints{point: strangersRoom},
			want:   http.StatusNotFound,
		},

		"не вошедший в пул": {
			points: fakePoints{point: flat, pool: map[uuid.UUID]bool{target.ID: true}},
			want:   http.StatusNotFound,
		},

		"вышедшая из пула цель": {
			points: fakePoints{point: flat, pool: map[uuid.UUID]bool{viewer: true}},
			want:   http.StatusNotFound,
		},
		"точка на проверке": {
			points: fakePoints{point: pending, pool: map[uuid.UUID]bool{viewer: true, target.ID: true}},
			want:   http.StatusNotFound,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := newRouter(New(&fakeService{}, &fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
				fakeSearches{}, tc.points, fakeMatches{}, fakeDir{}), viewer)

			rec := get(router, "/people/"+target.ID.String()+"?point="+tc.points.point.ID.String())

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
			if tc.label == "" {
				return
			}
			for _, want := range []string{`"context_point"`, tc.points.point.ID.String(), tc.label} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("profile %s does not carry %s", rec.Body, want)
				}
			}
		})
	}
}

func TestProfileIgnoresAnUnrelatedPointParam(t *testing.T) {
	viewer := uuid.New()
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakeService{}, &fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
		fakeSearches{}, fakePoints{}, fakeMatches{}, fakeDir{}), viewer)

	for _, param := range []string{uuid.NewString(), "not-a-uuid"} {
		rec := get(router, "/people/"+target.ID.String()+"?point="+param)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status for point=%s = %d, want %d", param, rec.Code, http.StatusNotFound)
		}
	}
}

func newRouter(h *Handler, viewerID uuid.UUID) chi.Router {
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: viewerID, Status: entity.StatusActive},
	}, nil))
	r.Mount("/people", h.Routes())
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

func (f fakePoints) ByID(_ context.Context, id uuid.UUID) (entity.Point, error) {
	if f.calls != nil {
		*f.calls++
	}
	if f.point.ID != id {
		return entity.Point{}, entity.ErrNotFound
	}
	return f.point, nil
}

func (f fakePoints) InPool(_ context.Context, userID, _ uuid.UUID) (bool, error) {
	return f.pool[userID], nil
}

func (fakeDir) Get(id string) (zones.Zone, bool) {
	if id != station {
		return zones.Zone{}, false
	}
	return zones.Zone{ID: station, Kind: zones.KindStation, Name: "Сокол"}, true
}

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
