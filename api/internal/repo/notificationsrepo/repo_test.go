package notificationsrepo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

// Test users live in their own negative tg_id range so leftovers never collide
// with real accounts or with the other repos' tests: each cleanup wipes its
// whole window, so a shared base makes parallel packages delete each other's
// rows. Taken: -993 points, -994 usersrepo, -995 matchesrepo, -996 likesrepo,
// -997 pools, -998 searchesrepo.
const testTGBase int64 = -992000000

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// pgx uses the extended protocol: one statement per Exec, no semicolons.
	// Order follows the foreign keys — notifications reference users.
	clean := func() {
		for _, q := range []string{
			`DELETE FROM notifications WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up test notifications: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

func seedUser(t *testing.T, pool *pgxpool.Pool, n int64, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', $2, 'male', 'any') RETURNING id`,
		testTGBase-n, status).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func blockBot(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET push_blocked_at = now() WHERE id = $1`, userID); err != nil {
		t.Fatalf("block the bot for %s: %v", userID, err)
	}
}

func notif(userID uuid.UUID, typ entity.NotificationType, actor, context string) entity.Notification {
	return entity.Notification{UserID: userID, Type: typ, ActorKey: actor, ContextKey: context}
}

func mustInsert(t *testing.T, repo *Repo, n entity.Notification) bool {
	t.Helper()
	inserted, err := repo.Insert(t.Context(), n)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}
	return inserted
}

// pendingFor фильтрует общую очередь до своих адресатов: тесты соседних пакетов
// идут параллельно и в ту же таблицу.
func pendingFor(t *testing.T, repo *Repo, users ...uuid.UUID) []entity.Notification {
	t.Helper()
	all, err := repo.Pending(t.Context(), 200)
	if err != nil {
		t.Fatalf("Pending() failed: %v", err)
	}
	var out []entity.Notification
	for _, n := range all {
		for _, u := range users {
			if n.UserID == u {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

func ids(ns []entity.Notification) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func sentAt(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT sent_at FROM notifications WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatalf("read sent_at of %s: %v", id, err)
	}
	return at
}

// Дубли невозможны на уровне БД (§7) — но КОНТЕКСТ входит в ключ: подписчик, у
// которого зону новичка пересекают два поиска, ждёт двух пушей, по одному на
// свой пул, и схлопывать их в один нельзя.
func TestInsertDedupsByActorWithinOneContext(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")
	other := seedUser(t, pool, 2, "active")
	actor := uuid.NewString()

	if !mustInsert(t, repo, notif(me, entity.NotifZoneNew, actor, "d-khamovniki")) {
		t.Fatal("the first Insert() reported no new row")
	}
	if mustInsert(t, repo, notif(me, entity.NotifZoneNew, actor, "d-khamovniki")) {
		t.Error("the same actor in the same context inserted twice")
	}
	if !mustInsert(t, repo, notif(me, entity.NotifZoneNew, actor, "line-1")) {
		t.Error("the same actor in another context was swallowed as a duplicate")
	}
	// Дедуп персональный: событие одного актора уходит всем подписчикам.
	if !mustInsert(t, repo, notif(other, entity.NotifZoneNew, actor, "d-khamovniki")) {
		t.Error("another addressee was swallowed as a duplicate")
	}
	// Тип — часть ключа: лайк и мэтч от одного человека не одно событие.
	if !mustInsert(t, repo, notif(me, entity.NotifLiked, actor, "")) {
		t.Error("another type was swallowed as a duplicate")
	}

	// Без актора дедупа нет — частичный индекс такие строки не покрывает.
	for range 2 {
		if !mustInsert(t, repo, notif(me, entity.NotifStillSearching, "", "")) {
			t.Error("a row without an actor was deduplicated")
		}
	}

	if n := countRows(t, pool, `SELECT count(*) FROM notifications WHERE user_id = $1`, me); n != 5 {
		t.Errorf("database holds %d rows for one addressee, want 5", n)
	}
}

func TestInsertManyCountsOnlyNewRows(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")
	actor := uuid.NewString()

	if n, err := repo.InsertMany(t.Context(), nil); n != 0 || err != nil {
		t.Errorf("InsertMany(nil) = %d, %v, want 0, nil", n, err)
	}

	first := notif(me, entity.NotifPoolNew, actor, "point-1")
	first.Payload = map[string]any{"station": "Фрунзенская"}
	batch := []entity.Notification{
		first,
		notif(me, entity.NotifPoolNew, actor, "point-2"),
		// Повтор внутри батча: фан-аут не должен ломаться от него.
		notif(me, entity.NotifPoolNew, actor, "point-1"),
	}
	inserted, err := repo.InsertMany(t.Context(), batch)
	if err != nil {
		t.Fatalf("InsertMany() failed: %v", err)
	}
	if inserted != 2 {
		t.Errorf("InsertMany() inserted %d rows, want 2", inserted)
	}

	queue := pendingFor(t, repo, me)
	if len(queue) != 2 {
		t.Fatalf("queue holds %d rows, want 2", len(queue))
	}
	byContext := map[string]entity.Notification{}
	for _, n := range queue {
		byContext[n.ContextKey] = n
	}
	// Payload доезжает как есть: воркер берёт из него текст, а не собирает заново.
	stored := byContext["point-1"]
	if got := stored.Payload["station"]; got != "Фрунзенская" {
		t.Errorf("payload station = %v, want Фрунзенская", got)
	}
	if stored.Type != entity.NotifPoolNew || stored.ActorKey != actor ||
		stored.Attempts != 0 || stored.SentAt != nil || stored.FailedAt != nil {
		t.Errorf("row lost its identity: %+v", stored)
	}
	// Строка без payload читается пустой картой, а не json null.
	if second := byContext["point-2"]; len(second.Payload) != 0 {
		t.Errorf("payload of a plain row = %v, want empty", second.Payload)
	}
}

// Забаненный и заблокировавший бота из очереди выпадают: первое — правило
// видимости (§4), второе — заведомо мёртвая отправка.
func TestPendingSkipsUnreachableAddressees(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	live := seedUser(t, pool, 1, "active")
	banned := seedUser(t, pool, 2, "banned")
	blocked := seedUser(t, pool, 3, "active")
	blockBot(t, pool, blocked)
	// Черновик и «на проверке» пуши получают: вердикт модератора приходит именно им.
	draft := seedUser(t, pool, 4, "draft")

	for _, user := range []uuid.UUID{live, banned, blocked, draft} {
		mustInsert(t, repo, notif(user, entity.NotifLiked, uuid.NewString(), ""))
	}

	queue := pendingFor(t, repo, live, banned, blocked, draft)
	if len(queue) != 2 {
		t.Fatalf("queue holds %d rows, want 2 (live and draft)", len(queue))
	}
	for _, n := range queue {
		if n.UserID != live && n.UserID != draft {
			t.Errorf("queue holds a row for an unreachable addressee %s", n.UserID)
		}
	}

	// Лимит — это порция одного прохода воркера, а не фильтр.
	got, err := repo.Pending(t.Context(), 1)
	if err != nil {
		t.Fatalf("Pending() failed: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("Pending(1) returned %d rows, want 1", len(got))
	}
}

func TestPendingOrdersOldestFirst(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")

	newer := uuid.NewString()
	mustInsert(t, repo, notif(me, entity.NotifLiked, newer, ""))
	older := uuid.NewString()
	mustInsert(t, repo, notif(me, entity.NotifLiked, older, ""))
	if _, err := pool.Exec(t.Context(), `
		UPDATE notifications SET created_at = now() - interval '1 hour'
		WHERE user_id = $1 AND actor_key = $2`, me, older); err != nil {
		t.Fatalf("backdate a row: %v", err)
	}

	queue := pendingFor(t, repo, me)
	if len(queue) != 2 || queue[0].ActorKey != older || queue[1].ActorKey != newer {
		t.Errorf("queue order = %+v, want the older row first", queue)
	}
}

func TestMarkSentKeepsTheFirstMarkAndBumpCounts(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")

	mustInsert(t, repo, notif(me, entity.NotifLiked, uuid.NewString(), ""))
	mustInsert(t, repo, notif(me, entity.NotifMatched, uuid.NewString(), ""))
	queue := pendingFor(t, repo, me)
	if len(queue) != 2 {
		t.Fatalf("queue holds %d rows, want 2", len(queue))
	}

	// Ретрай: попытки считаются на строке, чтобы после пятой её погасить.
	failing := queue[0].ID
	for range 2 {
		if err := repo.Bump(t.Context(), failing); err != nil {
			t.Fatalf("Bump() failed: %v", err)
		}
	}
	for _, n := range pendingFor(t, repo, me) {
		want := 0
		if n.ID == failing {
			want = 2
		}
		if n.Attempts != want {
			t.Errorf("attempts of %s = %d, want %d", n.ID, n.Attempts, want)
		}
	}

	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	if err := repo.MarkSent(t.Context(), ids(queue), at); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if left := pendingFor(t, repo, me); len(left) != 0 {
		t.Errorf("queue still holds %d sent rows", len(left))
	}
	if got := sentAt(t, pool, failing); got == nil || !got.Equal(at) {
		t.Fatalf("sent_at = %v, want %v", got, at)
	}
	// Второй проход не должен сдвигать окно агрегата, посчитанное по отправке.
	if err := repo.MarkSent(t.Context(), ids(queue), time.Now()); err != nil {
		t.Fatalf("second MarkSent() failed: %v", err)
	}
	if got := sentAt(t, pool, failing); got == nil || !got.Equal(at) {
		t.Errorf("sent_at moved to %v, want it kept at %v", got, at)
	}

	// Отправленную строку MarkFailed не трогает — иначе успех превратился бы в
	// провал.
	if err := repo.MarkFailed(t.Context(), ids(queue)); err != nil {
		t.Fatalf("MarkFailed() failed: %v", err)
	}
	if n := countRows(t, pool,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND failed_at IS NOT NULL`, me); n != 0 {
		t.Errorf("%d sent rows were marked failed", n)
	}

	// А ждущую — гасит: пятая неудача выводит строку из очереди навсегда.
	mustInsert(t, repo, notif(me, entity.NotifPointOK, uuid.NewString(), ""))
	last := pendingFor(t, repo, me)
	if len(last) != 1 {
		t.Fatalf("queue holds %d rows, want 1", len(last))
	}
	if err := repo.MarkFailed(t.Context(), ids(last)); err != nil {
		t.Fatalf("MarkFailed() failed: %v", err)
	}
	if left := pendingFor(t, repo, me); len(left) != 0 {
		t.Errorf("queue still holds %d failed rows", len(left))
	}
}

func TestLastSentAtIsPerContext(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")

	lastSent := func(typ entity.NotificationType, contextKey string) *time.Time {
		t.Helper()
		at, err := repo.LastSentAt(t.Context(), me, typ, contextKey)
		if err != nil {
			t.Fatalf("LastSentAt() failed: %v", err)
		}
		return at
	}

	if at := lastSent(entity.NotifZoneNew, "d-khamovniki"); at != nil {
		t.Errorf("LastSentAt() before anything was sent = %v, want nil", at)
	}

	// Ждущая строка окно не открывает: агрегат считается по отправкам.
	mustInsert(t, repo, notif(me, entity.NotifZoneNew, uuid.NewString(), "d-khamovniki"))
	pending := pendingFor(t, repo, me)
	if at := lastSent(entity.NotifZoneNew, "d-khamovniki"); at != nil {
		t.Errorf("LastSentAt() of a pending row = %v, want nil", at)
	}

	old := time.Now().Add(-30 * time.Hour).UTC().Truncate(time.Millisecond)
	if err := repo.MarkSent(t.Context(), ids(pending), old); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if at := lastSent(entity.NotifZoneNew, "d-khamovniki"); at == nil || !at.Equal(old) {
		t.Errorf("LastSentAt() = %v, want %v", at, old)
	}
	// Соседняя зона своё окно не наследует, как и другой тип.
	if at := lastSent(entity.NotifZoneNew, "line-1"); at != nil {
		t.Errorf("LastSentAt() of another context = %v, want nil", at)
	}
	if at := lastSent(entity.NotifPoolNew, "d-khamovniki"); at != nil {
		t.Errorf("LastSentAt() of another type = %v, want nil", at)
	}

	// Свежая отправка по тому же контексту закрывает окно заново.
	mustInsert(t, repo, notif(me, entity.NotifZoneNew, uuid.NewString(), "d-khamovniki"))
	fresh := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.MarkSent(t.Context(), ids(pendingFor(t, repo, me)), fresh); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if at := lastSent(entity.NotifZoneNew, "d-khamovniki"); at == nil || !at.Equal(fresh) {
		t.Errorf("LastSentAt() = %v, want the newest send %v", at, fresh)
	}
}

// Предохранитель считает ЛИЧНЫЕ отправки: зонные и пуловые держит своё окно, а
// сворачивать в хвост можно только то, что уходило поштучно.
func TestSentPersonalCountCountsOnlyPersonalSends(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")
	other := seedUser(t, pool, 2, "active")

	personal := []entity.NotificationType{
		entity.NotifLiked, entity.NotifMatched, entity.NotifProfileOK,
		entity.NotifPhotoRejected, entity.NotifAboutRejected,
		entity.NotifPointOK, entity.NotifPointRejected,
	}
	for _, typ := range personal {
		mustInsert(t, repo, notif(me, typ, uuid.NewString(), ""))
	}
	for _, typ := range []entity.NotificationType{entity.NotifZoneNew, entity.NotifPoolNew} {
		mustInsert(t, repo, notif(me, typ, uuid.NewString(), "d-khamovniki"))
	}
	mustInsert(t, repo, notif(other, entity.NotifLiked, uuid.NewString(), ""))

	count := func(since time.Time) int {
		t.Helper()
		n, err := repo.SentPersonalCount(t.Context(), me, since)
		if err != nil {
			t.Fatalf("SentPersonalCount() failed: %v", err)
		}
		return n
	}
	day := time.Now().Add(-entity.AggregateWindow)

	// Ждущие строки не считаются: предохранитель про доставленное.
	if n := count(day); n != 0 {
		t.Errorf("SentPersonalCount() before any send = %d, want 0", n)
	}

	sent := time.Now().Add(-time.Hour)
	if err := repo.MarkSent(t.Context(), ids(pendingFor(t, repo, me)), sent); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if n := count(day); n != len(personal) {
		t.Errorf("SentPersonalCount() = %d, want %d — агрегируемые в счёт не идут", n, len(personal))
	}
	// Окно скользящее: что ушло раньше since, бюджет уже не держит.
	if n := count(time.Now().Add(-time.Minute)); n != 0 {
		t.Errorf("SentPersonalCount() inside a minute = %d, want 0", n)
	}
	// Счётчик персональный: чужие отправки в него не попадают.
	if err := repo.MarkSent(t.Context(), ids(pendingFor(t, repo, other)), sent); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if n := count(day); n != len(personal) {
		t.Errorf("SentPersonalCount() after another user's send = %d, want %d", n, len(personal))
	}
}

func TestFailAllPendingEmptiesOneQueue(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")
	other := seedUser(t, pool, 2, "active")

	for range 3 {
		mustInsert(t, repo, notif(me, entity.NotifLiked, uuid.NewString(), ""))
	}
	mustInsert(t, repo, notif(other, entity.NotifLiked, uuid.NewString(), ""))
	sentRow := pendingFor(t, repo, me)[0]
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	if err := repo.MarkSent(t.Context(), []uuid.UUID{sentRow.ID}, at); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}

	if err := repo.FailAllPending(t.Context(), me); err != nil {
		t.Fatalf("FailAllPending() failed: %v", err)
	}
	if left := pendingFor(t, repo, me); len(left) != 0 {
		t.Errorf("queue still holds %d rows", len(left))
	}
	// Отправленное остаётся отправленным: воркер не переписывает историю.
	if got := sentAt(t, pool, sentRow.ID); got == nil || !got.Equal(at) {
		t.Errorf("sent_at = %v, want %v", got, at)
	}
	if n := countRows(t, pool,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND failed_at IS NOT NULL`, me); n != 2 {
		t.Errorf("%d rows failed, want 2", n)
	}
	// Чужая очередь не тронута: блокировка бота — личное дело адресата.
	if left := pendingFor(t, repo, other); len(left) != 1 {
		t.Errorf("another user's queue holds %d rows, want 1", len(left))
	}
}

// Хвост «И ещё N лайков за сегодня» живёт в payload отправленной строки:
// message_id — что редактировать, tail_count — сколько лайков сведено.
func TestLikedTailLivesInTheSentRow(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active")

	tail := func(since time.Time) *TailMessage {
		t.Helper()
		got, err := repo.LastLikedTail(t.Context(), me, since)
		if err != nil {
			t.Fatalf("LastLikedTail() failed: %v", err)
		}
		return got
	}
	day := time.Now().Add(-entity.AggregateWindow)

	if got := tail(day); got != nil {
		t.Errorf("LastLikedTail() with nothing sent = %+v, want nil", got)
	}

	mustInsert(t, repo, notif(me, entity.NotifLiked, uuid.NewString(), ""))
	carrier := pendingFor(t, repo, me)[0]
	sent := time.Now().Add(-2 * time.Hour)
	if err := repo.MarkSent(t.Context(), []uuid.UUID{carrier.ID}, sent); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	// Отправленный лайк без запомненного message_id носителем не становится:
	// редактировать нечего.
	if got := tail(day); got != nil {
		t.Errorf("LastLikedTail() without a message_id = %+v, want nil", got)
	}

	if err := repo.SetTail(t.Context(), carrier.ID, 4242, 1); err != nil {
		t.Fatalf("SetTail() failed: %v", err)
	}
	got := tail(day)
	if got == nil || got.ID != carrier.ID || got.MessageID != 4242 || got.Count != 1 {
		t.Fatalf("LastLikedTail() = %+v, want the carrier with message 4242 and count 1", got)
	}

	// Свёрнутый лайк уходит без своего сообщения — носитель остаётся тем же, а
	// счётчик растёт.
	mustInsert(t, repo, notif(me, entity.NotifLiked, uuid.NewString(), ""))
	swallowed := pendingFor(t, repo, me)[0]
	if err := repo.MarkSent(t.Context(), []uuid.UUID{swallowed.ID}, time.Now()); err != nil {
		t.Fatalf("MarkSent() failed: %v", err)
	}
	if err := repo.SetTail(t.Context(), carrier.ID, 4242, got.Count+1); err != nil {
		t.Fatalf("SetTail() failed: %v", err)
	}
	if got = tail(day); got == nil || got.ID != carrier.ID || got.Count != 2 {
		t.Errorf("LastLikedTail() = %+v, want the same carrier with count 2", got)
	}

	// «За сегодня» — окно: вчерашнее сообщение редактировать поздно, пойдёт новое.
	if got := tail(time.Now().Add(-time.Hour)); got != nil {
		t.Errorf("LastLikedTail() outside the window = %+v, want nil", got)
	}
}
