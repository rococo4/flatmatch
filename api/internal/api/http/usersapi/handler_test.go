package usersapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/users"
	"flatmatch/api/internal/tma"
)

type fakeAccounts struct {
	user    entity.User
	sources []string
	err     error
}

type fakeBot struct{}

type fakeValidator struct{}

func (f *fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) UpdateProfile(context.Context, uuid.UUID, entity.ProfilePatch) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) SetPhoto(context.Context, uuid.UUID, io.ReadSeeker) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) Submit(context.Context, uuid.UUID) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) SetSource(_ context.Context, _ uuid.UUID, raw string) error {
	f.sources = append(f.sources, raw)
	return f.err
}

func (f *fakeAccounts) OwnPhotoURL(entity.User) string { return "" }

func (fakeBot) Username() string { return "flatmatch_bot" }

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func newTestRouter(accounts *fakeAccounts) chi.Router {
	r := chi.NewRouter()
	r.Use(authmw.New(fakeValidator{}, accounts, nil))
	r.Mount("/", New(accounts, fakeBot{}).Routes())
	return r
}

func postSource(router chi.Router, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/me/source", strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSetSourceStoresTheCode(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusDraft}}

	rec := postSource(newTestRouter(accounts), `{"source":"hsetndr"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(accounts.sources) != 1 || accounts.sources[0] != "hsetndr" {
		t.Errorf("sources = %v, want [hsetndr]", accounts.sources)
	}
}

func TestSetSourceReportsAnInvalidCode(t *testing.T) {
	accounts := &fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusDraft},
		err:  &entity.FieldsError{Err: users.ErrInvalidSource, Fields: []string{"source"}},
	}

	rec := postSource(newTestRouter(accounts), `{"source":"НЕ КОД"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "invalid_field") || !strings.Contains(rec.Body.String(), "source") {
		t.Errorf("body = %s, want invalid_field on the source field", rec.Body)
	}
}

func TestSetSourceRefusesABrokenBody(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusDraft}}

	rec := postSource(newTestRouter(accounts), `{"src":"hsetndr"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if len(accounts.sources) != 0 {
		t.Errorf("an unreadable body reached the service: %v", accounts.sources)
	}
}
