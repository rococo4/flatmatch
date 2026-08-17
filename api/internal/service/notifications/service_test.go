package notifications

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type fakeRepo struct {
	rows []entity.Notification
	// ctxErr — отмену запроса строка события переживает, и проверить это можно
	// только тем контекстом, который доехал до репозитория.
	ctxErr error
	err    error
}

func (f *fakeRepo) Insert(ctx context.Context, n entity.Notification) (bool, error) {
	f.ctxErr = ctx.Err()
	if f.err != nil {
		return false, f.err
	}
	f.rows = append(f.rows, n)
	return true, nil
}

func (f *fakeRepo) InsertMany(ctx context.Context, ns []entity.Notification) (int, error) {
	f.ctxErr = ctx.Err()
	if f.err != nil {
		return 0, f.err
	}
	f.rows = append(f.rows, ns...)
	return len(ns), nil
}

func (f *fakeRepo) contexts(userID uuid.UUID) []string {
	var out []string
	for _, n := range f.rows {
		if n.UserID == userID {
			out = append(out, n.ContextKey)
		}
	}
	return out
}

type fakeSearches struct {
	searches []entity.Search
	asked    []string
	err      error
}

func (f *fakeSearches) ActiveIn(_ context.Context, zoneIDs []string) ([]entity.Search, error) {
	f.asked = zoneIDs
	if f.err != nil {
		return nil, f.err
	}
	var out []entity.Search
	for _, s := range f.searches {
		if slices.Contains(zoneIDs, s.ZoneID) {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakePoints struct {
	members []uuid.UUID
	err     error
}

func (f *fakePoints) PoolMemberIDs(_ context.Context, _, _ uuid.UUID) ([]uuid.UUID, error) {
	return f.members, f.err
}

// fakePeople отвечает по адресату: актор в тестах один, и его видимость — это
// свойство пары, которое достаточно завести на смотрящего.
type fakePeople struct {
	invisibleTo  map[uuid.UUID]bool
	matchedWith  map[uuid.UUID]bool
	visibleCalls map[uuid.UUID]int
}

func (f *fakePeople) VisibleBetween(_ context.Context, a, _ uuid.UUID) (bool, error) {
	if f.visibleCalls == nil {
		f.visibleCalls = map[uuid.UUID]int{}
	}
	f.visibleCalls[a]++
	return !f.invisibleTo[a], nil
}

func (f *fakePeople) IsMatched(_ context.Context, a, _ uuid.UUID) (bool, error) {
	return f.matchedWith[a], nil
}

func testDir(t *testing.T) *zones.Directory {
	t.Helper()
	raw, err := os.ReadFile("../zones/testdata/mini.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := zones.New(raw)
	if err != nil {
		t.Fatalf("zones.New: %v", err)
	}
	return d
}

func testService(t *testing.T, repo *fakeRepo, searches *fakeSearches, points *fakePoints, people *fakePeople) *Service {
	t.Helper()
	return New(repo, searches, points, people, testDir(t), slog.New(slog.DiscardHandler))
}

func search(userID uuid.UUID, zoneID string) entity.Search {
	return entity.Search{ID: uuid.New(), UserID: userID, SearchParams: entity.SearchParams{ZoneID: zoneID}}
}

// Зона поиска — граница пула (§2.1): новичок в «красной ветке» виден и тем, кто
// ищет в Хамовниках, потому что станции пересекаются.
func TestZoneNewFansOutToIntersectingSubscribers(t *testing.T) {
	author := uuid.New()
	district, both, elsewhere := uuid.New(), uuid.New(), uuid.New()
	matched, invisible := uuid.New(), uuid.New()

	repo := &fakeRepo{}
	searches := &fakeSearches{searches: []entity.Search{
		search(author, "line-1"),
		search(district, "d-khamovniki"),
		search(both, "line-1"),
		search(both, "d-khamovniki"),
		search(elsewhere, "st-sokol"),
		search(matched, "line-1"),
		search(invisible, "line-1"),
	}}
	people := &fakePeople{
		invisibleTo: map[uuid.UUID]bool{invisible: true},
		matchedWith: map[uuid.UUID]bool{matched: true},
	}
	svc := testService(t, repo, searches, &fakePoints{}, people)

	svc.ZoneNew(t.Context(), search(author, "line-1"))

	if want := []string{"d-khamovniki", "line-1"}; !slices.Equal(sorted(searches.asked), want) {
		t.Errorf("asked for subscribers of %v, want %v", searches.asked, want)
	}
	if len(repo.rows) != 3 {
		t.Fatalf("wrote %d rows, want 3: %+v", len(repo.rows), repo.rows)
	}
	for _, unwanted := range []uuid.UUID{author, elsewhere, matched, invisible} {
		if len(repo.contexts(unwanted)) != 0 {
			t.Errorf("wrote a row for %s — the author, a stranger, a match or an invisible person", unwanted)
		}
	}
	if got := repo.contexts(district); !slices.Equal(got, []string{"d-khamovniki"}) {
		t.Errorf("contexts of the district subscriber = %v, want [d-khamovniki]", got)
	}
	// Два пересекающих поиска — два пула и две строки: контекст у них разный, и
	// уникальный индекс их не схлопнет.
	if got := sorted(repo.contexts(both)); !slices.Equal(got, []string{"d-khamovniki", "line-1"}) {
		t.Errorf("contexts of the double subscriber = %v, want both zones", got)
	}
	// Видимость — вопрос про пару, а не про поиск: второй поиск того же
	// человека не стоит второго запроса.
	if people.visibleCalls[both] != 1 {
		t.Errorf("asked about the visibility of %s %d times, want 1", both, people.visibleCalls[both])
	}

	for _, n := range repo.rows {
		if n.Type != entity.NotifZoneNew || n.ActorKey != author.String() {
			t.Errorf("row %+v, want type zone_new with the author as the actor", n)
		}
	}
}

func TestZoneNewSurvivesFailures(t *testing.T) {
	author := uuid.New()

	broken := &fakeSearches{err: errors.New("no database today")}
	repo := &fakeRepo{}
	testService(t, repo, broken, &fakePoints{}, &fakePeople{}).ZoneNew(t.Context(), search(author, "line-1"))
	if len(repo.rows) != 0 {
		t.Errorf("wrote %d rows without subscribers", len(repo.rows))
	}

	// Упавшая вставка тоже не поднимается наверх: доменная операция уже
	// закоммичена, пуш ей не судья.
	failing := &fakeRepo{err: errors.New("insert failed")}
	svc := testService(t, failing, &fakeSearches{searches: []entity.Search{search(uuid.New(), "line-1")}},
		&fakePoints{}, &fakePeople{})
	svc.ZoneNew(t.Context(), search(author, "line-1"))
}

func TestPoolNewSkipsTheOneWhoEntered(t *testing.T) {
	entered, author, mate, matched, invisible := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pointID := uuid.New()

	repo := &fakeRepo{}
	points := &fakePoints{members: []uuid.UUID{author, mate, matched, invisible, entered}}
	people := &fakePeople{
		invisibleTo: map[uuid.UUID]bool{invisible: true},
		matchedWith: map[uuid.UUID]bool{matched: true},
	}
	svc := testService(t, repo, &fakeSearches{}, points, people)

	svc.PoolNew(t.Context(), pointID, entered)

	if len(repo.rows) != 2 {
		t.Fatalf("wrote %d rows, want 2: %+v", len(repo.rows), repo.rows)
	}
	for _, n := range repo.rows {
		if n.Type != entity.NotifPoolNew || n.ActorKey != entered.String() || n.ContextKey != pointID.String() {
			t.Errorf("row %+v, want pool_new about %s in point %s", n, entered, pointID)
		}
	}
	got := []uuid.UUID{repo.rows[0].UserID, repo.rows[1].UserID}
	for _, want := range []uuid.UUID{author, mate} {
		if !slices.Contains(got, want) {
			t.Errorf("rows went to %v, want the pool members %s and %s", got, author, mate)
		}
	}
}

func TestLikedCarriesTheName(t *testing.T) {
	to, from := uuid.New(), uuid.New()
	repo := &fakeRepo{}
	svc := testService(t, repo, &fakeSearches{}, &fakePoints{}, &fakePeople{})

	// Отменённый контекст: лайк уже записан, и пуш о нём не должен пропасть
	// вместе с отвалившимся клиентом.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc.Liked(ctx, to, from, "Катя", entity.GenderFemale)

	if repo.ctxErr != nil {
		t.Errorf("the repository saw a cancelled context: %v", repo.ctxErr)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("wrote %d rows, want 1", len(repo.rows))
	}
	n := repo.rows[0]
	if n.UserID != to || n.Type != entity.NotifLiked || n.ActorKey != from.String() {
		t.Errorf("row %+v, want a liked push to %s from %s", n, to, from)
	}
	if n.Payload["name"] != "Катя" || n.Payload["gender"] != string(entity.GenderFemale) {
		t.Errorf("payload %v, want the name and the gender of the sender", n.Payload)
	}
}

func TestMatchedTellsBothSides(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	match := entity.Match{ID: uuid.New(), AID: a, BID: b}

	repo := &fakeRepo{}
	testService(t, repo, &fakeSearches{}, &fakePoints{}, &fakePeople{}).Matched(t.Context(), match)

	if len(repo.rows) != 2 {
		t.Fatalf("wrote %d rows, want one per side", len(repo.rows))
	}
	for i, want := range []uuid.UUID{a, b} {
		n := repo.rows[i]
		// Актор — мэтч: гонка встречных лайков приводит сюда дважды, и вторую
		// пару строк снимает уникальный индекс.
		if n.UserID != want || n.Type != entity.NotifMatched || n.ActorKey != match.ID.String() {
			t.Errorf("row %+v, want a matched push to %s about %s", n, want, match.ID)
		}
	}
}

func TestProfileVerdict(t *testing.T) {
	userID := uuid.New()
	repo := &fakeRepo{}
	svc := testService(t, repo, &fakeSearches{}, &fakePoints{}, &fakePeople{})

	svc.ProfileVerdict(t.Context(), userID, entity.NotifPhotoRejected)
	// Не вердикт по анкете — не пуш: тип приходит из вызова, и опечатка в нём
	// не должна становиться строкой очереди.
	svc.ProfileVerdict(t.Context(), userID, entity.NotifZoneNew)

	if len(repo.rows) != 1 {
		t.Fatalf("wrote %d rows, want 1: %+v", len(repo.rows), repo.rows)
	}
	n := repo.rows[0]
	if n.UserID != userID || n.Type != entity.NotifPhotoRejected {
		t.Errorf("row %+v, want a photo_rejected push to %s", n, userID)
	}
	// Вердикты повторяются, поэтому актора им даёт время: два реджекта фото —
	// два разных ключа.
	prefix := userID.String() + ":" + string(entity.NotifPhotoRejected) + ":"
	if !strings.HasPrefix(n.ActorKey, prefix) {
		t.Fatalf("actor %q, want the prefix %q", n.ActorKey, prefix)
	}
	if at := n.ActorKey[len(prefix):]; at == "" || at == "0" {
		t.Errorf("actor %q carries no verdict time", n.ActorKey)
	}
}

func TestPointVerdict(t *testing.T) {
	creator := uuid.New()
	mine := entity.Point{ID: uuid.New(), CreatedBy: pointer.To(creator)}
	parsed := entity.Point{ID: uuid.New()}

	repo := &fakeRepo{}
	svc := testService(t, repo, &fakeSearches{}, &fakePoints{}, &fakePeople{})

	svc.PointVerdict(t.Context(), mine, true)
	// У парсера и досыпки создателя нет (§8) — адресата тоже.
	svc.PointVerdict(t.Context(), parsed, false)

	if len(repo.rows) != 1 {
		t.Fatalf("wrote %d rows, want 1: %+v", len(repo.rows), repo.rows)
	}
	n := repo.rows[0]
	if n.UserID != creator || n.Type != entity.NotifPointOK || n.ContextKey != mine.ID.String() {
		t.Errorf("row %+v, want a point_ok push to %s about %s", n, creator, mine.ID)
	}

	repo.rows = nil
	svc.PointVerdict(t.Context(), mine, false)
	if len(repo.rows) != 1 || repo.rows[0].Type != entity.NotifPointRejected {
		t.Fatalf("rows %+v, want a point_rejected push", repo.rows)
	}
	if !strings.HasPrefix(repo.rows[0].ActorKey, mine.ID.String()+":") {
		t.Errorf("actor %q, want the point as the subject", repo.rows[0].ActorKey)
	}
}

func sorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
