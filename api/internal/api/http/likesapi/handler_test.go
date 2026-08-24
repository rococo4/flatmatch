package likesapi

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
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/likes"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const station = "st-sokol"

type fakeLikes struct {
	toID     uuid.UUID
	zoneID   string
	pointID  *uuid.UUID
	calls    int
	incoming []likesrepo.IncomingLike
}

type fakePhotos struct{}

type fakePoints struct{ point entity.Point }

type fakePairs struct {
	got   []likesrepo.IncomingLike
	calls int
}

type fakeDir struct{}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestLikeCarriesItsContextToTheService(t *testing.T) {
	point := uuid.New()
	to := uuid.New()

	cases := map[string]struct {
		body      string
		wantZone  string
		wantPoint *uuid.UUID
	}{
		"пул зоны":  {`{"to_id":"` + to.String() + `","zone_id":"` + station + `"}`, station, nil},
		"точка":     {`{"to_id":"` + to.String() + `","point_id":"` + point.String() + `"}`, "", &point},
		"без места": {`{"to_id":"` + to.String() + `"}`, "", nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			likes := &fakeLikes{}

			rec := post(t, likes, fakePoints{}, uuid.New(), tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
			}
			if likes.calls != 1 || likes.toID != to {
				t.Fatalf("service saw %d call(s) to %s, want one to %s", likes.calls, likes.toID, to)
			}
			if likes.zoneID != tc.wantZone {
				t.Errorf("zone = %q, want %q", likes.zoneID, tc.wantZone)
			}
			switch {
			case tc.wantPoint == nil && likes.pointID != nil:
				t.Errorf("point = %s, want none", likes.pointID)
			case tc.wantPoint != nil && (likes.pointID == nil || *likes.pointID != *tc.wantPoint):
				t.Errorf("point = %v, want %s", likes.pointID, tc.wantPoint)
			}
		})
	}
}

func TestLikeRefusesTwoContexts(t *testing.T) {
	likes := &fakeLikes{}
	body := `{"to_id":"` + uuid.NewString() + `","zone_id":"` + station + `","point_id":"` + uuid.NewString() + `"}`

	rec := post(t, likes, fakePoints{}, uuid.New(), body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	for _, want := range []string{`"invalid_field"`, `"zone_id"`, `"point_id"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body %s does not carry %s", rec.Body, want)
		}
	}
}

func TestLikeRefusesABrokenPointID(t *testing.T) {
	likes := &fakeLikes{}

	rec := post(t, likes, fakePoints{}, uuid.New(), `{"to_id":"`+uuid.NewString()+`","point_id":"комната"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"point_id"`) {
		t.Errorf("body = %s, want invalid_field on point_id", rec.Body)
	}
	if likes.calls != 0 {
		t.Errorf("a broken point id reached the service %d time(s)", likes.calls)
	}
}

func TestIncomingCarriesThePointCard(t *testing.T) {
	room := entity.Point{
		ID: uuid.New(), Type: entity.PointIntentHome, Status: entity.PointActive, ZoneID: station,
	}
	likes := &fakeLikes{incoming: []likesrepo.IncomingLike{{
		From:    entity.User{ID: uuid.New(), Name: "Олег"},
		PointID: &room.ID,
	}}}

	router := newRouter(likes, fakePoints{point: room}, uuid.New())
	req := httptest.NewRequest(http.MethodGet, "/likes/incoming", nil)
	req.Header.Set("Authorization", "tma stub")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	for _, want := range []string{`"point"`, room.ID.String(), `"intent_home"`, "на твою комнату · м. Сокол"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("incoming %s does not carry %s", rec.Body, want)
		}
	}
}

func post(t *testing.T, likes *fakeLikes, points fakePoints, viewer uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/likes", strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	newRouter(likes, points, viewer).ServeHTTP(rec, req)
	return rec
}

func newRouter(likes *fakeLikes, points fakePoints, viewer uuid.UUID) chi.Router {
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: viewer, Status: entity.StatusActive},
	}, nil))
	r.Mount("/likes", New(likes, fakePhotos{}, points, &fakePairs{}, fakeDir{}).Routes())
	return r
}

func (f *fakeLikes) Like(_ context.Context, _ entity.User, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error) {
	f.calls++
	f.toID, f.zoneID, f.pointID = toID, zoneID, pointID

	if zoneID != "" && pointID != nil {
		return entity.LikeResult{}, &entity.FieldsError{
			Err: likes.ErrInvalidParams, Fields: []string{"zone_id", "point_id"}}
	}
	return entity.LikeResult{Budget: entity.LikeBudget{Remaining: entity.MaxDailyLikes - 1}}, nil
}

func (f *fakeLikes) Incoming(context.Context, uuid.UUID) ([]likesrepo.IncomingLike, error) {
	return f.incoming, nil
}

func (f *fakeLikes) Budget(context.Context, uuid.UUID) (entity.LikeBudget, error) {
	return entity.LikeBudget{Remaining: entity.MaxDailyLikes}, nil
}

func (fakePhotos) PhotoURLs(entity.User) []string { return nil }

func (f fakePoints) ByID(_ context.Context, id uuid.UUID) (entity.Point, error) {
	if f.point.ID != id {
		return entity.Point{}, entity.ErrNotFound
	}
	return f.point, nil
}

func (fakeDir) Get(id string) (zones.Zone, bool) {
	if id != station {
		return zones.Zone{}, false
	}
	return zones.Zone{ID: station, Kind: zones.KindStation, Name: "Сокол"}, true
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}

func (f *fakePairs) RankIncoming(_ context.Context, _ uuid.UUID,
	list []likesrepo.IncomingLike,
) ([]deck.RankedLike, error) {
	f.calls++
	f.got = list

	out := make([]deck.RankedLike, 0, len(list))
	for _, like := range list {
		out = append(out, deck.RankedLike{Like: like})
	}
	return out, nil
}
