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

// station is the zone of every test point: fakeDir knows this one id and no
// other, so a label with «м. Сокол» proves the directory was asked.
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

// fakePoints holds one point and the membership of its pool: the two facts the
// point half of «мы увиделись» is made of. Calls are counted for the same
// reason Between's are — про невидимого человека нельзя спрашивать ничего.
type fakePoints struct {
	point entity.Point
	pool  map[uuid.UUID]bool
	calls *int
}

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
	pointCalls := 0
	router := newRouter(New(&fakePeople{err: entity.ErrNotFound}, likes, fakePhotos{},
		fakeSearches{}, fakePoints{calls: &pointCalls}, fakeDir{}), uuid.New())

	rec := get(router, "/people/"+uuid.NewString()+"?point="+uuid.NewString())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if likes.betweenCalls != 0 {
		t.Errorf("Between was called %d time(s) for an invisible person — like existence leaks",
			likes.betweenCalls)
	}
	// Точку в query спрашивать тоже не о чем: невидимый человек не существует
	// нигде (§4), и контекст его профиля — тем более.
	if pointCalls != 0 {
		t.Errorf("Points was asked %d time(s) about an invisible person", pointCalls)
	}
}

// TestProfileReadsLikesOfVisiblePerson is the other half of the pin: once the
// gate lets the viewer through, the like state IS read — otherwise the test
// above would pass on a handler that never asks at all.
func TestProfileReadsLikesOfVisiblePerson(t *testing.T) {
	likes := &fakeLikes{}
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakePeople{user: target}, likes, fakePhotos{},
		fakeSearches{met: true}, fakePoints{}, fakeDir{}), uuid.New())

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
	router := newRouter(New(&fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
		fakeSearches{}, fakePoints{}, fakeDir{}), uuid.New())

	rec := get(router, "/people/"+target.ID.String())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d — стороннему человеку профиль не показываем", rec.Code, http.StatusNotFound)
	}
}

// Точка — вторая половина «мы увиделись» (§2.3): пул квартиры сводит двоих
// членством, карточка «к себе» — тем, что цель и есть её автор. Поисков ни у
// кого нет: точечное отношение их не требует.
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
		// Комната открывает профиль СВОЕГО автора: соседа по отклику она не
		// сводит ни с кем — пула у «к себе» нет (§2.2).
		"«к себе» чужого автора": {
			points: fakePoints{point: strangersRoom},
			want:   http.StatusNotFound,
		},
		// Посторонний называет ту же точку — но в её пуле его нет.
		"не вошедший в пул": {
			points: fakePoints{point: flat, pool: map[uuid.UUID]bool{target.ID: true}},
			want:   http.StatusNotFound,
		},
		// Цель вышла из пула: смотреть больше не на что общее.
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
			router := newRouter(New(&fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
				fakeSearches{}, tc.points, fakeDir{}), viewer)

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

// Чужая точка в query — не пропуск: отношение её не подтверждает, и профиль
// отвечает тем же 404, что и невидимому.
func TestProfileIgnoresAnUnrelatedPointParam(t *testing.T) {
	viewer := uuid.New()
	target := entity.User{ID: uuid.New(), Name: "Катя", Status: entity.StatusActive}
	router := newRouter(New(&fakePeople{user: target}, &fakeLikes{}, fakePhotos{},
		fakeSearches{}, fakePoints{}, fakeDir{}), viewer)

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
