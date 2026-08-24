package usersapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
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

	added    int
	removed  []int
	reorders [][]int
	photoErr error
}

type fakeBot struct{}

type fakeValidator struct{}

func (f *fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) UpdateProfile(context.Context, uuid.UUID, entity.ProfilePatch) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) AddPhoto(context.Context, uuid.UUID, io.ReadSeeker) (entity.User, error) {
	f.added++
	return f.user, f.photoErr
}

func (f *fakeAccounts) RemovePhoto(_ context.Context, _ uuid.UUID, index int) (entity.User, error) {
	f.removed = append(f.removed, index)
	return f.user, f.photoErr
}

func (f *fakeAccounts) ReorderPhotos(_ context.Context, _ uuid.UUID, order []int) (entity.User, error) {
	f.reorders = append(f.reorders, order)
	return f.user, f.photoErr
}

func (f *fakeAccounts) Submit(context.Context, uuid.UUID) (entity.User, error) {
	return f.user, nil
}

func (f *fakeAccounts) SetSource(_ context.Context, _ uuid.UUID, raw string) error {
	f.sources = append(f.sources, raw)
	return f.err
}

func (f *fakeAccounts) OwnPhotoURLs(u entity.User) []string {
	out := make([]string, 0, len(u.PhotoKeys))
	for _, key := range u.PhotoKeys {
		out = append(out, "/media/"+key)
	}
	return out
}

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

func call(router chi.Router, method, path, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func multipartPhoto(t *testing.T) (string, string) {
	t.Helper()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("photo", "face.jpg")
	if err != nil {
		t.Fatalf("build the multipart body: %v", err)
	}
	if _, err := part.Write([]byte("\xff\xd8\xff\xe0")); err != nil {
		t.Fatalf("write the photo: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close the multipart body: %v", err)
	}
	return form.FormDataContentType(), body.String()
}

func TestOwnProfileCarriesThePhotoSet(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{
		ID:        uuid.New(),
		Status:    entity.StatusActive,
		PhotoKeys: []string{"photos/a.jpg", "photos/b.jpg"},
	}}

	rec := call(newTestRouter(accounts), http.MethodGet, "/me", "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := `"photo_urls":["/media/photos/a.jpg","/media/photos/b.jpg"]`
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s, want %s", rec.Body, want)
	}
}

func TestAddPhotoTakesAMultipartFile(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusDraft}}
	contentType, body := multipartPhoto(t)

	rec := call(newTestRouter(accounts), http.MethodPost, "/me/photos", contentType, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	if accounts.added != 1 {
		t.Errorf("service saw %d uploads, want 1", accounts.added)
	}
}

func TestAddPhotoReportsAFullSet(t *testing.T) {
	accounts := &fakeAccounts{
		user:     entity.User{ID: uuid.New(), Status: entity.StatusActive},
		photoErr: &entity.FieldsError{Err: users.ErrPhotoLimit, Fields: []string{"photo"}},
	}
	contentType, body := multipartPhoto(t)

	rec := call(newTestRouter(accounts), http.MethodPost, "/me/photos", contentType, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "photo_limit") {
		t.Errorf("body = %s, want photo_limit", rec.Body)
	}
}

func TestRemovePhotoPassesTheIndexThrough(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusActive}}
	router := newTestRouter(accounts)

	if rec := call(router, http.MethodDelete, "/me/photos/2", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	if len(accounts.removed) != 1 || accounts.removed[0] != 2 {
		t.Errorf("service saw %v, want [2]", accounts.removed)
	}

	rec := call(router, http.MethodDelete, "/me/photos/first", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a non-numeric index (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if len(accounts.removed) != 1 {
		t.Errorf("a non-numeric index reached the service: %v", accounts.removed)
	}
}

func TestRemovePhotoReportsTheLastOne(t *testing.T) {
	accounts := &fakeAccounts{
		user:     entity.User{ID: uuid.New(), Status: entity.StatusActive},
		photoErr: &entity.FieldsError{Err: users.ErrLastPhoto, Fields: []string{"photo"}},
	}

	rec := call(newTestRouter(accounts), http.MethodDelete, "/me/photos/0", "", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "last_photo") {
		t.Errorf("body = %s, want last_photo", rec.Body)
	}
}

func TestReorderPhotosPassesTheOrderThrough(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusActive}}

	rec := call(newTestRouter(accounts), http.MethodPut, "/me/photos/order",
		"application/json", `{"order":[2,0,1]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	if len(accounts.reorders) != 1 || !slices.Equal(accounts.reorders[0], []int{2, 0, 1}) {
		t.Errorf("service saw %v, want [[2 0 1]]", accounts.reorders)
	}
}

func TestReorderPhotosRefusesABrokenBody(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusActive}}

	rec := call(newTestRouter(accounts), http.MethodPut, "/me/photos/order",
		"application/json", `{"ordering":[0]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if len(accounts.reorders) != 0 {
		t.Errorf("an unreadable body reached the service: %v", accounts.reorders)
	}
}

func TestTheOldSinglePhotoRouteIsGone(t *testing.T) {
	accounts := &fakeAccounts{user: entity.User{ID: uuid.New(), Status: entity.StatusDraft}}
	contentType, body := multipartPhoto(t)

	rec := call(newTestRouter(accounts), http.MethodPost, "/me/photo", contentType, body)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want the retired route gone (%s)", rec.Code, rec.Body)
	}
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
