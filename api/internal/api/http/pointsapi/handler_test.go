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
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
	"flatmatch/api/internal/tma"
)

const station = "st-sokol"

const flatForm = `{"type":"flat","url":"https://avito.ru/1","zone_id":"st-sokol",
	"price_total":90000,"photo_keys":["points/flat.jpg"]}`

const homeForm = `{"type":"intent_home","stage":"live","zone_id":"st-sokol",
	"price_share":40000,"photo_keys":["points/room.jpg"]}`

// fakePoints stands in for the service and keeps its gates: the card of a
// point still on review is the creator's alone (§5), and here that rule is
// what the handler must not route around.
type fakePoints struct {
	created entity.Point
	err     error

	card    pointsrepo.FeedPoint
	mine    []pointsrepo.FeedPoint
	pool    points.PoolView
	entered []uuid.UUID
	left    []uuid.UUID
	closed  []uuid.UUID
	cards   int

	photoKey  string
	photoBody []byte
}

type fakeValidator struct{}

type fakeAccounts struct{ user entity.User }

type fakeDir struct{}

func TestCreateAnswersWithTheCardOnReview(t *testing.T) {
	viewer := uuid.New()
	service := &fakePoints{created: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointPending,
		Source: entity.SourceUserLink, CreatedBy: &viewer, ZoneID: station,
		PriceTotal: pointer.To(90000), PhotoKeys: []string{"points/flat.jpg"},
		URLNorm: pointer.To("https://avito.ru/1"), CreatedAt: time.Now(),
	}}

	rec := do(t, service, viewer, http.MethodPost, "/points", flatForm)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusCreated, rec.Body)
	}
	body := rec.Body.String()
	// Создателю точка видна сразу, «на проверке» (§5), и в пуле своей квартиры
	// он стоит с первой секунды — «ты первый в пуле» (§6).
	for _, want := range []string{`"status":"pending"`, `"in_pool":true`, `"Сокол"`} {
		if !strings.Contains(body, want) {
			t.Errorf("created card %s does not carry %s", body, want)
		}
	}
	if strings.Contains(body, "created_by") || strings.Contains(body, viewer.String()) {
		t.Errorf("created card leaks its creator: %s", body)
	}
}

// «Уже живу здесь» / «договорился и заезжаю» — гейт формы (§2.2); всё
// остальное не наш кейс, и ручка отвечает про поле, а не пятисоткой из CHECK.
func TestCreateRejectsAnUnknownType(t *testing.T) {
	service := &fakePoints{}

	rec := do(t, service, uuid.New(), http.MethodPost, "/points", `{"type":"townhouse"}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"invalid_field"`) ||
		!strings.Contains(rec.Body.String(), `"type"`) {
		t.Errorf("body = %s, want invalid_field on type", rec.Body)
	}
}

// Дубль ссылки — не тупик: активная точка зовёт войти в свой пул, точка на
// проверке ещё ничья, и входить в неё некуда (§5, §6).
func TestDuplicateURLBranchesByStatus(t *testing.T) {
	existing := entity.Point{ID: uuid.New(), Type: entity.PointFlat, ZoneID: station}

	cases := map[string]struct {
		status entity.PointStatus
		pool   int
		want   string
		// offersToEnter: точку можно назвать и предложить войти. У точки на
		// проверке ни того, ни другого: до аппрува её видит только подавший
		// (§5), и «в пуле N» на чужой ссылке стало бы оракулом видимости (§4).
		offersToEnter bool
	}{
		"активная зовёт в пул": {
			status: entity.PointActive, pool: 3, offersToEnter: true,
			want: "Эта квартира уже есть, в пуле 3 — войти?",
		},
		"на проверке — только сообщение": {
			status: entity.PointPending,
			want:   "Эта квартира уже добавлена и ждёт проверки",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dup := existing
			dup.Status = tc.status
			service := &fakePoints{err: &points.ErrDuplicateURL{Existing: dup, PoolCount: tc.pool}}

			rec := do(t, service, uuid.New(), http.MethodPost, "/points", flatForm)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusConflict, rec.Body)
			}
			body := rec.Body.String()
			for _, want := range []string{`"duplicate_url"`, tc.want, `"status":"` + string(tc.status) + `"`} {
				if !strings.Contains(body, want) {
					t.Errorf("409 body %s does not carry %s", body, want)
				}
			}
			if got := strings.Contains(body, existing.ID.String()); got != tc.offersToEnter {
				t.Errorf("409 body names the point = %v, want %v: %s", got, tc.offersToEnter, body)
			}
			if !tc.offersToEnter && !strings.Contains(body, `"pool_count":0`) {
				t.Errorf("409 body %s carries a pool count of a point on review", body)
			}
		})
	}
}

// Лимитов два, и человеку важно, в какой он упёрся: пять ссылок с рынка (§8)
// и две точки «к себе» (§2.2) — разные истории и разные выходы.
func TestPointLimitSpeaksByFormType(t *testing.T) {
	cases := map[string]struct {
		form string
		want string
	}{
		"квартира": {flatForm, "Пять квартир уже добавлено — закрой одну в Моих поисках"},
		"к себе":   {homeForm, "Ты уже зовёшь к себе — активных точек может быть две"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			service := &fakePoints{err: entity.ErrPointLimit}

			rec := do(t, service, uuid.New(), http.MethodPost, "/points", tc.form)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusBadRequest, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"point_limit"`) ||
				!strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("body = %s, want point_limit with %q", rec.Body, tc.want)
			}
		})
	}
}

// Точка на проверке для всех, кроме создателя, не существует (§5) — тот же
// 404, что и у несуществующей.
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

// «Мои» — не uuid: разъедься порядок роутов, блоки «Моих поисков» уходили бы
// в разбор идентификатора.
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

// «В пуле N — увидишь после входа» (§6): вход бесплатен и безлимитен, поэтому
// гейт стоит на нём.
func TestPoolNeedsEntering(t *testing.T) {
	service := &fakePoints{err: entity.ErrNeedEnter}

	rec := do(t, service, uuid.New(), http.MethodGet, "/points/"+uuid.NewString()+"/pool", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusForbidden, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"need_enter"`) ||
		!strings.Contains(rec.Body.String(), "Пул откроется после входа") {
		t.Errorf("body = %s, want need_enter with the invitation to enter", rec.Body)
	}
}

func TestPoolMembersCarryNoContacts(t *testing.T) {
	service := &fakePoints{pool: points.PoolView{Members: []points.MemberView{{
		Member: pointsrepo.Member{
			User: entity.User{
				ID: uuid.New(), TelegramID: 777000123, Name: "Катя", Username: "katya",
				Profile:     entity.Profile{Age: 27, Schedule: entity.ScheduleOwl},
				PhotoKey:    "users/katya.jpg",
				PhotoStatus: entity.ModerationOK,
			},
			Search: &entity.Search{SearchParams: entity.SearchParams{
				ZoneID: station, PriceMax: 45000,
			}},
			EnteredAt: time.Now(),
		},
		Chips:         []pools.Chip{{Param: pools.ParamPrice, Verdict: pools.VerdictGreen}},
		SharedAnswers: 4,
	}}}}

	rec := do(t, service, uuid.New(), http.MethodGet, "/points/"+uuid.NewString()+"/pool", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, http.StatusOK, rec.Body)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"username", "tg_id", "katya\"", "777000123"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("pool JSON leaks %q:\n%s", forbidden, body)
		}
	}
	// Вердикт-чипы, совпадения анкеты и зона поиска участника — то, из чего
	// собрана мини-карточка пула (§6).
	for _, want := range []string{`"shared_answers":4`, `"verdict":"green"`, `"price_max":45000`, `"Сокол"`} {
		if !strings.Contains(body, want) {
			t.Errorf("pool card %s does not carry %s", body, want)
		}
	}
}

// Вход и выход — один тап и никакого тела в ответе; отказ входа приходит от
// репозитория: у «к себе» и у точки на проверке пула нет (§2.2, §5).
func TestEnterAndLeave(t *testing.T) {
	viewer := uuid.New()
	id := uuid.New()
	service := &fakePoints{}

	if rec := do(t, service, viewer, http.MethodPost, "/points/"+id.String()+"/enter", ""); rec.Code != http.StatusNoContent {
		t.Errorf("enter status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if rec := do(t, service, viewer, http.MethodDelete, "/points/"+id.String()+"/enter", ""); rec.Code != http.StatusNoContent {
		t.Errorf("leave status = %d, want %d (%s)", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(service.entered) != 1 || service.entered[0] != id || len(service.left) != 1 || service.left[0] != id {
		t.Errorf("service saw entered=%v left=%v, want both %s", service.entered, service.left, id)
	}

	refusing := &fakePoints{err: entity.ErrNotFound}
	if rec := do(t, refusing, viewer, http.MethodPost, "/points/"+id.String()+"/enter", ""); rec.Code != http.StatusNotFound {
		t.Errorf("enter into a point without a pool = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func do(t *testing.T, service *fakePoints, viewer uuid.UUID, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Use(authmw.New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: viewer, Status: entity.StatusActive},
	}))
	router.Mount("/points", New(service, stubPhotos{}, fakeDir{}).Routes())

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func (f *fakePoints) CreateFlat(context.Context, uuid.UUID, points.CreateFlat) (entity.Point, error) {
	return f.created, f.err
}

func (f *fakePoints) CreateIntentHome(context.Context, uuid.UUID, points.CreateIntentHome) (entity.Point, error) {
	return f.created, f.err
}

// Point repeats the service's gate: всё, кроме одобренной точки, видит только
// создатель (§5).
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

func (f *fakePoints) Enter(_ context.Context, _, pointID uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.entered = append(f.entered, pointID)
	return nil
}

func (f *fakePoints) Leave(_ context.Context, _, pointID uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.left = append(f.left, pointID)
	return nil
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

func (f *fakePoints) Pool(context.Context, entity.User, uuid.UUID) (points.PoolView, error) {
	return f.pool, f.err
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

// Фото уходит до самой точки: форма грузит его, пока точки ещё нет, и получает
// ключ, который приложит к ней при сабмите. Анкеты автора это не касается —
// у фото точки нет своего статуса модерации (§5).
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

// Без формы грузить нечего — это ошибка запроса, а не пятисотка.
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

// Закрыть точку может только создатель (§5): чужая отвечает 404 и остаётся на
// месте, своя закрывается без тела в ответе.
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
	}))
	router.Mount("/points", New(service, stubPhotos{}, fakeDir{}).Routes())

	req := httptest.NewRequest(http.MethodPost, "/points/photo", strings.NewReader(body))
	req.Header.Set("Authorization", "tma stub")
	req.Header.Set("Content-Type", contentType)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
