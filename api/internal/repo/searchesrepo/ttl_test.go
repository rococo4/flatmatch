package searchesrepo

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

var (
	ttlQuiet      = time.Duration(entity.TTLQuietDays) * 24 * time.Hour
	ttlAfterMatch = time.Duration(entity.TTLAfterMatchDays) * 24 * time.Hour
	ttlAnswer     = time.Duration(entity.TTLAnswerDays) * 24 * time.Hour
)

// backdate стареет поиск: TTL считается от отметок, а не от нашего терпения.
func backdate(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, column string, ago time.Duration) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`UPDATE searches SET `+column+` = now() - make_interval(secs => $2) WHERE id = $1`,
		id, ago.Seconds())
	if err != nil {
		t.Fatalf("backdate %s of %s: %v", column, id, err)
	}
}

func seedMatch(t *testing.T, pool *pgxpool.Pool, a, b uuid.UUID, ago time.Duration) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO matches (a_id, b_id, created_at)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid),
		        now() - make_interval(secs => $3))`,
		a, b, ago.Seconds())
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
}

func lastActiveAt(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) time.Time {
	t.Helper()
	var at time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT last_active_at FROM searches WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatalf("read last_active_at of %s: %v", id, err)
	}
	return at
}

func staleIDs(t *testing.T, repo *Repo, owners ...uuid.UUID) []uuid.UUID {
	t.Helper()
	stale, err := repo.StaleForTTL(t.Context(), ttlQuiet, ttlAfterMatch)
	if err != nil {
		t.Fatalf("StaleForTTL() failed: %v", err)
	}
	var out []uuid.UUID
	for _, s := range stale {
		// Тесты соседних пакетов идут параллельно и в ту же таблицу.
		if slices.Contains(owners, s.UserID) {
			out = append(out, s.ID)
		}
	}
	return out
}

// Touch зовётся на каждом авторизованном запросе, поэтому пишет он не чаще раза
// в час: условие живёт в SQL, а не в памяти процесса (§1 спеки).
func TestTouchMovesFreshnessAtMostHourly(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active", "male", "any")
	other := seedUser(t, pool, 2, "active", "male", "any")

	mine, err := repo.Create(t.Context(), me, testParams("d-khamovniki"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	theirs, err := repo.Create(t.Context(), other, testParams("line-1"))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	fresh := lastActiveAt(t, pool, mine.ID)
	if err := repo.Touch(t.Context(), me); err != nil {
		t.Fatalf("Touch() failed: %v", err)
	}
	if got := lastActiveAt(t, pool, mine.ID); !got.Equal(fresh) {
		t.Errorf("Touch() rewrote a fresh mark: %v -> %v", fresh, got)
	}

	backdate(t, pool, mine.ID, "last_active_at", 90*time.Minute)
	if err := repo.Touch(t.Context(), me); err != nil {
		t.Fatalf("Touch() failed: %v", err)
	}
	if got := lastActiveAt(t, pool, mine.ID); time.Since(got) > time.Minute {
		t.Errorf("last_active_at = %v, want now", got)
	}

	// Чужие подписки заход в приложение не освежает.
	backdate(t, pool, theirs.ID, "last_active_at", 90*time.Minute)
	if err := repo.Touch(t.Context(), me); err != nil {
		t.Fatalf("Touch() failed: %v", err)
	}
	if got := lastActiveAt(t, pool, theirs.ID); time.Since(got) < 80*time.Minute {
		t.Errorf("another user's search was touched: %v", got)
	}

	// Закрытый поиск свежести не имеет: он не в пулах и пушей по нему нет.
	closed, err := repo.Close(t.Context(), me, mine.ID, entity.CloseChangedMind)
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	backdate(t, pool, closed.ID, "last_active_at", 90*time.Minute)
	if err := repo.Touch(t.Context(), me); err != nil {
		t.Fatalf("Touch() failed: %v", err)
	}
	if got := lastActiveAt(t, pool, closed.ID); time.Since(got) < 80*time.Minute {
		t.Errorf("a closed search was touched: %v", got)
	}
}

// «Ещё ищешь?» — три ветки: молчание, повторный вопрос после своего окна и
// дедлайн после мэтча (§3).
func TestStaleForTTLBranches(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	fresh := seedUser(t, pool, 1, "active", "male", "any")
	quiet := seedUser(t, pool, 2, "active", "male", "any")
	asked := seedUser(t, pool, 3, "active", "male", "any")
	longAsked := seedUser(t, pool, 4, "active", "male", "any")
	matched := seedUser(t, pool, 5, "active", "male", "any")
	justMatched := seedUser(t, pool, 6, "active", "male", "any")
	peer := seedUser(t, pool, 7, "active", "male", "any")

	create := func(userID uuid.UUID) entity.Search {
		t.Helper()
		s, err := repo.Create(t.Context(), userID, testParams("d-khamovniki"))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		return s
	}
	owners := []uuid.UUID{fresh, quiet, asked, longAsked, matched, justMatched}

	// Свежий поиск спрашивать не о чем.
	create(fresh)

	// Ветка 1: молчит дольше окна, вопрос не задавали.
	quietSearch := create(quiet)
	backdate(t, pool, quietSearch.ID, "last_active_at", ttlQuiet+time.Hour)

	// Ветка 2: спросили — ждём ответа, второй раз не дёргаем; а когда протухнет
	// и вопрос, спросим снова.
	askedSearch := create(asked)
	backdate(t, pool, askedSearch.ID, "last_active_at", ttlQuiet+time.Hour)
	backdate(t, pool, askedSearch.ID, "ttl_asked_at", time.Hour)
	longAskedSearch := create(longAsked)
	backdate(t, pool, longAskedSearch.ID, "last_active_at", 2*ttlQuiet)
	backdate(t, pool, longAskedSearch.ID, "ttl_asked_at", ttlQuiet+time.Hour)

	// Ветка 3: после мэтча дедлайн короче — мэтч старше afterMatch и случился
	// позже последней активности.
	matchedSearch := create(matched)
	backdate(t, pool, matchedSearch.ID, "last_active_at", ttlAfterMatch+2*time.Hour)
	seedMatch(t, pool, matched, peer, ttlAfterMatch+time.Hour)
	// Свежий мэтч ещё не повод: пара только начала переписываться.
	justMatchedSearch := create(justMatched)
	backdate(t, pool, justMatchedSearch.ID, "last_active_at", ttlAfterMatch)
	seedMatch(t, pool, justMatched, peer, time.Hour)

	want := []uuid.UUID{quietSearch.ID, longAskedSearch.ID, matchedSearch.ID}
	got := staleIDs(t, repo, owners...)
	if len(got) != len(want) {
		t.Fatalf("StaleForTTL() returned %d searches, want %d (%v)", len(got), len(want), got)
	}
	for _, id := range want {
		if !slices.Contains(got, id) {
			t.Errorf("StaleForTTL() missed search %s", id)
		}
	}

	// Ответ «Да, ищу» — это свежая активность: поиск уходит из выборки, а метка
	// вопроса возвращает его туда, только протухнув.
	if err := repo.MarkTTLAsked(t.Context(), quietSearch.ID); err != nil {
		t.Fatalf("MarkTTLAsked() failed: %v", err)
	}
	if got := staleIDs(t, repo, quiet); len(got) != 0 {
		t.Errorf("a just-asked search is still stale: %v", got)
	}

	// Пауза и закрытие выводят поиск из TTL: спрашивать больше не о чем.
	paused, err := repo.Close(t.Context(), matched, matchedSearch.ID, entity.CloseFoundHere)
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if got := staleIDs(t, repo, matched); len(got) != 0 {
		t.Errorf("a closed search (%s) is still stale: %v", paused.ID, got)
	}
}

func TestPauseUnansweredGasesOnlySilence(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	silent := seedUser(t, pool, 1, "active", "male", "any")
	answered := seedUser(t, pool, 2, "active", "male", "any")
	waiting := seedUser(t, pool, 3, "active", "male", "any")
	never := seedUser(t, pool, 4, "active", "male", "any")

	create := func(userID uuid.UUID) entity.Search {
		t.Helper()
		s, err := repo.Create(t.Context(), userID, testParams("d-khamovniki"))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		return s
	}

	// Спросили и не дождались.
	silentSearch := create(silent)
	backdate(t, pool, silentSearch.ID, "last_active_at", 2*ttlAnswer)
	backdate(t, pool, silentSearch.ID, "ttl_asked_at", ttlAnswer+time.Hour)
	// Ответили — активность свежее вопроса.
	answeredSearch := create(answered)
	backdate(t, pool, answeredSearch.ID, "ttl_asked_at", ttlAnswer+time.Hour)
	// Спросили только что — дедлайн ещё не вышел.
	waitingSearch := create(waiting)
	backdate(t, pool, waitingSearch.ID, "last_active_at", 2*ttlAnswer)
	backdate(t, pool, waitingSearch.ID, "ttl_asked_at", time.Hour)
	// Не спрашивали вовсе: молчание само по себе поиск не гасит — гасит
	// неотвеченный вопрос.
	neverSearch := create(never)
	backdate(t, pool, neverSearch.ID, "last_active_at", 10*ttlAnswer)

	paused, err := repo.PauseUnanswered(t.Context(), ttlAnswer)
	if err != nil {
		t.Fatalf("PauseUnanswered() failed: %v", err)
	}
	if !slices.Contains(paused, silentSearch.ID) {
		t.Errorf("PauseUnanswered() = %v, want it to hold %s", paused, silentSearch.ID)
	}
	for _, id := range []uuid.UUID{answeredSearch.ID, waitingSearch.ID, neverSearch.ID} {
		if slices.Contains(paused, id) {
			t.Errorf("PauseUnanswered() paused %s", id)
		}
	}

	status := func(userID, id uuid.UUID) entity.SearchStatus {
		t.Helper()
		s, err := repo.ByID(t.Context(), userID, id)
		if err != nil {
			t.Fatalf("ByID() failed: %v", err)
		}
		return s.Status
	}
	if got := status(silent, silentSearch.ID); got != entity.SearchPaused {
		t.Errorf("the silent search is %s, want paused", got)
	}
	for _, pair := range [][2]uuid.UUID{
		{answered, answeredSearch.ID}, {waiting, waitingSearch.ID}, {never, neverSearch.ID},
	} {
		if got := status(pair[0], pair[1]); got != entity.SearchActive {
			t.Errorf("search %s is %s, want active", pair[1], got)
		}
	}

	// Второй проход того же поиска не находит: он уже не активен.
	again, err := repo.PauseUnanswered(t.Context(), ttlAnswer)
	if err != nil {
		t.Fatalf("PauseUnanswered() failed: %v", err)
	}
	if slices.Contains(again, silentSearch.ID) {
		t.Errorf("PauseUnanswered() paused %s twice", silentSearch.ID)
	}
}
