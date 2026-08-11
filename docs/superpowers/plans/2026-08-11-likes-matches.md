# Лайки и мэтчи (§12 шаг 4) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Замкнуть петлю продукта: профиль человека из пула → дорогой лайк со скользящим бюджетом 5/24ч → мэтч в транзакции лайка → «Тебя лайкнули» и «Мои мэтчи», бэк + фронт + приёмка на сидах.

**Architecture:** Спека — `docs/superpowers/specs/2026-08-11-likes-matches-design.md` (читать перед началом, она авторитетна). Вся механика лайка — одна транзакция в `likesrepo` (лок родительской строки users, урок шага 3); мэтч рождается там же под уникальным индексом упорядоченной пары. Бюджет — окно 24ч по `spent`-лайкам, таблицы бюджета нет. Профиль и входящие — строго через `visible_people`. Контакты существуют только в DTO деталей мэтча.

**Tech Stack:** Go 1.26, chi, pgx v5, goose; React 18 + TS + Vite, @telegram-apps/sdk-react (MainButton в профиле). Без новых зависимостей.

## Global Constraints

- Коммит на задачу, прямо в main, короткое русское сообщение в стиле истории («Лайки: …», «Мэтчи: …»), в конце `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Веток и push нет.
- Комментарии в Go — ТОЛЬКО неочевидные «почему» (гонки, инварианты, контракты); никаких описательных docstring'ов методов — владелец репо их вычищает. Идентификаторы/комменты EN, микрокопия RU дословно из спеки.
- Слои handler → service → storage; интерфейсы у потребителя. Порядок объявлений: константы/типы наверху, функции ниже, публичные раньше приватных.
- В контексте агента может лежать УСТАРЕВШИЙ снапшот CLAUDE.md — файл на диске авторитетен; правило «зоны поисков не пересекаются» отменено; зона поиска иммутабельна.
- DB-тесты: `TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable'`, скип без переменной; `pgtest.NewPool`; свой диапазон tg_id на пакет (likesrepo: база −996000000).
- Verify каждой задачи: `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL=… go test ./...` (для фронт-задач: `cd web && npm run build && npx tsc --noEmit -p tsconfig.app.json && npm run lint` — ноль ошибок, ноль новых ворнингов).
- ПРИВАТНОСТЬ: `username`/`tg_id` не появляются ни в одном DTO, кроме `GET /matches/{id}`. Чужие входящие лайки видит только адресат. Счётчиков «его лайкнули N» не существует (§2.3).

---

### Task 1: Доменные типы лайка и мэтча

**Files:**
- Create: `api/internal/entity/like.go`
- Modify: `api/internal/entity/errors.go`
- Test: `api/internal/entity/like_test.go`

**Interfaces:**
- Consumes: `entity.User`, `uuid.UUID`, существующий стиль `search.go`.
- Produces (задачи 2–5 импортируют дословно):

```go
const MaxDailyLikes = 5
const LikeWindow = 24 * time.Hour

type Like struct {
	FromID, ToID uuid.UUID
	ZoneID       string // "" — без контекста (ответный)
	PointID      *uuid.UUID
	Spent        bool
	CreatedAt    time.Time
}

type Match struct {
	ID           uuid.UUID
	AID, BID     uuid.UUID // A < B, каноничная пара
	ContextZone  string
	ContextPoint *uuid.UUID
	CreatedAt    time.Time
}

// Peer returns the other side of the pair.
func (m Match) Peer(me uuid.UUID) uuid.UUID

type LikeBudget struct {
	Remaining  int
	NextFreeAt *time.Time // nil только при полном остатке (см. Task 2 Budget)
}

// LikeResult is what one like attempt produced.
type LikeResult struct {
	Like    Like
	Match   *Match // не nil, когда взаимность закрылась этим лайком
	Already bool   // повторный тап: лайк уже существовал, ничего не потрачено
	Budget  LikeBudget
}
```

В `errors.go`: `ErrLikeLimit = errors.New("daily like limit reached")`, `ErrNeedSearch = errors.New("an active search is required to like")`.

- [ ] **Step 1: тест** — `like_test.go`: `TestMatchPeer` (Peer(A)=B, Peer(B)=A, Peer(чужой)=uuid.Nil), таблица на нормализацию пары не нужна (пары строит репозиторий).

```go
func TestMatchPeer(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	if a.String() > b.String() {
		a, b = b, a
	}
	m := Match{AID: a, BID: b}
	if m.Peer(a) != b || m.Peer(b) != a {
		t.Errorf("Peer() mixed up the pair")
	}
	if m.Peer(uuid.New()) != uuid.Nil {
		t.Errorf("Peer() of a stranger must be Nil")
	}
}
```

- [ ] **Step 2: red** — `go test ./internal/entity/ -run TestMatchPeer` падает.
- [ ] **Step 3: реализация** по сигнатурам выше; `Peer`: `switch me { case m.AID: return m.BID; case m.BID: return m.AID; default: return uuid.Nil }`.
- [ ] **Step 4: green + Verify.**
- [ ] **Step 5: commit** «Лайки: доменные типы».

---

### Task 2: Миграция 0003 + транзакция лайка в likesrepo

Сердце шага. Спека §1–2 дословно.

**Files:**
- Create: `api/migrations/0003_likes.sql`
- Create: `api/internal/repo/likesrepo/repo.go`, `model.go`, `postgres.go`
- Test: `api/internal/repo/likesrepo/repo_test.go`

**Interfaces:**
- Consumes: `pgtest.NewPool`, `postgres.InTx`, `visible_people` (миграция 0002), entity из Task 1.
- Produces:

```go
func New(pool *pgxpool.Pool) *Repo
// Create runs the whole §2.3 mechanics in one transaction.
func (r *Repo) Create(ctx context.Context, fromID, toID uuid.UUID, zoneID string) (entity.LikeResult, error)
func (r *Repo) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
```

- [ ] **Step 1: миграция** — SQL из спеки §1 дословно (goose-формат по образцу 0002; likes_users, matches, три индекса, CHECK'и). Down: drop в обратном порядке.

- [ ] **Step 2: red-тесты** — `repo_test.go` с хелперами по образцу `searchesrepo` (`cleanTestUsers` с диапазоном −996000000, чистить matches → likes_users → searches → users; `seedUser`; `mustSearch` — активный поиск через searchesrepo либо прямой INSERT). Обязательные сценарии:

```go
// Все — против live Postgres; образцы утверждений давать полностью, стиль
// searchesrepo/repo_test.go.

TestLikeSpendsBudget            // первый лайк: Spent=true, Budget.Remaining=4
TestLikeRequiresActiveSearch    // без активного поиска → entity.ErrNeedSearch
TestReplyIsFreeAndUnconditional // B без поиска отвечает на входящий: Spent=false,
                                // лимит не тронут, Match != nil, оба ID в паре, A<B
TestMatchContextIsFirstLike     // контекст мэтча = зона ПЕРВОГО лайка пары
TestRepeatedLikeIsIdempotent    // второй Create той же пары: Already=true,
                                // бюджет не тронут, мэтча не появилось
TestLikeInvisiblePersonIs404    // same-pref/блок → entity.ErrNotFound, строки нет
TestBudgetWindowSlides          // 5 spent-лайков, один UPDATE created_at на -25h →
                                // Remaining=1, NextFreeAt указывает на второй по старшинству
TestLikeLimitUnderConcurrency   // у юзера 0 потрачено; 7 горутин на close(start)-барьере
                                // с прогретым пулом лайкают 7 РАЗНЫХ видимых людей →
                                // ровно 5 ok + 2 entity.ErrLikeLimit, в БД 5 spent
TestMutualRaceMakesOneMatch     // A→B и B→A одновременно (барьер): в matches ровно
                                // одна строка, ОБА результата несут Match != nil
```

- [ ] **Step 3: реализация `postgres.go`.** Скелет транзакции (комментарии — только «почему», по правилу владельца):

```go
func (r *Repo) Create(ctx context.Context, fromID, toID uuid.UUID, zoneID string) (entity.LikeResult, error) {
	var out entity.LikeResult
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		// Lock the parent row: locking like rows would lock nothing for a
		// user's first like, and the budget count below needs a fresh
		// statement snapshot after the wait (READ COMMITTED, как в searchesrepo).
		if err := lockUser(ctx, tx, fromID); err != nil { return err }

		visible, err := isVisible(ctx, tx, fromID, toID) // JOIN visible_people($1)
		if err != nil { return err }
		if !visible { return entity.ErrNotFound }

		reply, err := exists(ctx, tx, `SELECT 1 FROM likes_users WHERE from_id=$1 AND to_id=$2`, toID, fromID)
		if err != nil { return err }

		spent := !reply
		if !reply {
			hasSearch, err := exists(ctx, tx, `SELECT 1 FROM searches WHERE user_id=$1 AND status='active'`, fromID)
			if err != nil { return err }
			if !hasSearch { return entity.ErrNeedSearch }
			n, err := spentInWindow(ctx, tx, fromID)
			if err != nil { return err }
			if n >= entity.MaxDailyLikes { return entity.ErrLikeLimit }
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO likes_users (from_id, to_id, zone_id, point_id, spent)
			VALUES ($1, $2, NULLIF($3, ''), NULL, $4)
			ON CONFLICT (from_id, to_id) DO NOTHING`, fromID, toID, zoneID, spent)
		if err != nil { return err }
		if tag.RowsAffected() == 0 {
			out.Already = true
			// Лайк уже был: мэтч (если есть) дочитываем ниже, бюджет не трогаем.
		}

		if reply || out.Already {
			if err := ensureMatch(ctx, tx, fromID, toID, &out); err != nil { return err }
		}
		out.Like, err = readLike(ctx, tx, fromID, toID)
		return err
	})
	if err != nil { return entity.LikeResult{}, fmt.Errorf("like %s -> %s: %w", fromID, toID, err) }
	budget, err := r.Budget(ctx, fromID)
	if err != nil { return entity.LikeResult{}, err }
	out.Budget = budget
	return out, nil
}
```

`ensureMatch`: `INSERT INTO matches (a_id, b_id, context_zone_id, context_point_id) SELECT least($1,$2), greatest($1,$2), l.zone_id, l.point_id FROM likes_users l WHERE (l.from_id, l.to_id) IN (($1,$2),($2,$1)) ORDER BY l.created_at LIMIT 1 ON CONFLICT (a_id, b_id) DO NOTHING`, затем `SELECT` мэтча пары в `out.Match`. uuid сравнивается в Postgres лексикографически — `least/greatest` дают ту же каноничность, что CHECK `a_id < b_id`.

`spentInWindow`: `SELECT count(*) FROM likes_users WHERE from_id=$1 AND spent AND created_at > now() - interval '24 hours'`.

`Budget`: тот же count вне транзакции + `NextFreeAt`: `SELECT min(created_at) + interval '24 hours' FROM (SELECT created_at FROM likes_users WHERE from_id=$1 AND spent AND created_at > now() - interval '24 hours' ORDER BY created_at LIMIT 1) t` — только когда Remaining=0? Нет: NextFreeAt считается всегда при Remaining < MaxDailyLikes (старейший spent в окне + 24ч), при полном остатке — nil.

- [ ] **Step 4: green** — все 9 тестов; гонки прогнать `-count=3`.
- [ ] **Step 5: Verify + commit** «Лайки: миграция и транзакция с бюджетом».

---

### Task 3: Чтения — входящие, пара, профиль; matchesrepo

**Files:**
- Modify: `api/internal/repo/likesrepo/postgres.go`, `model.go`
- Create: `api/internal/repo/matchesrepo/repo.go`, `model.go`, `postgres.go`
- Modify: `api/internal/repo/usersrepo/postgres.go` (+`VisibleByID`)
- Test: `api/internal/repo/likesrepo/incoming_test.go`, `api/internal/repo/matchesrepo/repo_test.go`

**Interfaces — Produces:**

```go
// likesrepo
type IncomingLike struct {
	From         entity.User // частичный, как entity.PoolCandidate.User
	ZoneID       string
	NotSearching bool // нет активных поисков — серая пометка §2.3
	CreatedAt    time.Time
}
// Неотвеченные (нет мэтча пары), лайкер виден адресату, свежие сверху.
func (r *Repo) Incoming(ctx context.Context, userID uuid.UUID) ([]IncomingLike, error)
// (aLikedB, bLikedA) — для liked_me / my_like на профиле.
func (r *Repo) Between(ctx context.Context, a, b uuid.UUID) (bool, bool, error)

// matchesrepo
type MatchWithPeer struct {
	Match entity.Match
	Peer  entity.User // ВНИМАНИЕ: Username/TelegramID заполняются ТОЛЬКО в ByID
}
func New(pool *pgxpool.Pool) *Repo
func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]MatchWithPeer, error)
func (r *Repo) ByID(ctx context.Context, userID, id uuid.UUID) (MatchWithPeer, error) // чужой мэтч → ErrNotFound

// usersrepo
func (r *Repo) VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error)
```

- [ ] **Step 1: red-тесты.** likesrepo: `TestIncomingHidesAnsweredAndInvisible` (мэтчнутый лайк исчез из списка; same-pref лайкер не виден адресату), `TestIncomingNotSearchingFlag` (лайкер закрыл все поиски → true). matchesrepo: `TestListAndByID` (список обеих сторон; ByID чужого → ErrNotFound; `Peer.Username` ПУСТ в List и заполнен в ByID — приватность закреплена тестом). usersrepo: `TestVisibleByID` (невидимый → ErrNotFound).
- [ ] **Step 2: реализация.** Incoming: `SELECT l.*, u_*: FROM likes_users l JOIN visible_people($1) vp ON vp = l.from_id JOIN users u ON u.id = l.from_id WHERE l.to_id = $1 AND NOT EXISTS (SELECT 1 FROM matches m WHERE m.a_id = least(l.from_id, l.to_id) AND m.b_id = greatest(l.from_id, l.to_id)) ORDER BY l.created_at DESC` + `NOT EXISTS (searches active)` как `not_searching`. Колонки юзера — по образцу `candidateColumns` (БЕЗ username/tg_id). matchesrepo.List: JOIN users по peer-стороне, те же безопасные колонки; ByID — расширенный список колонок с `tg_username`, `tg_id`. VisibleByID: `SELECT <columns> FROM users u JOIN visible_people($1) vp ON vp = u.id WHERE u.id = $2`.
- [ ] **Step 3: green + Verify + commit** «Лайки: входящие и мэтчи — чтения».

---

### Task 4: Сервисы likes и matches

**Files:**
- Create: `api/internal/service/likes/service.go` (заменить doc.go-строку «Empty on purpose»)
- Create: `api/internal/service/matches/service.go` (то же)
- Test: `api/internal/service/likes/service_test.go`, `api/internal/service/matches/summary_test.go`

**Interfaces — Produces (Task 5 зовёт):**

```go
// likes
type Service struct { repo Repository; dir *zones.Directory }
func New(repo Repository, dir *zones.Directory) *Service
func (s *Service) Like(ctx context.Context, fromID, toID uuid.UUID, zoneID string) (entity.LikeResult, error)
func (s *Service) Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
func (s *Service) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
// Repository interface: Create, Incoming, Budget (consumer-side).

// matches
type Summary struct {
	Zone          *zones.Zone            // контекст, если зонный
	ParamsOverlap []ParamOverlap         // «бюджет: у обоих 40–45к»
	SharedAnswers []entity.SharedAnswer  // «вы оба совы»
}
type ParamOverlap struct {
	Param    string // price | rooms | move_in | term
	Min, Max int    // для вилок
	Month    string // для move_in, «2026-09»
	Term     string
}
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]matchesrepo.MatchWithPeer, error)
func (s *Service) Detail(ctx context.Context, userID, id uuid.UUID) (matchesrepo.MatchWithPeer, Summary, error)
```

Плюс в entity/profile.go: `type SharedAnswer struct{ Field, Value string }` и `func (p Profile) Shared(o Profile) []SharedAnswer` — структурная версия SharedAnswers (тот же список из 9 полей; SharedAnswers переписать как `len(Shared())`).

- [ ] **Step 1: red.** likes: валидация зоны (непустая и неизвестная директории → FieldsError zone_id; пустая — легальна, ответный без контекста); проброс ошибок репо. matches summary: юнит на пересечение параметров — у пары по свежайшему активному поиску каждой стороны, пересечение вилок только где обе определены и пересекаются; нет поиска у стороны → ParamsOverlap пуст; Shared: 2 совпавших поля → ровно они.
- [ ] **Step 2: реализация.** Summary.ParamsOverlap: взять свежайший активный поиск каждого через searchesrepo.ListByUser (интерфейс у matches-сервиса), пересечения: price всегда (max(minA,minB)..min(maxA,maxB) если пересеклись), rooms аналогично при обоих заданных, move_in при равенстве месяцев, term при равенстве. Это ЖИВЫЕ данные (спека: историчен только контекст-зона).
- [ ] **Step 3: green + Verify + commit** «Лайки и мэтчи: сервисы и саммари».

---

### Task 5: HTTP — peopleapi, likesapi, matchesapi + wiring

**Files:**
- Create: `api/internal/api/http/peopleapi/handler.go`, `dto.go`
- Create: `api/internal/api/http/likesapi/handler.go`, `dto.go`, `errors.go`
- Create: `api/internal/api/http/matchesapi/handler.go`, `dto.go`
- Modify: `api/internal/httpx/respond.go` (+`CodeLikeLimit = "like_limit"`, `CodeNeedSearch = "need_search"`), `api/cmd/api/main.go`

**Interfaces — Produces (фронт-задачи потребляют дословно):**

```
GET /api/v1/people/{id}?zone={zoneID} →
{ user_id, name, age, photo_url, occupation, schedule, smoking, pets_own,
  pets_ok, noise, guests, cleaning, alcohol, expenses,        // строки-енумы
  about?,                                                     // только about_status=ok
  shared: [{field, value}], context_zone?: {id,name,name_in,name_gen},
  liked_me: bool, my_like: bool,
  budget: {remaining, next_free_at?} }

POST /api/v1/likes {to_id, zone_id?} →
{ matched: bool, match_id?: uuid, already: bool, budget: {remaining, next_free_at?} }
400 need_search | 400 like_limit — message конверта уже человеческий:
«лимит на сегодня: следующий лайк в HH:MM» (next_free_at сервер знает при
формировании ошибки; отдельного поля в 400-конверте не заводим)
404 not_found (невидим/нет)

GET /api/v1/likes/incoming →
[{ user_id, name, age, photo_url, schedule, smoking, pets_own,
   zone?: {…}, not_searching, at }]

GET /api/v1/matches → [{ match_id, peer: {user_id, name, age, photo_url}, zone?: {…}, at }]
GET /api/v1/matches/{id} →
{ match_id, peer: {user_id, name, age, photo_url, username?, tg_id}, at,
  where?: {…зона…}, params: [{param, min?, max?, month?, term?}],
  profile: [{field, value}] }
```

Маршруты в `main.go`: `r.Get("/people/{id}", people.Profile)`, `r.Mount("/likes", likesapi…)` (`POST /`, `GET /incoming`), `r.Mount("/matches", matchesapi…)`. Профиль-хендлер собирает: `usersrepo.VisibleByID` + `likesrepo.Between` + `likes.Budget` + `Shared(viewer.Profile)` + зона из Directory.

- [ ] **Step 1: реализация** по образцам searchesapi/poolsapi (DisallowUnknownFields, MaxBytesReader, handleError-switch: ErrNeedSearch→need_search, ErrLikeLimit→like_limit, ErrNotFound→404, FieldsError→invalid_field).
- [ ] **Step 2: curl-проходка** (стенд: postgres up, `go run ./cmd/api`, AUTH от devinitdata): профиль сида из пула (200, about виден, budget 5), лайк → 201?—нет, 200 `{matched:false,…budget.remaining:4}`, повтор → `already:true`, remaining не упал; шестой spent-лайк разным людям → like_limit; профиль невидимой Ани → 404; собрать вывод в отчёт.
- [ ] **Step 3: Verify + commit** «HTTP: профиль, лайки, мэтчи».

---

### Task 6: devseed — входящие лайки и заготовка мэтча

**Files:**
- Modify: `api/cmd/devseed/main.go`, README (раздел сидера — абзац про лайки)

- [ ] **Step 1:** после вставки поисков — идемпотентный блок лайков: Катя (900000001) и Ира (900000003) лайкают dev-юзера `tg_id=777000001` (`spent=true, zone_id='d-khamovniki'`, `ON CONFLICT DO NOTHING`); если dev-юзера нет в базе — блок молча пропускается (сидер не создаёт его). НЕ создавать мэтч сидом: ответ и мэтч — ручная часть приёмки. Wipe-инструкция в README дополняется likes_users/matches (порядок: matches → likes_users → searches → users).
- [ ] **Step 2:** прогнать дважды (идемпотентность), psql-проверка count, `GET /likes/incoming` за dev-юзера → 2 входящих с зоной и `not_searching:false`.
- [ ] **Step 3: Verify + commit** «devseed: входящие лайки».

---

### Task 7: Фронт A — клиент, профиль из пула, лайк-флоу

**Files:**
- Modify: `web/src/api/client.ts` (типы + `fetchPerson`, `postLike`, `fetchIncoming`, `fetchMatches`, `fetchMatch`), `web/src/api/format.ts` (лейблы 9 полей анкеты для shared/профиля — переиспользовать существующие карты; добавить `fieldLabel(field, value)`)
- Modify: `web/src/screens/PersonProfileScreen.tsx` (пропсы под живые данные; кнопка: остаток «X из 5», тон «Ответить — мэтч сразу» при liked_me, disabled при remaining=0 с «следующий в HH:MM»)
- Create: `web/src/screens/PersonProfileView.tsx` (контейнер: fetch по user_id+zone, маппинг DTO→пропсы, POST лайка, обработка need_search → открыть форму поиска с возвратом, like_limit → подпись)
- Modify: `web/src/App.tsx` (тап по карточке пула → PersonProfileView с реальным user_id; `matched:true` → view «match»)
- MainButton: в PersonProfileView через `mainButton` из @telegram-apps/sdk-react — mount/setParams/onClick на входе, hide+off на unmount (в браузере SDK недоступен → фолбэк-кнопка вёрстки; паттерн back-button из SearchParamsScreen)

- [ ] **Step 1:** типы клиента зеркалят DTO Task 5 (snake_case, опциональность как в Go).
- [ ] **Step 2:** проводка экрана; `shared` рисуется зелёными чипами «вы оба {label}»; «о себе» — блок только при наличии.
- [ ] **Step 3:** Verify фронта + commit «Фронт: живой профиль и дорогой лайк».

---

### Task 8: Фронт B — «Тебя лайкнули», «Это мэтч», «Мои мэтчи», бейдж

**Files:**
- Modify: `web/src/screens/IncomingLikesScreen.tsx` (живой список: контекст-зона тапабельна → пул; «Ответить» → postLike; «сейчас не в поиске» серым при not_searching), `web/src/screens/MatchScreen.tsx` (саммари из `/matches/{id}`: where/params/profile через format.ts; кнопка: username → `https://t.me/{username}`, иначе `tg://user?id={tg_id}` + подсказка «если не открылось — у него скрыт профиль, он напишет тебе первым»), `web/src/screens/MyProfileScreen.tsx` (живые «Мои мэтчи»), `web/src/App.tsx` (бейдж = длина incoming; state и refetch: incoming тянется на старте и после каждого postLike; DEMO_INCOMING/DEMO_MATCHES/DEMO_MY_MATCHES и match-DEMO удалить)
- [ ] **Step 1:** проводка всех трёх + удаление DEMO этого шага (points-DEMO не трогать).
- [ ] **Step 2:** Verify фронта + commit «Фронт: тебя лайкнули и мэтчи».

---

### Task 9: Приёмка и закрытие шага

**Files:**
- Modify: `CLAUDE.md` (§2.3 скользящее окно; §11 likes_users без status, like_budget удалить с заменой на окно; §12 шаг 4 — сделано, текущая — шаг 5; §13 — заглушка шага 5 по §12), README при необходимости.

- [ ] **Step 1: браузерная приёмка** (стенд + Chrome MCP; если расширение не отвечает дважды — curl-фолбэк с пометкой в отчёте). Сценарий: dev-юзер: бейдж «2» на Профиле → «Тебя лайкнули»: Катя и Ира с зоной; профиль Кати из списка → «Ответить — мэтч сразу» → «Это мэтч» с саммари и контактом; «Мои мэтчи» = 1; профиль Олега из пула → лайк «X из 5» уменьшился; поставить 5-й лайк → кнопка гаснет «следующий в HH:MM»; повторный тап по лайкнутому — «уже лайкнуто», бюджет цел. Скриншоты ключевых состояний.
- [ ] **Step 2: правки спеки** из списка выше.
- [ ] **Step 3: полный Verify обоих стеков + commit** «Спека: шаг 4 закрыт» (правки спеки отдельным коммитом от приёмочных фиксов, если они будут).
