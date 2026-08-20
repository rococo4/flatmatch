package zonesapi

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
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

type fakeDir struct{}

type fakeTops struct {
	limits []int
	top    []pools.ZoneTop
}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestTopCarriesTheZonesWithTheirCounts(t *testing.T) {
	tops := &fakeTops{top: []pools.ZoneTop{
		{Zone: zones.Zone{ID: "d-khamovniki", Name: "Хамовники", NameIn: "в Хамовниках",
			NameGen: "Хамовников"}, People: 12},
		{Zone: zones.Zone{ID: "s-sokol", Name: "Сокол", NameIn: "у метро Сокол",
			NameGen: "метро Сокол"}, People: 3},
	}}

	rec := get(t, tops, "/zones/top")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	body := strings.TrimSpace(rec.Body.String())
	want := `{"zones":[{"id":"d-khamovniki","name":"Хамовники","name_in":"в Хамовниках","people":12},` +
		`{"id":"s-sokol","name":"Сокол","name_in":"у метро Сокол","people":3}]}`
	if body != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

func TestTopWithoutZonesCarriesAnEmptyList(t *testing.T) {
	rec := get(t, &fakeTops{}, "/zones/top")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"zones":[]}` {
		t.Errorf("body = %s, want an empty list, not null", body)
	}
}

func TestTopLimitDefaultsAndCaps(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", defaultTopLimit},
		{"?limit=3", 3},
		{"?limit=20", maxTopLimit},
		{"?limit=100", maxTopLimit},
	}
	for _, tc := range tests {
		tops := &fakeTops{}
		rec := get(t, tops, "/zones/top"+tc.query)
		if rec.Code != http.StatusOK {
			t.Fatalf("status for %q = %d, want %d (%s)", tc.query, rec.Code, http.StatusOK, rec.Body)
		}
		if len(tops.limits) != 1 || tops.limits[0] != tc.want {
			t.Errorf("%q asked for limits %v, want [%d]", tc.query, tops.limits, tc.want)
		}
	}
}

func TestTopRefusesABrokenLimit(t *testing.T) {
	for _, query := range []string{"?limit=abc", "?limit=0", "?limit=-2"} {
		tops := &fakeTops{}
		rec := get(t, tops, "/zones/top"+query)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status for %q = %d, want %d (%s)", query, rec.Code, http.StatusBadRequest, rec.Body)
		}
		if len(tops.limits) != 0 {
			t.Errorf("%q reached the service with %v", query, tops.limits)
		}
	}
}

func TestDirectoryStaysOnTheRoot(t *testing.T) {
	rec := get(t, &fakeTops{}, "/zones")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	if body := rec.Body.String(); body != `{"version":7}` {
		t.Errorf("body = %s, want the raw directory", body)
	}
	if etag := rec.Header().Get("ETag"); etag != `"zones-v7"` {
		t.Errorf("ETag = %s, want the version of the directory", etag)
	}
}

func get(t *testing.T, tops *fakeTops, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	r.Mount("/zones", New(fakeDir{}, tops).Routes())

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func (fakeDir) Raw() []byte { return []byte(`{"version":7}`) }

func (fakeDir) Version() int { return 7 }

func (f *fakeTops) Top(_ context.Context, _ uuid.UUID, limit int) ([]pools.ZoneTop, error) {
	f.limits = append(f.limits, limit)
	return f.top, nil
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
