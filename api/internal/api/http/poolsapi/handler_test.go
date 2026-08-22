package poolsapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const zoneID = "d-khamovniki"

const station = "st-sportivnaya"

type fakePools struct{}

type fakeSearches struct{}

type fakePoints struct {
	list  []pointsrepo.FeedPoint
	zones []string
}

type fakePhotos struct{}

type fakeDir struct{}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestFeedCarriesThePointsOfTheZone(t *testing.T) {
	flat := pointsrepo.FeedPoint{
		Point: entity.Point{
			ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive,
			ZoneID: station, CreatedAt: time.Now(),
		},
		PoolCount: 4,
	}
	points := &fakePoints{list: []pointsrepo.FeedPoint{flat}}

	rec := feed(t, points, "/feed?zone_id="+zoneID+"&price_max=45000")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{`"promo"`, flat.Point.ID.String(), `"pool_count":4`, `"Спортивная"`} {
		if !strings.Contains(body, want) {
			t.Errorf("feed %s does not carry %s", body, want)
		}
	}
	if len(points.zones) != 1 || points.zones[0] != zoneID {
		t.Errorf("points were asked about %v, want only %s", points.zones, zoneID)
	}
}

func TestFeedRefusesBrokenFiltersBeforeCounting(t *testing.T) {
	points := &fakePoints{}

	rec := feed(t, points, "/feed?zone_id=d-nowhere&price_max=45000")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if len(points.zones) != 0 {
		t.Errorf("points were asked about %v for a zone that does not exist", points.zones)
	}
}

func TestParseParamsReadsSets(t *testing.T) {
	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := map[string]string{
		"repeated keys": "zone_id=" + zoneID + "&price_max=45000" +
			"&rooms=3&rooms=2&terms=year&move_in=2026-10&move_in=2026-09",
		"comma separated": "zone_id=" + zoneID + "&price_max=45000" +
			"&rooms=3,2&terms=year&move_in=2026-10,2026-09",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			q, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}

			p, bad := parseParams(q)

			if len(bad) != 0 {
				t.Fatalf("bad fields = %v, want none", bad)
			}
			if !slices.Equal(p.Rooms, []int{2, 3}) {
				t.Errorf("Rooms = %v, want [2 3]", p.Rooms)
			}
			if !slices.Equal(p.Terms, []entity.Term{entity.TermYear}) {
				t.Errorf("Terms = %v, want [year]", p.Terms)
			}
			if len(p.MoveInMonths) != 2 ||
				!p.MoveInMonths[0].Equal(sept) || !p.MoveInMonths[1].Equal(oct) {
				t.Errorf("MoveInMonths = %v, want [sept oct]", p.MoveInMonths)
			}
		})
	}
}

func TestParseParamsReportsBrokenSets(t *testing.T) {
	tests := map[string]string{
		"rooms":   "zone_id=" + zoneID + "&price_max=45000&rooms=две",
		"move_in": "zone_id=" + zoneID + "&price_max=45000&move_in=сентябрь",
	}
	for field, raw := range tests {
		t.Run(field, func(t *testing.T) {
			q, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			if _, bad := parseParams(q); !slices.Contains(bad, field) {
				t.Errorf("bad fields = %v, want to contain %q", bad, field)
			}
		})
	}
}

func TestParseParamsMissingSetsAreEmpty(t *testing.T) {
	q, err := url.ParseQuery("zone_id=" + zoneID + "&price_max=45000")
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}

	p, bad := parseParams(q)

	if len(bad) != 0 {
		t.Fatalf("bad fields = %v, want none", bad)
	}
	if len(p.Rooms) != 0 || len(p.Terms) != 0 || len(p.MoveInMonths) != 0 {
		t.Errorf("sets = %+v, want all empty", p)
	}
}

func feed(t *testing.T, points *fakePoints, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	r.Get("/feed", New(fakePools{}, fakeSearches{}, points, fakePhotos{}, fakeDir{}).Feed)

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (fakePools) Counts(context.Context, uuid.UUID, entity.SearchParams, *time.Time) (pools.Counts, error) {
	return pools.Counts{People: 17, FullMatch: 15}, nil
}

func (fakePools) ZonePool(context.Context, entity.User, string) (pools.Pool, error) {
	return pools.Pool{}, nil
}

func (fakeSearches) List(context.Context, uuid.UUID) ([]entity.Search, error) { return nil, nil }

func (f *fakePoints) Feed(_ context.Context, _ uuid.UUID, zone string) ([]pointsrepo.FeedPoint, error) {
	f.zones = append(f.zones, zone)
	return f.list, nil
}

func (fakePhotos) URL(key string) string { return "/media/" + key }

func (fakeDir) Get(id string) (zones.Zone, bool) {
	switch id {
	case zoneID:
		return zones.Zone{ID: zoneID, Kind: zones.KindDistrict, Name: "Хамовники",
			NameIn: "в Хамовниках", NameGen: "Хамовников"}, true
	case station:
		return zones.Zone{ID: station, Kind: zones.KindStation, Name: "Спортивная",
			NameIn: "у метро Спортивная", NameGen: "метро Спортивная"}, true
	default:
		return zones.Zone{}, false
	}
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
