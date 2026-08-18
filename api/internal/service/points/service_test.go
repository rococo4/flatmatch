package points

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/imagetype"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

const (
	station      = "st-sportivnaya"
	district     = "d-khamovniki"
	otherStation = "st-sokol"
)

type fakeRepo struct {
	byURL    []*entity.Point
	urlCalls []string

	createIn    entity.Point
	createCalls int
	createErr   error

	card      pointsrepo.FeedPoint
	cardErr   error
	cardCalls int

	feedZones  []string
	countZones []string

	closed   uuid.UUID
	closedBy uuid.UUID
	closeErr error

	inPool       bool
	members      []pointsrepo.Member
	membersCalls int

	entered    bool
	enterErr   error
	enterCalls int
}

type fakeSearches struct {
	list []entity.Search
}

type fakePhotos struct {
	keys []string
	err  error
}

type fakeNotifier struct {
	got  []entity.Point
	ctxs []context.Context
}

func (f *fakeRepo) Create(_ context.Context, p entity.Point) (entity.Point, error) {
	f.createCalls++
	f.createIn = p
	if f.createErr != nil {
		return entity.Point{}, f.createErr
	}
	p.ID = uuid.New()
	if p.Status == "" {
		p.Status = entity.PointPending
	}
	return p, nil
}

func (f *fakePhotos) Put(_ context.Context, key string, _ io.Reader, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	return nil
}

func (f *fakeRepo) Close(_ context.Context, userID, pointID uuid.UUID) error {
	f.closedBy, f.closed = userID, pointID
	return f.closeErr
}

func (f *fakeRepo) ByURLNorm(_ context.Context, urlNorm string) (*entity.Point, error) {
	f.urlCalls = append(f.urlCalls, urlNorm)
	if len(f.byURL) == 0 {
		return nil, nil
	}
	out := f.byURL[0]
	f.byURL = f.byURL[1:]
	return out, nil
}

func (f *fakeRepo) CardByID(context.Context, uuid.UUID, uuid.UUID) (pointsrepo.FeedPoint, error) {
	f.cardCalls++
	return f.card, f.cardErr
}

func (f *fakeRepo) Feed(_ context.Context, _ uuid.UUID, zoneIDs []string) ([]pointsrepo.FeedPoint, error) {
	f.feedZones = zoneIDs
	return nil, nil
}

func (f *fakeRepo) Mine(context.Context, uuid.UUID) ([]pointsrepo.FeedPoint, []pointsrepo.FeedPoint, error) {
	return nil, nil, nil
}

func (f *fakeRepo) PoolMembers(context.Context, uuid.UUID, uuid.UUID) ([]pointsrepo.Member, error) {
	f.membersCalls++
	return f.members, nil
}

func (f *fakeRepo) CountActiveIn(_ context.Context, zoneIDs []string) (int, error) {
	f.countZones = zoneIDs
	return len(zoneIDs), nil
}

func (f *fakeRepo) Enter(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	f.enterCalls++
	return f.entered, f.enterErr
}

func (f *fakeRepo) Leave(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (f *fakeRepo) InPool(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.inPool, nil
}

func (f *fakeSearches) ListByUser(context.Context, uuid.UUID) ([]entity.Search, error) {
	return f.list, nil
}

func (f *fakeNotifier) PointPending(ctx context.Context, p entity.Point) {
	f.got = append(f.got, p)
	f.ctxs = append(f.ctxs, ctx)
}

func newService(t *testing.T, repo *fakeRepo, searches *fakeSearches, notify Notifier) *Service {
	t.Helper()
	return New(repo, searches, &fakePhotos{}, mustDir(t), notify, nil)
}

func mustDir(t *testing.T) *zones.Directory {
	t.Helper()
	dir, err := zones.Load()
	if err != nil {
		t.Fatalf("load zones: %v", err)
	}
	return dir
}

func validFlat() CreateFlat {
	return CreateFlat{
		URL:        "https://www.avito.ru/moskva/kvartiry/2-k_kvartira_1234567/?context=abc",
		ZoneID:     station,
		PriceTotal: 90000,
		PhotoKeys:  []string{"points/screen.jpg"},
	}
}

func validHome() CreateIntentHome {
	return CreateIntentHome{
		Stage:      entity.StageLive,
		ZoneID:     station,
		PriceShare: 45000,
		PhotoKeys:  []string{"points/room.jpg"},
	}
}

func fields(t *testing.T, err error) []string {
	t.Helper()
	var fe *entity.FieldsError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v, want a FieldsError", err)
	}
	return fe.Fields
}

func TestNormalizeURL(t *testing.T) {
	const want = "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567"

	good := map[string]string{
		"plain":            "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
		"www":              "https://www.avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
		"mobile mirror":    "https://m.avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
		"query":            "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567?context=H4sI&slocation=1",
		"trailing slash":   "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567/",
		"two slashes":      "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567//",
		"percent-encoded":  "https://avito.ru/moskva/kvartiry/2-k_kv%61rtira_1234567",
		"http":             "http://avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
		"no scheme":        "avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
		"uppercase host":   "https://AVITO.RU/moskva/kvartiry/2-k_kvartira_1234567",
		"spaces around it": "  https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567  ",
		"fragment":         "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567#photo",
	}
	for name, raw := range good {
		t.Run(name, func(t *testing.T) {
			got, ok := normalizeURL(raw)
			if !ok || got != want {
				t.Errorf("normalizeURL(%q) = %q, %v; want %q, true", raw, got, ok, want)
			}
		})
	}

	bad := map[string]string{
		"empty":            "",
		"other classified": "https://realty.yandex.ru/offer/1",
		"lookalike domain": "https://avito.ru.evil.com/1",
		"subdomain":        "https://spb.avito.ru/kvartira/1",
		"not a url":        "квартира на авито",
		"other scheme":     "ftp://avito.ru/1",

		"bare host":              "https://avito.ru",
		"bare host with a slash": "https://avito.ru/",
		"bare host no scheme":    "avito.ru",
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			if got, ok := normalizeURL(raw); ok {
				t.Errorf("normalizeURL(%q) = %q, true; want it refused", raw, got)
			}
		})
	}

	if got, _ := normalizeURL("https://avito.ru/Moskva/Kvartiry/1"); got != "https://avito.ru/Moskva/Kvartiry/1" {
		t.Errorf("path case changed: %q", got)
	}

	if got, ok := normalizeURL("https://www.cian.ru/rent/flat/123/"); !ok || got != "https://cian.ru/rent/flat/123" {
		t.Errorf("cian link = %q, %v", got, ok)
	}
}

func TestCreateFlatValidatesTheForm(t *testing.T) {
	tests := []struct {
		name string
		with func(*CreateFlat)
		want []string
	}{
		{"cheap", func(f *CreateFlat) { f.PriceTotal = MinPrice - 1 }, []string{"price_total"}},
		{"expensive", func(f *CreateFlat) { f.PriceTotal = MaxPrice + 1 }, []string{"price_total"}},
		{"foreign link", func(f *CreateFlat) { f.URL = "https://realty.yandex.ru/1" }, []string{"url"}},
		{"unknown zone", func(f *CreateFlat) { f.ZoneID = "st-nowhere" }, []string{"zone_id"}},
		{"district instead of a station", func(f *CreateFlat) { f.ZoneID = district }, []string{"zone_id"}},
		{"no photo", func(f *CreateFlat) { f.PhotoKeys[0] = "  " }, []string{"photo_keys"}},
		{"zero rooms", func(f *CreateFlat) { f.Rooms = pointer.To(0) }, []string{"rooms"}},
		{"garbage bathrooms", func(f *CreateFlat) {
			f.Bathrooms = pointer.To(entity.ParseBathrooms("три"))
		}, []string{"bathrooms"}},
		{"garbage utilities", func(f *CreateFlat) {
			f.Utilities = pointer.To(entity.ParseUtilities("как-нибудь"))
		}, []string{"utilities"}},
		{"garbage restrictions", func(f *CreateFlat) {
			f.Restrictions = []entity.Restriction{entity.ParseRestriction("no_children")}
		}, []string{"restrictions"}},
		{"everything at once", func(f *CreateFlat) {
			f.URL, f.PriceTotal, f.PhotoKeys = "", 0, nil
		}, []string{"url", "price_total", "photo_keys"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			s := newService(t, repo, &fakeSearches{}, nil)
			in := validFlat()
			tt.with(&in)

			_, err := s.CreateFlat(t.Context(), uuid.New(), in)
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("fields = %v, want %v", got, tt.want)
			}

			if repo.createCalls != 0 {
				t.Errorf("the form reached the repository %d times", repo.createCalls)
			}
		})
	}
}

func TestCreateFlatFillsProvenance(t *testing.T) {
	repo := &fakeRepo{}
	notify := &fakeNotifier{}
	s := newService(t, repo, &fakeSearches{}, notify)
	author := uuid.New()

	in := validFlat()
	in.Bathrooms = pointer.To(entity.BathroomsSeparate)
	in.Utilities = pointer.To(entity.UtilitiesMeters)
	in.Restrictions = []entity.Restriction{
		entity.RestrictionNoPets, entity.RestrictionNoPets, entity.RestrictionNoSmoking}

	created, err := s.CreateFlat(t.Context(), author, in)
	if err != nil {
		t.Fatalf("CreateFlat() failed: %v", err)
	}

	sent := repo.createIn
	if sent.Type != entity.PointFlat || sent.Source != entity.SourceUserLink ||
		sent.CreatedBy == nil || *sent.CreatedBy != author {
		t.Errorf("provenance = %s / %s / %v, want flat / user_link / %s",
			sent.Type, sent.Source, sent.CreatedBy, author)
	}

	if sent.Status != "" {
		t.Errorf("Status = %q, want it left to the repository", sent.Status)
	}
	if sent.URLNorm == nil || *sent.URLNorm != "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567" {
		t.Errorf("URLNorm = %v, want the normalised link", sent.URLNorm)
	}

	want := []entity.Restriction{entity.RestrictionNoPets, entity.RestrictionNoSmoking}
	if !slices.Equal(sent.Restrictions, want) {
		t.Errorf("Restrictions = %v, want %v", sent.Restrictions, want)
	}

	if len(notify.got) != 1 || notify.got[0].ID != created.ID {
		t.Errorf("moderation queue got %+v, want the created point %s", notify.got, created.ID)
	}
}

func TestModerationSurvivesACancelledRequest(t *testing.T) {
	notify := &fakeNotifier{}
	s := newService(t, &fakeRepo{}, &fakeSearches{}, notify)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := s.CreateFlat(ctx, uuid.New(), validFlat()); err != nil {
		t.Fatalf("CreateFlat() failed: %v", err)
	}
	if len(notify.ctxs) != 1 {
		t.Fatalf("moderation was called %d times, want 1", len(notify.ctxs))
	}
	if err := notify.ctxs[0].Err(); err != nil {
		t.Errorf("moderation got a cancelled context: %v", err)
	}
}

func TestCreateFlatAnswersDuplicateLink(t *testing.T) {
	existing := entity.Point{ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive}

	t.Run("already there", func(t *testing.T) {
		repo := &fakeRepo{
			byURL: []*entity.Point{&existing},
			card:  pointsrepo.FeedPoint{Point: existing, PoolCount: 3},
		}
		s := newService(t, repo, &fakeSearches{}, nil)

		_, err := s.CreateFlat(t.Context(), uuid.New(), validFlat())
		var dup *ErrDuplicateURL
		if !errors.As(err, &dup) || dup.Existing.ID != existing.ID {
			t.Fatalf("CreateFlat() = %v, want ErrDuplicateURL with %s", err, existing.ID)
		}

		if dup.PoolCount != 3 {
			t.Errorf("PoolCount = %d, want 3", dup.PoolCount)
		}
		if repo.createCalls != 0 {
			t.Errorf("a duplicate reached the repository %d times", repo.createCalls)
		}
	})

	t.Run("still on review", func(t *testing.T) {
		pending := entity.Point{ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointPending}
		repo := &fakeRepo{
			byURL: []*entity.Point{&pending},
			card:  pointsrepo.FeedPoint{Point: pending, PoolCount: 1},
		}
		s := newService(t, repo, &fakeSearches{}, nil)

		_, err := s.CreateFlat(t.Context(), uuid.New(), validFlat())
		var dup *ErrDuplicateURL
		if !errors.As(err, &dup) {
			t.Fatalf("CreateFlat() = %v, want ErrDuplicateURL", err)
		}
		if dup.Existing.ID != pending.ID || dup.PoolCount != 0 {
			t.Errorf("duplicate = %s / pool %d, want the pending point with an empty pool",
				dup.Existing.ID, dup.PoolCount)
		}
		if repo.cardCalls != 0 {
			t.Errorf("the pool of a point on review was read %d time(s)", repo.cardCalls)
		}
	})

	t.Run("lost the race", func(t *testing.T) {
		repo := &fakeRepo{
			byURL:     []*entity.Point{nil, &existing},
			createErr: pointsrepo.ErrURLTaken,
		}
		s := newService(t, repo, &fakeSearches{}, nil)

		_, err := s.CreateFlat(t.Context(), uuid.New(), validFlat())
		var dup *ErrDuplicateURL
		if !errors.As(err, &dup) || dup.Existing.ID != existing.ID {
			t.Fatalf("CreateFlat() = %v, want ErrDuplicateURL with %s", err, existing.ID)
		}
		if len(repo.urlCalls) != 2 {
			t.Errorf("ByURLNorm called %d times, want a re-read after the race", len(repo.urlCalls))
		}
	})

	t.Run("nothing to point at", func(t *testing.T) {
		repo := &fakeRepo{byURL: []*entity.Point{nil, nil}, createErr: pointsrepo.ErrURLTaken}
		s := newService(t, repo, &fakeSearches{}, nil)

		_, err := s.CreateFlat(t.Context(), uuid.New(), validFlat())
		var dup *ErrDuplicateURL
		if errors.As(err, &dup) || !errors.Is(err, pointsrepo.ErrURLTaken) {
			t.Errorf("CreateFlat() = %v, want the raw ErrURLTaken", err)
		}
	})
}

func TestCreateIntentHomeValidatesTheStage(t *testing.T) {
	tests := []struct {
		name string
		with func(*CreateIntentHome)
		want []string
	}{
		{"garbage stage", func(h *CreateIntentHome) {
			h.Stage = entity.ParsePointStage("хочу сдать")
		}, []string{"stage"}},
		{"no stage", func(h *CreateIntentHome) { h.Stage = "" }, []string{"stage"}},

		{"moving in without a date", func(h *CreateIntentHome) {
			h.Stage = entity.StageMovingIn
		}, []string{"move_in"}},
		{"share out of range", func(h *CreateIntentHome) { h.PriceShare = 900000 }, []string{"price_share"}},
		{"no room photo", func(h *CreateIntentHome) { h.PhotoKeys = nil }, []string{"photo_keys"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			s := newService(t, repo, &fakeSearches{}, nil)
			in := validHome()
			tt.with(&in)

			_, err := s.CreateIntentHome(t.Context(), uuid.New(), in)
			if got := fields(t, err); !slices.Equal(got, tt.want) {
				t.Errorf("fields = %v, want %v", got, tt.want)
			}
			if repo.createCalls != 0 {
				t.Errorf("the form reached the repository %d times", repo.createCalls)
			}
		})
	}

	repo := &fakeRepo{}
	notify := &fakeNotifier{}
	s := newService(t, repo, &fakeSearches{}, notify)
	in := validHome()
	in.Stage = entity.StageMovingIn
	in.MoveIn = pointer.To(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))

	if _, err := s.CreateIntentHome(t.Context(), uuid.New(), in); err != nil {
		t.Fatalf("CreateIntentHome() failed: %v", err)
	}
	sent := repo.createIn

	if sent.Type != entity.PointIntentHome || sent.Source != entity.SourceUserLink ||
		sent.Stage == nil || *sent.Stage != entity.StageMovingIn ||
		sent.PriceShare == nil || *sent.PriceShare != 45000 || sent.URLNorm != nil {
		t.Errorf("point = %+v, want a moving_in room without a link", sent)
	}
	if len(notify.got) != 1 {
		t.Errorf("moderation queue got %d points, want 1", len(notify.got))
	}
}

func TestPointHidesWhatIsNotApproved(t *testing.T) {
	creator := uuid.New()
	stranger := uuid.New()

	for _, status := range []entity.PointStatus{entity.PointPending, entity.PointClosed} {
		t.Run(string(status), func(t *testing.T) {
			card := pointsrepo.FeedPoint{Point: entity.Point{
				ID: uuid.New(), Type: entity.PointFlat, Status: status, CreatedBy: &creator}}
			s := newService(t, &fakeRepo{card: card}, &fakeSearches{}, nil)

			if _, err := s.Point(t.Context(), stranger, card.Point.ID); !errors.Is(err, entity.ErrNotFound) {
				t.Errorf("Point() for a stranger = %v, want ErrNotFound", err)
			}
			if _, err := s.Point(t.Context(), creator, card.Point.ID); err != nil {
				t.Errorf("Point() for the creator failed: %v", err)
			}
		})
	}

	card := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, CreatedBy: &creator}}
	s := newService(t, &fakeRepo{card: card}, &fakeSearches{}, nil)
	if _, err := s.Point(t.Context(), stranger, card.Point.ID); err != nil {
		t.Errorf("Point() of an active flat failed: %v", err)
	}

	orphan := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Source: entity.SourceManual,
		Status: entity.PointPending, CreatedBy: nil}}
	s = newService(t, &fakeRepo{card: orphan}, &fakeSearches{}, nil)
	for _, viewer := range []uuid.UUID{stranger, uuid.Nil} {
		if _, err := s.Point(t.Context(), viewer, orphan.Point.ID); !errors.Is(err, entity.ErrNotFound) {
			t.Errorf("Point() of an ownerless pending point for %s = %v, want ErrNotFound", viewer, err)
		}
	}
}

func TestPoolOpensAfterEntry(t *testing.T) {
	card := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station}}
	repo := &fakeRepo{card: card, inPool: false}
	s := newService(t, repo, &fakeSearches{}, nil)

	_, err := s.Pool(t.Context(), entity.User{ID: uuid.New()}, card.Point.ID)
	if !errors.Is(err, entity.ErrNeedEnter) {
		t.Errorf("Pool() without entering = %v, want ErrNeedEnter", err)
	}
	if repo.membersCalls != 0 {
		t.Errorf("the pool was read %d times for somebody outside it", repo.membersCalls)
	}
}

func TestPoolChipsNeedBothSearches(t *testing.T) {
	card := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station}}
	withSearch := pointsrepo.Member{
		User: entity.User{ID: uuid.New(), Profile: entity.Profile{Smoking: entity.SmokingNo}},
		Search: &entity.Search{Status: entity.SearchActive, SearchParams: entity.SearchParams{
			ZoneID: district, PriceMin: pointer.To(40000), PriceMax: 55000}},
	}
	without := pointsrepo.Member{
		User: entity.User{ID: uuid.New(), Profile: entity.Profile{Smoking: entity.SmokingNo}}}
	viewer := entity.User{ID: uuid.New(), Profile: entity.Profile{Smoking: entity.SmokingNo}}

	mine := entity.Search{Status: entity.SearchActive, SearchParams: entity.SearchParams{
		ZoneID: district, PriceMin: pointer.To(35000), PriceMax: 45000}}
	elsewhere := entity.Search{Status: entity.SearchActive, SearchParams: entity.SearchParams{
		ZoneID: otherStation, PriceMax: 45000}}
	closed := mine
	closed.Status = entity.SearchClosed

	tests := []struct {
		name      string
		searches  []entity.Search
		wantChips bool
	}{
		{"search over the same station", []entity.Search{mine}, true},
		{"no search at all", nil, false},

		{"search elsewhere", []entity.Search{elsewhere}, false},
		{"closed search", []entity.Search{closed}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{card: card, inPool: true, members: []pointsrepo.Member{withSearch, without}}
			s := newService(t, repo, &fakeSearches{list: tt.searches}, nil)

			view, err := s.Pool(t.Context(), viewer, card.Point.ID)
			if err != nil {
				t.Fatalf("Pool() failed: %v", err)
			}
			if len(view.Members) != 2 {
				t.Fatalf("Pool() returned %d members, want 2", len(view.Members))
			}
			if got := len(view.Members[0].Chips) > 0; got != tt.wantChips {
				t.Errorf("chips against a member with a search: %v, want %v", got, tt.wantChips)
			}

			if len(view.Members[1].Chips) != 0 {
				t.Errorf("member without a search carries chips: %v", view.Members[1].Chips)
			}

			if view.Members[0].SharedAnswers == 0 {
				t.Error("SharedAnswers = 0, want the questionnaire counted")
			}
		})
	}
}

func TestPoolChipsSpeakFromTheViewerSide(t *testing.T) {
	card := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station}}

	viewerParams := entity.SearchParams{
		ZoneID: district, PriceMin: pointer.To(35000), PriceMax: 45000,
		RoomsMin: pointer.To(2), RoomsMax: pointer.To(2),
		MoveIn: pointer.To(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
	}
	member := pointsrepo.Member{
		User: entity.User{ID: uuid.New()},
		Search: &entity.Search{Status: entity.SearchActive, SearchParams: entity.SearchParams{
			ZoneID: district, PriceMin: pointer.To(40000), PriceMax: 55000}},
	}

	repo := &fakeRepo{card: card, inPool: true, members: []pointsrepo.Member{member}}
	s := newService(t, repo, &fakeSearches{list: []entity.Search{
		{Status: entity.SearchActive, SearchParams: viewerParams}}}, nil)

	view, err := s.Pool(t.Context(), entity.User{ID: uuid.New()}, card.Point.ID)
	if err != nil {
		t.Fatalf("Pool() failed: %v", err)
	}
	want := []pools.Chip{
		{Param: pools.ParamPrice, Verdict: pools.VerdictGreen},
		{Param: pools.ParamRooms, Verdict: pools.VerdictNone},
		{Param: pools.ParamMoveIn, Verdict: pools.VerdictNone},
		{Param: pools.ParamTerm, Verdict: pools.VerdictNone},
	}
	if !slices.Equal(view.Members[0].Chips, want) {
		t.Errorf("chips = %v, want %v", view.Members[0].Chips, want)
	}
}

func TestPoolChipsUseTheFreshestSearch(t *testing.T) {
	card := pointsrepo.FeedPoint{Point: entity.Point{
		ID: uuid.New(), Type: entity.PointFlat, Status: entity.PointActive, ZoneID: station}}
	member := pointsrepo.Member{
		User: entity.User{ID: uuid.New()},
		Search: &entity.Search{Status: entity.SearchActive, SearchParams: entity.SearchParams{
			ZoneID: district, PriceMin: pointer.To(40000), PriceMax: 55000}},
	}
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	stale := entity.Search{Status: entity.SearchActive, CreatedAt: base,
		SearchParams: entity.SearchParams{ZoneID: district, PriceMin: pointer.To(100000), PriceMax: 120000}}
	fresh := entity.Search{Status: entity.SearchActive, CreatedAt: base.Add(time.Hour),
		SearchParams: entity.SearchParams{ZoneID: district, PriceMin: pointer.To(35000), PriceMax: 45000}}

	for _, order := range [][]entity.Search{{stale, fresh}, {fresh, stale}} {
		repo := &fakeRepo{card: card, inPool: true, members: []pointsrepo.Member{member}}
		s := newService(t, repo, &fakeSearches{list: order}, nil)

		view, err := s.Pool(t.Context(), entity.User{ID: uuid.New()}, card.Point.ID)
		if err != nil {
			t.Fatalf("Pool() failed: %v", err)
		}

		if got := view.Members[0].Chips[0]; got.Verdict != pools.VerdictGreen {
			t.Errorf("price chip = %v, want green — чипы взяли не свежайший поиск", got.Verdict)
		}
	}
}

func TestFeedAndCountUseIntersectingZones(t *testing.T) {
	repo := &fakeRepo{}
	s := newService(t, repo, &fakeSearches{}, nil)

	if _, err := s.Feed(t.Context(), uuid.New(), "st-nowhere"); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Feed() of an unknown zone = %v, want ErrNotFound", err)
	}
	if _, err := s.Feed(t.Context(), uuid.New(), district); err != nil {
		t.Fatalf("Feed() failed: %v", err)
	}

	if !slices.Contains(repo.feedZones, station) || !slices.Contains(repo.feedZones, district) {
		t.Errorf("feed zones = %v, want the district and its stations", repo.feedZones)
	}

	if _, err := s.CountIn(t.Context(), district); err != nil {
		t.Fatalf("CountIn() failed: %v", err)
	}
	if !slices.Equal(repo.countZones, repo.feedZones) {
		t.Errorf("«M квартир» counts %v, but the feed shows %v", repo.countZones, repo.feedZones)
	}
	if n, err := s.CountIn(t.Context(), "st-nowhere"); err != nil || n != 0 {
		t.Errorf("CountIn() of an unknown zone = %d, %v, want 0, nil", n, err)
	}
}

func TestSavePhotoStoresUnderItsOwnKey(t *testing.T) {
	photos := &fakePhotos{}
	s := New(&fakeRepo{}, &fakeSearches{}, photos, mustDir(t), nil, nil)

	key, err := s.SavePhoto(t.Context(), bytes.NewReader([]byte("\xff\xd8\xff\xe0 jpeg body")))
	if err != nil {
		t.Fatalf("SavePhoto() failed: %v", err)
	}
	if !strings.HasPrefix(key, "points/") {
		t.Errorf("key = %q, want it under the points prefix", key)
	}
	if !strings.HasSuffix(key, ".jpg") {
		t.Errorf("key = %q, want the sniffed extension", key)
	}
	if len(photos.keys) != 1 || photos.keys[0] != key {
		t.Errorf("store got %v, want the returned key %q", photos.keys, key)
	}

	second, err := s.SavePhoto(t.Context(), bytes.NewReader([]byte("\x89PNG\r\n\x1a\n body")))
	if err != nil {
		t.Fatalf("SavePhoto() failed: %v", err)
	}
	if second == key {
		t.Errorf("second upload reused the key %q", key)
	}
	if !strings.HasSuffix(second, ".png") {
		t.Errorf("key = %q, want the sniffed extension", second)
	}

	if _, err := s.SavePhoto(t.Context(), bytes.NewReader([]byte("GIF87a"))); !errors.Is(err, imagetype.ErrUnsupported) {
		t.Errorf("SavePhoto() of a gif = %v, want ErrUnsupported", err)
	}
	if len(photos.keys) != 2 {
		t.Errorf("store holds %d keys, want 2", len(photos.keys))
	}
}

func TestCloseDelegatesToTheRepository(t *testing.T) {
	repo := &fakeRepo{}
	s := newService(t, repo, &fakeSearches{}, nil)
	user, point := uuid.New(), uuid.New()

	if err := s.Close(t.Context(), user, point); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if repo.closedBy != user || repo.closed != point {
		t.Errorf("repo saw close(%s, %s), want (%s, %s)", repo.closedBy, repo.closed, user, point)
	}

	repo.closeErr = entity.ErrNotFound
	if err := s.Close(t.Context(), user, point); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Close() of a foreign point = %v, want ErrNotFound", err)
	}
}
