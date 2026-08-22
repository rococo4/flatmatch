package matchesapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

type fakeMatches struct {
	err     error
	viewer  uuid.UUID
	deleted uuid.UUID
	calls   int
}

type fakePhotos struct{}

type fakeDir struct{}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

func TestDeleteAnswersTheServiceVerdicts(t *testing.T) {
	tests := map[string]struct {
		err  error
		path string
		want int
	}{
		"участник":  {want: http.StatusNoContent},
		"чужой":     {err: entity.ErrNotFound, want: http.StatusNotFound},
		"кривой id": {path: "/matches/not-a-uuid", want: http.StatusBadRequest},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := &fakeMatches{err: tt.err}
			viewer, id := uuid.New(), uuid.New()

			path := tt.path
			if path == "" {
				path = "/matches/" + id.String()
			}
			rec := remove(newRouter(service, viewer), path)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.path != "" {
				if service.calls != 0 {
					t.Errorf("a broken id reached the service %d time(s)", service.calls)
				}
				return
			}
			if service.calls != 1 || service.viewer != viewer || service.deleted != id {
				t.Errorf("service saw %d call(s) from %s about %s, want one from %s about %s",
					service.calls, service.viewer, service.deleted, viewer, id)
			}
		})
	}
}

func TestDeleteWithoutAuthorizationIs401(t *testing.T) {
	service := &fakeMatches{}

	req := httptest.NewRequest(http.MethodDelete, "/matches/"+uuid.NewString(), nil)
	rec := httptest.NewRecorder()
	newRouter(service, uuid.New()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusUnauthorized, rec.Body)
	}
	if service.calls != 0 {
		t.Errorf("an anonymous request reached the service %d time(s)", service.calls)
	}
}

func remove(router chi.Router, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func newRouter(service *fakeMatches, viewer uuid.UUID) chi.Router {
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: viewer, Status: entity.StatusActive},
	}, nil))
	r.Mount("/matches", New(service, fakePhotos{}, fakeDir{}).Routes())
	return r
}

func (f *fakeMatches) List(context.Context, uuid.UUID) ([]matchesrepo.MatchWithPeer, error) {
	return nil, f.err
}

func (f *fakeMatches) Detail(context.Context, uuid.UUID, uuid.UUID) (matchesrepo.MatchWithPeer, matches.Summary, error) {
	return matchesrepo.MatchWithPeer{}, matches.Summary{}, f.err
}

func (f *fakeMatches) Delete(_ context.Context, userID, id uuid.UUID) error {
	f.calls++
	f.viewer, f.deleted = userID, id
	return f.err
}

func (fakePhotos) PhotoURL(entity.User) string { return "" }

func (fakeDir) Get(string) (zones.Zone, bool) { return zones.Zone{}, false }

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}
