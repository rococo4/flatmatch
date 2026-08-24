package pointsapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"flatmatch/api/internal/api/http/authmw"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/points"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const station = "st-sokol"

const flatForm = `{"type":"flat","url":"https://avito.ru/1","zone_id":"st-sokol",
	"price_total":90000,"photo_keys":["points/flat.jpg"]}`

const homeForm = `{"type":"intent_home","stage":"live","zone_id":"st-sokol",
	"price_share":40000,"photo_keys":["points/room.jpg"]}`

type fakePoints struct {
	created entity.Point
	homeIn  points.CreateIntentHome
	err     error

	card   pointsrepo.FeedPoint
	mine   []pointsrepo.FeedPoint
	closed []uuid.UUID
	cards  int

	photoKey  string
	photoBody []byte

	passes *fakePasses
}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

type fakeDir struct{}

type fakePasses struct {
	targets []uuid.UUID
	err     error
}

func TestCreateAnswersWithTheCardOnReview(t *testing.T) {
	viewer := uuid.New()
	service := &fakePoints{created: entity.Point{
		ID: uuid.New(), Type: entity.PointIntentHome, Stage: pointer.To(entity.StageLive),
		Status: entity.PointPending, Source: entity.SourceUserLink, CreatedBy: &viewer,
		ZoneID: station, PriceShare: pointer.To(40000),
		PhotoKeys: []string{"points/room.jpg"}, CreatedAt: time.Now(),
	}}

	rec := do(t, service, viewer, http.MethodPost, "/points", homeForm)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	body := rec.Body.String()

	for _, want := range []string{`"status":"pending"`, `"stage":"live"`, `"Сокол"`} {
		if !strings.Contains(body, want) {
			t.Errorf("created card %s does not carry %s", body, want)
		}
	}
	if strings.Contains(body, "created_by") || strings.Contains(body, viewer.String()) {
		t.Errorf("created card leaks its creator: %s", body)
	}
}

func TestCreateRejectsEverythingButTheRoom(t *testing.T) {
	cases := map[string]string{
		"неизвестный тип":  `{"type":"townhouse"}`,
		"квартира с рынка": flatForm,
	}

	for name, form := range cases {
		t.Run(name, func(t *testing.T) {
			service := &fakePoints{}

			rec := do(t, service, uuid.New(), http.MethodPost, "/points", form)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"invalid_field"`) ||
				!strings.Contains(rec.Body.String(), `"type"`) {
				t.Errorf("body = %s, want invalid_field on type", rec.Body)
			}
			if service.homeIn.ZoneID != "" {
				t.Errorf("the form reached the service anyway: %+v", service.homeIn)
			}
		})
	}
}

func TestPointLimitSpeaksOfTheRoom(t *testing.T) {
	service := &fakePoints{err: entity.ErrPointLimit}

	rec := do(t, service, uuid.New(), http.MethodPost, "/points", homeForm)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	const want = "Ты уже зовёшь к себе — активных точек может быть две"
	if !strings.Contains(rec.Body.String(), `"point_limit"`) ||
		!strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s, want point_limit with %q", rec.Body, want)
	}
}

func TestSwitchedOffRoutesAreGone(t *testing.T) {
	id := uuid.NewString()
	gone := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/points/" + id + "/enter"},
		{http.MethodDelete, "/points/" + id + "/enter"},
		{http.MethodPost, "/points/" + id + "/vote"},
		{http.MethodPost, "/points/" + id + "/restore"},
		{http.MethodGet, "/points/" + id + "/pool"},
	}

	for _, route := range gone {
		rec := do(t, &fakePoints{}, uuid.New(), route.method, route.path, `{"alive":false}`)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want the route gone (%s)", route.method, route.path, rec.Code, rec.Body)
		}
	}
}

func TestPendingPointIsTheCreatorsAlone(t *testing.T) {
	creator := uuid.New()
	pending := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointPending,
		CreatedBy: &creator, ZoneID: station, CreatedAt: time.Now(),
	}}

	service := &fakePoints{card: pending}
	if rec := do(t, service, creator, http.MethodGet, "/points/"+pending.Point.ID.String(), ""); rec.Code != http.StatusOK {
		t.Errorf("status for the creator = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	rec := do(t, service, uuid.New(), http.MethodGet, "/points/"+pending.Point.ID.String(), "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status for a stranger = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body)
	}
}

func TestMineRouteWinsOverTheCard(t *testing.T) {
	service := &fakePoints{mine: []pointsrepo.FeedPoint{{
		Point: entity.Point{ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station},
	}}}

	rec := do(t, service, uuid.New(), http.MethodGet, "/points/mine", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	for _, want := range []string{`"created"`, `"entered"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("body %s does not carry %s", rec.Body, want)
		}
	}
	if service.cards != 0 {
		t.Errorf("the card handler ran %d time(s) for /mine", service.cards)
	}
}

func TestPassStoresTheRoomAndAnswersEmpty(t *testing.T) {
	service := &fakePoints{}
	id := uuid.New()

	rec := do(t, service, uuid.New(), http.MethodPost, "/points/"+id.String()+"/pass", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(service.passes.targets) != 1 || service.passes.targets[0] != id {
		t.Errorf("passes = %v, want one row for %s", service.passes.targets, id)
	}
}

func TestPassOfAMissingRoomIsNotFound(t *testing.T) {
	service := &fakePoints{passes: &fakePasses{err: entity.ErrNotFound}}

	rec := do(t, service, uuid.New(), http.MethodPost, "/points/"+uuid.NewString()+"/pass", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusNotFound, rec.Body)
	}
}

func TestPassOfMyOwnRoomIsRefused(t *testing.T) {
	service := &fakePoints{passes: &fakePasses{err: entity.ErrSelfTarget}}

	rec := do(t, service, uuid.New(), http.MethodPost, "/points/"+uuid.NewString()+"/pass", "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"invalid_field"`) {
		t.Errorf("body = %s, want invalid_field", rec.Body)
	}
}

func passes(service *fakePoints) *fakePasses {
	if service.passes == nil {
		service.passes = &fakePasses{}
	}
	return service.passes
}

func do(t *testing.T, service *fakePoints, viewer uuid.UUID, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: viewer, Status: entity.StatusActive},
	}, nil))
	router.Mount("/points", New(service, passes(service), stubPhotos{}, fakeDir{}).Routes())

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func (f *fakePoints) CreateIntentHome(_ context.Context, _ uuid.UUID, in points.CreateIntentHome) (entity.Point, error) {
	f.homeIn = in
	return f.created, f.err
}

func (f *fakePoints) Point(_ context.Context, viewerID, _ uuid.UUID) (pointsrepo.FeedPoint, error) {
	f.cards++
	if f.err != nil {
		return pointsrepo.FeedPoint{}, f.err
	}
	p := f.card.Point
	if p.Status != entity.PointActive && (p.CreatedBy == nil || *p.CreatedBy != viewerID) {
		return pointsrepo.FeedPoint{}, entity.ErrNotFound
	}
	return f.card, nil
}

func (f *fakePoints) Mine(context.Context, uuid.UUID) ([]pointsrepo.FeedPoint, []pointsrepo.FeedPoint, error) {
	return f.mine, nil, f.err
}

func (f *fakePoints) Close(_ context.Context, _, pointID uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.closed = append(f.closed, pointID)
	return nil
}

func (f *fakePoints) SavePhoto(_ context.Context, file io.ReadSeeker) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	f.photoBody = body
	return f.photoKey, nil
}

func (fakeDir) Get(id string) (zones.Zone, bool) {
	if id != station {
		return zones.Zone{}, false
	}
	return zones.Zone{ID: station, Kind: zones.KindStation, Name: "Сокол", NameIn: "у метро Сокол", NameGen: "Сокола"}, true
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 777000001, FirstName: "Дев"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}

func TestUploadPhotoReturnsTheKey(t *testing.T) {
	service := &fakePoints{photoKey: "points/a1b2.jpg"}
	body := new(bytes.Buffer)
	form := multipart.NewWriter(body)
	part, err := form.CreateFormFile("photo", "room.jpg")
	if err != nil {
		t.Fatalf("build the form: %v", err)
	}
	if _, err := part.Write([]byte("\xff\xd8\xff\xe0 jpeg body")); err != nil {
		t.Fatalf("write the file: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close the form: %v", err)
	}

	rec := upload(t, service, form.FormDataContentType(), body.String())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"photo_key":"points/a1b2.jpg"`) {
		t.Errorf("body = %s, want the stored key", rec.Body)
	}
	if string(service.photoBody) != "\xff\xd8\xff\xe0 jpeg body" {
		t.Errorf("service got %q, want the uploaded bytes", service.photoBody)
	}
}

func TestUploadPhotoNeedsAMultipartForm(t *testing.T) {
	service := &fakePoints{photoKey: "points/a1b2.jpg"}

	rec := upload(t, service, "application/json", `{"photo":"room.jpg"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if service.photoBody != nil {
		t.Errorf("the service was handed %q from a body with no file", service.photoBody)
	}
}

func TestCloseIsTheCreatorsOwn(t *testing.T) {
	id := uuid.New()
	service := &fakePoints{}

	if rec := do(t, service, uuid.New(), http.MethodPost, "/points/"+id.String()+"/close", ""); rec.Code != http.StatusNoContent {
		t.Errorf("close status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(service.closed) != 1 || service.closed[0] != id {
		t.Errorf("service closed %v, want %s", service.closed, id)
	}

	foreign := &fakePoints{err: entity.ErrNotFound}
	rec := do(t, foreign, uuid.New(), http.MethodPost, "/points/"+id.String()+"/close", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("close of a foreign point = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if len(foreign.closed) != 0 {
		t.Errorf("a foreign point was closed anyway: %v", foreign.closed)
	}
}

func upload(t *testing.T, service *fakePoints, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil))
	router.Mount("/points", New(service, passes(service), stubPhotos{}, fakeDir{}).Routes())

	req := httptest.NewRequest(http.MethodPost, "/points/photo", strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	req.Header.Set("Content-Type", contentType)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestIntentHomeFormCarriesTheDescription(t *testing.T) {
	const form = `{"type":"intent_home","stage":"live","zone_id":"st-sokol",
		"price_share":40000,"photo_keys":["points/room.jpg"],
		"description":"Светлая комната с балконом"}`

	service := &fakePoints{created: entity.Point{
		ID: uuid.New(), Type: entity.PointIntentHome, Status: entity.PointPending,
		Stage: pointer.To(entity.StageLive), ZoneID: station,
		Description: pointer.To("Светлая комната с балконом"),
	}}

	rec := do(t, service, uuid.New(), http.MethodPost, "/points", form)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	if service.homeIn.Description == nil || *service.homeIn.Description != "Светлая комната с балконом" {
		t.Errorf("service saw description %v, want it passed through", service.homeIn.Description)
	}
	if !strings.Contains(rec.Body.String(), `"description":"Светлая комната с балконом"`) {
		t.Errorf("body = %s, want the description back on the card", rec.Body)
	}
}

func (f *fakePasses) PassPoint(_ context.Context, _, pointID uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.targets = append(f.targets, pointID)
	return nil
}
