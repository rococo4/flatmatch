package searchesrepo

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func testParams(zone string) entity.SearchParams {
	move := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return entity.SearchParams{
		ZoneID:   zone,
		PriceMin: pointer.To(35000),
		PriceMax: 45000,
		// Комнатность разная по краям: перепутанные местами min/max видны.
		RoomsMin: pointer.To(2),
		RoomsMax: pointer.To(3),
		MoveIn:   &move,
		Term:     pointer.To(entity.TermYear),
		HasStuff: []entity.StuffChip{entity.StuffFurniture},
	}
}

// poolSeenAt читает отметку напрямую: тестам про MarkPoolSeen нужно смотреть и
// на закрытые поиски, которые ByID отдаёт, но проверять удобнее одним запросом.
func poolSeenAt(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) time.Time {
	t.Helper()
	var at time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT pool_seen_at FROM searches WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatalf("read pool_seen_at of %s: %v", id, err)
	}
	return at
}

func TestCreateAndListRoundtrip(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	created, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if created.Status != entity.SearchActive || created.ZoneID != "d-khamovniki" {
		t.Errorf("unexpected search: %+v", created)
	}

	list, err := repo.ListByUser(t.Context(), user)
	if err != nil {
		t.Fatalf("ListByUser() failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
	// Round-trip всех опциональных полей: то, что положили, то и вернулось.
	got := list[0]
	if got.PriceMin == nil || *got.PriceMin != 35000 || got.Term == nil ||
		*got.Term != entity.TermYear || got.MoveIn == nil ||
		!got.MoveIn.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		len(got.HasStuff) != 1 {
		t.Errorf("fields lost in round-trip: %+v", got)
	}
	// Комнатность отдельной проверкой: перепутанные min/max прошли бы условие
	// «поле не потерялось», а вилка поиска от этого переворачивается.
	if got.RoomsMin == nil || *got.RoomsMin != 2 || got.RoomsMax == nil || *got.RoomsMax != 3 {
		t.Errorf("rooms lost or swapped in round-trip: %+v", got)
	}
}

// Порядок «Моих поисков» (§6): активные выше архива, свежие выше старых.
func TestListByUserOrdersActiveFirstThenNewest(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	var searches []entity.Search
	for _, zone := range []string{"d-khamovniki", "line-1", "s-sokol"} {
		s, err := repo.Create(t.Context(), user, testParams(zone))
		if err != nil {
			t.Fatalf("Create(%s) failed: %v", zone, err)
		}
		searches = append(searches, s)
	}
	// Закрываем средний: если бы сортировка шла только по created_at, он
	// остался бы посередине — а должен уехать в хвост.
	if _, err := repo.Close(t.Context(), user, searches[1].ID, entity.CloseChangedMind); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	list, err := repo.ListByUser(t.Context(), user)
	if err != nil {
		t.Fatalf("ListByUser() failed: %v", err)
	}
	want := []uuid.UUID{searches[2].ID, searches[0].ID, searches[1].ID}
	if len(list) != len(want) {
		t.Fatalf("got %d searches, want %d", len(list), len(want))
	}
	for i, id := range want {
		if list[i].ID != id {
			t.Errorf("position %d = %s (%s, zone %s), want %s",
				i, list[i].ID, list[i].Status, list[i].ZoneID, id)
		}
	}
}

func TestCreateEnforcesLimitUnderConcurrency(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	// 5 параллельных созданий: активных должно стать ровно 3 (§2.1),
	// остальные — entity.ErrSearchLimit. Блокировка строки юзера в Create
	// делает исход детерминированным.
	errs := make([]error, 5)

	// Прогрев пула: установка соединения — это TCP и аутентификация, миллисекунды.
	// Без прогрева горутины расходятся по времени настолько, что первая успевает
	// закоммититься до старта последней, и гонки, ради которой тест написан,
	// попросту не происходит.
	conns := make([]*pgxpool.Conn, len(errs))
	for i := range conns {
		c, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatalf("warm up pool: %v", err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Release()
	}

	// Барьер: горутины ждут на закрытии канала и стартуют разом.
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.Create(t.Context(), user, testParams("d-khamovniki"))
		}()
	}
	close(start)
	wg.Wait()

	var ok, limited int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, entity.ErrSearchLimit):
			limited++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 3 || limited != 2 {
		t.Errorf("ok=%d limited=%d, want 3/2", ok, limited)
	}

	// Главная проверка — состояние базы, а не счёт ошибок: протёкший лимит
	// виден здесь, даже если вызовы отчитались как надо.
	var active int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FROM searches
		WHERE user_id = $1 AND status = 'active'`, user).Scan(&active); err != nil {
		t.Fatalf("count active searches: %v", err)
	}
	if active != entity.MaxActiveSearches {
		t.Errorf("database holds %d active searches, want %d", active, entity.MaxActiveSearches)
	}
}

func TestCloseFreesSlotAndReactivateChecksIt(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	var searches []entity.Search
	for range 3 {
		s, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		searches = append(searches, s)
	}

	closed, err := repo.Close(t.Context(), user, searches[0].ID, entity.CloseFoundHere)
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if closed.Status != entity.SearchClosed || closed.CloseReason != entity.CloseFoundHere {
		t.Errorf("close did not stick: %+v", closed)
	}
	if _, err := repo.Close(t.Context(), user, searches[0].ID, entity.CloseFoundHere); !errors.Is(err, entity.ErrSearchClosed) {
		t.Errorf("double close = %v, want ErrSearchClosed", err)
	}

	// Слот освободился — четвёртый создаётся; реактивация теперь упирается в лимит.
	if _, err := repo.Create(t.Context(), user, testParams("line-1")); err != nil {
		t.Fatalf("Create() after close failed: %v", err)
	}
	if _, err := repo.Reactivate(t.Context(), user, searches[0].ID); !errors.Is(err, entity.ErrSearchLimit) {
		t.Errorf("Reactivate() = %v, want ErrSearchLimit", err)
	}
}

// Обратная сторона предыдущего теста: со свободным слотом архивный поиск
// оживает и теряет причину закрытия (§3 — реактивация из «Моих поисков»).
func TestReactivateRestoresArchivedSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	s, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := repo.Close(t.Context(), user, s.ID, entity.CloseChangedMind); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	back, err := repo.Reactivate(t.Context(), user, s.ID)
	if err != nil {
		t.Fatalf("Reactivate() failed: %v", err)
	}
	if back.Status != entity.SearchActive || back.CloseReason != "" {
		t.Errorf("reactivated search = %+v, want active without close reason", back)
	}
	if back.ZoneID != s.ZoneID || back.PriceMax != s.PriceMax {
		t.Errorf("parameters changed on reactivation: %+v", back)
	}
	// Пул за время архива уехал вперёд — отметка просмотра сбрасывается, иначе
	// бейдж «M новых» посчитает новичков от старой даты.
	if !back.PoolSeenAt.After(s.PoolSeenAt) {
		t.Errorf("pool_seen_at not reset on reactivation: %v then %v", s.PoolSeenAt, back.PoolSeenAt)
	}
}

func TestUpdateParamsOverwritesEverything(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	s, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Форма шлёт параметры целиком: снятое поле должно стать NULL, не остаться.
	updated, err := repo.UpdateParams(t.Context(), user, s.ID,
		entity.SearchParams{ZoneID: "line-1", PriceMax: 50000})
	if err != nil {
		t.Fatalf("UpdateParams() failed: %v", err)
	}
	if updated.ZoneID != "line-1" || updated.PriceMin != nil || updated.MoveIn != nil ||
		updated.Term != nil || len(updated.HasStuff) != 0 ||
		updated.RoomsMin != nil || updated.RoomsMax != nil {
		t.Errorf("old params leaked through: %+v", updated)
	}
}

// Закрытый поиск не редактируется: сначала реактивация. Ошибка обязана
// отличаться от ErrNotFound — фронту надо предложить разные действия.
func TestUpdateParamsOnClosedSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	s, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := repo.Close(t.Context(), user, s.ID, entity.CloseFoundElsewhere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	if _, err := repo.UpdateParams(t.Context(), user, s.ID, testParams("line-1")); !errors.Is(err, entity.ErrSearchClosed) {
		t.Errorf("UpdateParams() on closed = %v, want ErrSearchClosed", err)
	}
}

func TestOwnershipIsChecked(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	owner := seedUser(t, pool, 1, "active", "male", "any")
	other := seedUser(t, pool, 2, "active", "male", "any")

	s, err := repo.Create(t.Context(), owner, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := repo.Close(t.Context(), other, s.ID, entity.CloseChangedMind); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("closing someone else's search = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByID(t.Context(), other, s.ID); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("reading someone else's search = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByID(t.Context(), owner, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("reading a missing search = %v, want ErrNotFound", err)
	}
}

func TestMarkPoolSeen(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	s, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	// Сосед по юзеру в другой зоне и закрытый поиск в той же зоне: отметку
	// двигает только активный поиск именно этой зоны.
	otherZone, err := repo.Create(t.Context(), user, testParams("line-1"))
	if err != nil {
		t.Fatalf("Create() in other zone failed: %v", err)
	}
	sameZoneClosed, err := repo.Create(t.Context(), user, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() of the closed one failed: %v", err)
	}
	if _, err := repo.Close(t.Context(), user, sameZoneClosed.ID, entity.CloseFoundHere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	if err := repo.MarkPoolSeen(t.Context(), user, "d-khamovniki"); err != nil {
		t.Fatalf("MarkPoolSeen() failed: %v", err)
	}

	after, err := repo.ByID(t.Context(), user, s.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if !after.PoolSeenAt.After(s.PoolSeenAt) {
		t.Error("pool_seen_at did not move forward")
	}
	if got := poolSeenAt(t, pool, otherZone.ID); !got.Equal(otherZone.PoolSeenAt) {
		t.Errorf("pool_seen_at of another zone moved: %v then %v", otherZone.PoolSeenAt, got)
	}
	if got := poolSeenAt(t, pool, sameZoneClosed.ID); !got.Equal(sameZoneClosed.PoolSeenAt) {
		t.Errorf("pool_seen_at of a closed search moved: %v then %v", sameZoneClosed.PoolSeenAt, got)
	}
}
