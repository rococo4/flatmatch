package searchesapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const zoneID = "d-khamovniki"

type fakeSearches struct{ list []entity.Search }

type fakePools struct{}

type fakePoints struct {
	count int
	zones []string
}

type fakeDir struct{}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestListCarriesThePointsTeaser(t *testing.T) {
	points := &fakePoints{count: 7}
	searches := &fakeSearches{list: []entity.Search{
		{
			ID:           uuid.New(),
			SearchParams: entity.SearchParams{ZoneID: zoneID, PriceMax: 45000},
			Status:       entity.SearchActive,
		},

		{
			ID:           uuid.New(),
			SearchParams: entity.SearchParams{ZoneID: zoneID, PriceMax: 45000},
			Status:       entity.SearchClosed,
		},
	}}

	rec := get(t, searches, points)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"points_count":7`) {
		t.Errorf("active search %s carries no points teaser", body)
	}
	if strings.Count(body, `"points_count":0`) != 1 {
		t.Errorf("closed search should carry an empty teaser: %s", body)
	}
	if len(points.zones) != 1 || points.zones[0] != zoneID {
		t.Errorf("points were counted in %v, want only %s", points.zones, zoneID)
	}
}

func get(t *testing.T, searches *fakeSearches, points *fakePoints) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	r.Mount("/searches", New(searches, fakePools{}, points, fakeDir{}).Routes())

	req := httptest.NewRequest(http.MethodGet, "/searches", nil)
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (f *fakeSearches) List(context.Context, uuid.UUID) ([]entity.Search, error) {
	return f.list, nil
}

func (f *fakeSearches) Create(context.Context, uuid.UUID, entity.SearchParams) (entity.Search, error) {
	return entity.Search{}, nil
}

func (f *fakeSearches) Update(context.Context, uuid.UUID, uuid.UUID, entity.SearchParams) (entity.Search, error) {
	return entity.Search{}, nil
}

func (f *fakeSearches) Close(context.Context, uuid.UUID, uuid.UUID, entity.CloseReason) (entity.Search, error) {
	return entity.Search{}, nil
}

func (f *fakeSearches) Reactivate(context.Context, uuid.UUID, uuid.UUID) (entity.Search, error) {
	return entity.Search{}, nil
}

func (fakePools) Counts(context.Context, uuid.UUID, entity.SearchParams, *time.Time) (pools.Counts, error) {
	return pools.Counts{People: 3}, nil
}

func (f *fakePoints) CountIn(_ context.Context, zone string) (int, error) {
	f.zones = append(f.zones, zone)
	return f.count, nil
}

func (fakeDir) Get(id string) (zones.Zone, bool) {
	if id != zoneID {
		return zones.Zone{}, false
	}
	return zones.Zone{ID: zoneID, Kind: zones.KindDistrict, Name: "Хамовники",
		NameIn: "в Хамовниках", NameGen: "Хамовников"}, true
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
