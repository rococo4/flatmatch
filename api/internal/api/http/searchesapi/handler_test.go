package searchesapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/searches"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const zoneID = "d-khamovniki"

type fakeSearches struct {
	list    []entity.Search
	created entity.SearchParams
}

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

func TestCreateKeepsTheParamSets(t *testing.T) {
	searches := &fakeSearches{}
	body := `{"zone_id":"d-khamovniki","price_max":45000,` +
		`"rooms":[3,2],"terms":["year"],"move_in":["2026-09","2026-10"]}`

	rec := post(t, searches, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	if !slices.Equal(searches.created.Rooms, []int{2, 3}) {
		t.Errorf("service got rooms %v, want sorted [2 3]", searches.created.Rooms)
	}
	got := rec.Body.String()
	for _, want := range []string{`"rooms":[2,3]`, `"terms":["year"]`, `"move_in":["2026-09","2026-10"]`} {
		if !strings.Contains(got, want) {
			t.Errorf("response %s carries no %s", got, want)
		}
	}
}

func TestCreateEmptySetsAreArraysNotNull(t *testing.T) {
	rec := post(t, &fakeSearches{}, `{"zone_id":"d-khamovniki","price_max":45000}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	got := rec.Body.String()
	for _, want := range []string{`"rooms":[]`, `"terms":[]`, `"move_in":[]`} {
		if !strings.Contains(got, want) {
			t.Errorf("response %s carries no %s", got, want)
		}
	}
}

func TestCreateRejectsRoomsOutsideTheSet(t *testing.T) {
	rec := post(t, &fakeSearches{}, `{"zone_id":"d-khamovniki","price_max":45000,"rooms":[7]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	got := rec.Body.String()
	if !strings.Contains(got, `"invalid_field"`) || !strings.Contains(got, `"fields":["rooms"]`) {
		t.Errorf("response = %s, want invalid_field on rooms", got)
	}
}

func TestCreateRejectsMalformedMonth(t *testing.T) {
	rec := post(t, &fakeSearches{}, `{"zone_id":"d-khamovniki","price_max":45000,"move_in":["сентябрь"]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"fields":["move_in"]`) {
		t.Errorf("response = %s, want invalid_field on move_in", got)
	}
}

func post(t *testing.T, searches *fakeSearches, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	r.Mount("/searches", New(searches, fakePools{}, &fakePoints{}, fakeDir{}).Routes())

	req := httptest.NewRequest(http.MethodPost, "/searches", strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
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

func (f *fakeSearches) Create(_ context.Context, _ uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	if bad := p.InvalidFields(); len(bad) > 0 {
		return entity.Search{}, &entity.FieldsError{Err: searches.ErrInvalidParams, Fields: bad}
	}
	f.created = p
	return entity.Search{ID: uuid.New(), SearchParams: p, Status: entity.SearchActive}, nil
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
