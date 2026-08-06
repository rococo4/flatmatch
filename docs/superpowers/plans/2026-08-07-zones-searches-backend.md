# Зоны и поиски (бэкенд) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Бэкенд §12-шага 3: справочник зон Москвы в памяти, таблица `searches` с лимитом «≤3 активных», SQL-функция `visible_people`, вычисление зонного пула (секции + вердикт-чипы), HTTP API для ленты/поисков/пула, сидер фейковых людей.

**Architecture:** Спека дизайна — `docs/superpowers/specs/2026-08-07-zones-searches-design.md` (читать перед началом). Справочник зон — JSON в репе → `embed` → пакет `zones` в памяти, таблицы зон в БД нет; SQL получает готовые списки `zone_id`. Видимость — SQL-функция `visible_people(viewer)` в миграции, через неё все выборки людей. Секции и чипы пула — чистые функции в Go.

**Tech Stack:** Go 1.26, chi, pgx v5 (`pgxpool`, `RowToStructByName`, `CollectExactlyOneRow`), goose-миграции из `embed.FS`, slog. Без новых зависимостей.

## Global Constraints

- **Коммит на задачу, прямо в main** (решение пользователя 2026-08-07, оно переопределяет строки «Не коммитить» в шагах задач — те писались раньше): после зелёного шага Verify сделать `git add` затронутых файлов и один коммит с короткой русской строкой в стиле истории репо («Поиски: …», «Зоны: …»). Веток не создавать. Push не делать.
- Идентификаторы, комментарии, логи — английский; пользовательские строки — русский, дословно из CLAUDE.md.
- Слои `handler → service → storage`; интерфейсы объявляет потребитель (§10).
- Порядок объявлений в Go-файлах: константы и типы наверху, функции ниже, публичные раньше приватных.
- Модуль — `flatmatch/api`. Хендлеры — `api/internal/api/http/<name>api`, сервисы — `api/internal/service/<name>`, репозитории — `api/internal/repo/<name>repo`.
- DB-тесты: `TEST_DATABASE_URL` (см. README: `postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable`), без переменной — `t.Skip`. Postgres поднимается `docker compose up -d postgres`.
- Ошибки HTTP — конверт `httpx` (`{error: {code, message, fields?}}`), новые коды добавлять в `api/internal/httpx/respond.go`.
- Правило «зоны поисков одного юзера не пересекаются» ОТМЕНЕНО (спека обновлена 2026-08-07): единственный лимит — 3 активных поиска.

---

### Task 1: Типы поиска в entity

**Files:**
- Create: `api/internal/entity/search.go`
- Test: `api/internal/entity/search_test.go`

**Interfaces:**
- Consumes: `parseEnum[T ~string]` и константу `unknown` из `api/internal/entity/profile.go`, `FieldsError` из `errors.go`.
- Produces: `entity.Search`, `entity.SearchParams` (+ `InvalidFields()`), `entity.Term`, `entity.SearchStatus`, `entity.CloseReason`, `entity.StuffChip`, `entity.MaxActiveSearches`, парсеры `ParseTerm/ParseSearchStatus/ParseCloseReason/ParseStuffChip`. Ошибки `entity.ErrSearchLimit`, `entity.ErrSearchClosed` в `errors.go`.

- [ ] **Step 1: Написать падающий тест**

```go
// api/internal/entity/search_test.go
package entity

import (
	"slices"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
)

func validParams() SearchParams {
	move := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return SearchParams{
		ZoneID:   "d-khamovniki",
		PriceMin: pointer.To(35000),
		PriceMax: 45000,
		RoomsMin: pointer.To(2),
		RoomsMax: pointer.To(3),
		MoveIn:   &move,
		Term:     pointer.To(TermYear),
		HasStuff: []StuffChip{StuffFurniture},
	}
}

func TestSearchParamsValidPassesValidation(t *testing.T) {
	if bad := validParams().InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsMinimalIsValid(t *testing.T) {
	// «до» обязательно, всё остальное «неважно» (§2.1).
	p := SearchParams{ZoneID: "line-1", PriceMax: 40000}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsInvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		break_ func(*SearchParams)
		want  string
	}{
		{"no zone", func(p *SearchParams) { p.ZoneID = "" }, "zone_id"},
		{"no price_max", func(p *SearchParams) { p.PriceMax = 0 }, "price_max"},
		{"price_max above cap", func(p *SearchParams) { p.PriceMax = 2_000_000 }, "price_max"},
		{"min above max", func(p *SearchParams) { *p.PriceMin = 50000 }, "price_min"},
		{"negative min", func(p *SearchParams) { *p.PriceMin = -1 }, "price_min"},
		{"rooms inverted", func(p *SearchParams) { *p.RoomsMin = 4 }, "rooms_min"},
		{"rooms zero", func(p *SearchParams) { *p.RoomsMax = 0 }, "rooms_max"},
		{"unknown term", func(p *SearchParams) { *p.Term = TermUnknown }, "term"},
		{"unknown chip", func(p *SearchParams) { p.HasStuff = []StuffChip{"jacuzzi"} }, "has_stuff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validParams()
			tt.break_(&p)
			if bad := p.InvalidFields(); !slices.Contains(bad, tt.want) {
				t.Errorf("InvalidFields() = %v, want to contain %q", bad, tt.want)
			}
		})
	}
}

func TestParseSearchEnums(t *testing.T) {
	if got := ParseTerm("year"); got != TermYear {
		t.Errorf("ParseTerm(year) = %q", got)
	}
	if got := ParseTerm("decade"); got != TermUnknown {
		t.Errorf("ParseTerm(decade) = %q, want unknown", got)
	}
	if got := ParseTerm(""); got != "" {
		t.Errorf("ParseTerm(\"\") = %q, want empty", got)
	}
	if got := ParseCloseReason("found_here"); got != CloseFoundHere {
		t.Errorf("ParseCloseReason(found_here) = %q", got)
	}
	if got := ParseSearchStatus("paused"); got != SearchPaused {
		t.Errorf("ParseSearchStatus(paused) = %q", got)
	}
}
```

- [ ] **Step 2: Запустить, убедиться что падает**

Run: `cd api && go test ./internal/entity/ -run 'TestSearch|TestParseSearch' -v`
Expected: FAIL — `undefined: SearchParams` и т.д.

- [ ] **Step 3: Реализация**

```go
// api/internal/entity/search.go
package entity

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// MaxActiveSearches is the only limit on searches (§2.1): zones may overlap
// or even coincide, so nothing else is validated across a user's searches.
const MaxActiveSearches = 3

// PriceMaxCap is a sanity bound for «с человека», not a market opinion.
const PriceMaxCap = 1_000_000

type Term string

const (
	TermUnknown Term = unknown

	TermHalfYear Term = "half_year"
	TermYear     Term = "year"
	TermOpen     Term = "open"
)

type SearchStatus string

const (
	SearchStatusUnknown SearchStatus = unknown

	SearchActive SearchStatus = "active"
	SearchPaused SearchStatus = "paused"
	SearchClosed SearchStatus = "closed"
)

type CloseReason string

const (
	CloseReasonUnknown CloseReason = unknown

	CloseFoundHere      CloseReason = "found_here" // ГЛАВНАЯ МЕТРИКА (§3)
	CloseFoundElsewhere CloseReason = "found_elsewhere"
	CloseChangedMind    CloseReason = "changed_mind"
)

type StuffChip string

const (
	StuffChipUnknown StuffChip = unknown

	StuffFurniture  StuffChip = "furniture"
	StuffAppliances StuffChip = "appliances"
)

// SearchParams is what the user actually picks — shared by create, edit and
// the feed's look-without-saving query (§6: поиск — снапшот фильтров ленты).
// nil / empty means «неважно» and intersects everything (§2.1).
type SearchParams struct {
	ZoneID   string
	PriceMin *int
	PriceMax int
	RoomsMin *int
	RoomsMax *int
	MoveIn   *time.Time // first day of the move-in month
	Term     *Term
	HasStuff []StuffChip
}

type Search struct {
	ID     uuid.UUID
	UserID uuid.UUID
	SearchParams
	Status      SearchStatus
	CloseReason CloseReason // "" unless Status is closed
	PoolSeenAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func ParseTerm(raw string) Term {
	return parseEnum(raw, TermHalfYear, TermYear, TermOpen)
}

func ParseSearchStatus(raw string) SearchStatus {
	return parseEnum(raw, SearchActive, SearchPaused, SearchClosed)
}

func ParseCloseReason(raw string) CloseReason {
	return parseEnum(raw, CloseFoundHere, CloseFoundElsewhere, CloseChangedMind)
}

func ParseStuffChip(raw string) StuffChip {
	return parseEnum(raw, StuffFurniture, StuffAppliances)
}

func (p SearchParams) InvalidFields() []string {
	checks := []struct {
		field string
		bad   bool
	}{
		{"zone_id", p.ZoneID == ""},
		{"price_max", p.PriceMax <= 0 || p.PriceMax > PriceMaxCap},
		{"price_min", p.PriceMin != nil && (*p.PriceMin < 0 || *p.PriceMin > p.PriceMax)},
		{"rooms_min", p.RoomsMin != nil &&
			(*p.RoomsMin < 1 || (p.RoomsMax != nil && *p.RoomsMin > *p.RoomsMax))},
		{"rooms_max", p.RoomsMax != nil && *p.RoomsMax < 1},
		{"term", p.Term != nil && *p.Term == TermUnknown},
		{"has_stuff", slices.ContainsFunc(p.HasStuff, func(c StuffChip) bool {
			return ParseStuffChip(string(c)) != c || c == ""
		})},
	}

	var bad []string
	for _, c := range checks {
		if c.bad {
			bad = append(bad, c.field)
		}
	}
	return bad
}
```

В `api/internal/entity/errors.go` добавить:

```go
// ErrSearchLimit is answered when a fourth active search would appear (§2.1).
var ErrSearchLimit = errors.New("active search limit reached")

// ErrSearchClosed is answered when editing or re-closing a closed search;
// the way back is reactivation (§3).
var ErrSearchClosed = errors.New("search is closed")
```

- [ ] **Step 4: Запустить тесты**

Run: `cd api && go test ./internal/entity/ -v`
Expected: PASS (все, включая старые).

- [ ] **Step 5: Verify**

Run: `cd api && go build ./... && go vet ./...`
Expected: чисто. Не коммитить.

---

### Task 2: Миграция 0002 + пакет pgtest + тесты visible_people

**Files:**
- Create: `api/migrations/0002_searches.sql`
- Create: `api/internal/postgres/pgtest/pgtest.go` — общий тест-хелпер (третья копия `newTestPool` — уже перебор; существующий в `usersrepo` не трогать)
- Test: `api/internal/repo/searchesrepo/visibility_test.go`

**Interfaces:**
- Produces: таблицы `searches`, `blocks`; SQL-функция `visible_people(viewer uuid) RETURNS SETOF uuid`; `pgtest.NewPool(t *testing.T) *pgxpool.Pool`.

- [ ] **Step 1: Миграция**

```sql
-- api/migrations/0002_searches.sql
-- +goose Up

-- A search is a subscription to zone + parameters (§2.1). zone_id is a slug
-- from the in-memory directory (api/internal/service/zones) — deliberately no
-- FK: the directory lives in the repo, zones are never deleted.
CREATE TABLE searches (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id),
    zone_id      text NOT NULL,
    price_min    int NULL,
    price_max    int NOT NULL,
    rooms_min    int NULL,
    rooms_max    int NULL,
    move_in      date NULL, -- first day of the move-in month
    term         text NULL CHECK (term IN ('half_year', 'year', 'open')),
    has_stuff    text[] NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'paused', 'closed')),
    close_reason text NULL
                 CHECK (close_reason IN ('found_here', 'found_elsewhere', 'changed_mind')),
    -- Last time the owner opened this zone's pool; the «M новых» badge counts
    -- people whose search appeared after this moment.
    pool_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'closed') = (close_reason IS NOT NULL)),
    CHECK (price_min IS NULL OR price_min <= price_max),
    CHECK (rooms_min IS NULL OR rooms_max IS NULL OR rooms_min <= rooms_max)
);

CREATE INDEX searches_user_idx ON searches (user_id);
-- The pool query: active searches of intersecting zones.
CREATE INDEX searches_active_zone_idx ON searches (zone_id) WHERE status = 'active';

-- One row = one press of «заблокировать». Directional record, mutual effect:
-- visible_people checks both directions (§5). Created empty now so the
-- visibility rule never needs a rewrite when the button ships (§12 step 8).
CREATE TABLE blocks (
    blocker_id uuid NOT NULL REFERENCES users (id),
    blocked_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

-- +goose StatementBegin
-- THE single visibility point (§4): who exists for whom. Every consumer —
-- zone pools, counters, later flat pools, pushes and like validation — goes
-- through this function. LANGUAGE sql STABLE lets the planner inline it.
-- The viewer's own status is deliberately not checked: draft and pending may
-- browse (§5); the rule is about who can be SEEN.
CREATE FUNCTION visible_people(viewer uuid) RETURNS SETOF uuid
LANGUAGE sql STABLE AS $$
    SELECT c.id
    FROM users c, users v
    WHERE v.id = viewer
      AND c.id <> v.id
      AND c.status = 'active'
      AND (v.gender_pref = 'any' OR c.gender = v.gender)
      AND (c.gender_pref = 'any' OR v.gender = c.gender)
      AND NOT EXISTS (
          SELECT 1 FROM blocks b
          WHERE (b.blocker_id = v.id AND b.blocked_id = c.id)
             OR (b.blocker_id = c.id AND b.blocked_id = v.id)
      )
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION visible_people(uuid);
DROP TABLE blocks;
DROP TABLE searches;
```

- [ ] **Step 2: Пакет pgtest**

```go
// api/internal/postgres/pgtest/pgtest.go
// Package pgtest connects DB tests to TEST_DATABASE_URL. Skipping without the
// variable keeps `go test ./...` runnable with nothing but a Go toolchain.
package pgtest

import (
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/postgres"
	"flatmatch/api/migrations"
)

func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set — see README, «Тесты, которым нужна база»")
	}

	ctx := t.Context()
	if err := postgres.Migrate(ctx, dsn, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
```

- [ ] **Step 3: Тест на visible_people (падает, пока миграция не применится — потом зелёный)**

```go
// api/internal/repo/searchesrepo/visibility_test.go
package searchesrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/postgres/pgtest"
)

// Test users live in their own negative tg_id range so leftovers never
// collide with real accounts or with usersrepo's tests.
const testTGBase int64 = -998000000

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// pgx uses the extended protocol: one statement per Exec, no semicolons.
	clean := func() {
		for _, q := range []string{
			`DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM blocks WHERE blocker_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up test users: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

// seedUser inserts a user directly: repo methods can't set status/gender to
// arbitrary combinations, and this test is about the SQL function itself.
func seedUser(t *testing.T, pool *pgxpool.Pool, n int64, status, gender, pref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', $2, $3, $4) RETURNING id`,
		testTGBase-n, status, gender, pref).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func visibleTo(t *testing.T, pool *pgxpool.Pool, viewer uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT visible_people($1)`, viewer)
	if err != nil {
		t.Fatalf("visible_people: %v", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	set := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func TestVisiblePeopleGenderIsMutual(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	anyM := seedUser(t, pool, 1, "active", "male", "any")
	sameM := seedUser(t, pool, 2, "active", "male", "same")
	anyF := seedUser(t, pool, 3, "active", "female", "any")
	sameF := seedUser(t, pool, 4, "active", "female", "same")

	tests := []struct {
		name    string
		viewer  uuid.UUID
		wantYes []uuid.UUID
		wantNo  []uuid.UUID
	}{
		// same-M видит any-M (оба согласны), не видит женщин: он их отсёк.
		{"same male", sameM, []uuid.UUID{anyM}, []uuid.UUID{anyF, sameF}},
		// any-F готова на всех, но same-M отсёк её: правило обоюдно (§4).
		{"any female", anyF, []uuid.UUID{anyM, sameF}, []uuid.UUID{sameM}},
		// any-M не видит same-F по той же причине, себя — никогда.
		{"any male", anyM, []uuid.UUID{anyF, sameM}, []uuid.UUID{sameF, anyM}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis := visibleTo(t, pool, tt.viewer)
			for _, id := range tt.wantYes {
				if !vis[id] {
					t.Errorf("user %s must be visible", id)
				}
			}
			for _, id := range tt.wantNo {
				if vis[id] {
					t.Errorf("user %s must be invisible", id)
				}
			}
		})
	}
}

func TestVisiblePeopleOnlyActive(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	viewer := seedUser(t, pool, 1, "draft", "male", "any") // draft может смотреть (§5)
	for i, status := range []string{"draft", "pending", "banned"} {
		hidden := seedUser(t, pool, int64(10+i), status, "female", "any")
		if visibleTo(t, pool, viewer)[hidden] {
			t.Errorf("%s user must be invisible", status)
		}
	}
	active := seedUser(t, pool, 20, "active", "female", "any")
	if !visibleTo(t, pool, viewer)[active] {
		t.Error("active user must be visible")
	}
}

func TestVisiblePeopleBlockWorksBothWays(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	a := seedUser(t, pool, 1, "active", "male", "any")
	b := seedUser(t, pool, 2, "active", "female", "any")

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, a, b); err != nil {
		t.Fatalf("insert block: %v", err)
	}

	if visibleTo(t, pool, a)[b] {
		t.Error("blocker must not see the blocked")
	}
	if visibleTo(t, pool, b)[a] {
		t.Error("blocked must not see the blocker either")
	}
}
```

Пакету нужен пустой `api/internal/repo/searchesrepo/repo.go`, чтобы тест компилировался:

```go
// api/internal/repo/searchesrepo/repo.go
// Package searchesrepo stores searches and answers pool queries (§2.1).
package searchesrepo

import "github.com/jackc/pgx/v5/pgxpool"

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}
```

- [ ] **Step 4: Прогнать**

Run: `cd api && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./internal/repo/searchesrepo/ -v`
Expected: PASS все три (goose сам применит 0002 через pgtest.NewPool). Если Postgres не поднят: `docker compose up -d postgres`.

- [ ] **Step 5: Verify**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL=... go test ./...`
Expected: чисто, старые тесты живы. Не коммитить.

---

### Task 3: Пакет zones — ядро на тестовой фикстуре

**Files:**
- Create: `api/internal/service/zones/zones.go`
- Create: `api/internal/service/zones/testdata/mini.json`
- Test: `api/internal/service/zones/zones_test.go`
- Modify: `api/internal/service/zones/doc.go` — убрать строку «Empty on purpose»

**Interfaces:**
- Produces: `zones.Directory` с методами `Get(id) (Zone, bool)`, `Intersecting(id) []string`, `All() []Zone`, `Version() int`, `Raw() []byte`; `zones.New(raw []byte) (*Directory, error)` (для тестов и фикстур), `zones.Load() (*Directory, error)` (embed, появится в Task 4 — здесь `Load` уже написан, но `moscow.json` минимальный временный); тип `zones.Zone{ID, Kind, Name, NameIn, NameGen string; Stations []string}`, `zones.Kind` (`district/station/line`).

- [ ] **Step 1: Фикстура и падающий тест**

```json
// api/internal/service/zones/testdata/mini.json
{
  "version": 1,
  "stations": [
    {"id": "st:1:frunzenskaya", "name": "Фрунзенская", "lat": 55.7274, "lng": 37.5806},
    {"id": "st:1:sportivnaya", "name": "Спортивная", "lat": 55.7233, "lng": 37.5641},
    {"id": "st:2:sokol", "name": "Сокол", "lat": 55.8050, "lng": 37.5150}
  ],
  "zones": [
    {"id": "line-1", "kind": "line", "name": "красная ветка", "name_in": "на красной ветке",
     "name_gen": "красной ветки", "stations": ["st:1:frunzenskaya", "st:1:sportivnaya"]},
    {"id": "line-2", "kind": "line", "name": "зелёная ветка", "name_in": "на зелёной ветке",
     "name_gen": "зелёной ветки", "stations": ["st:2:sokol"]},
    {"id": "d-khamovniki", "kind": "district", "name": "Хамовники", "name_in": "в Хамовниках",
     "name_gen": "Хамовников", "stations": ["st:1:frunzenskaya", "st:1:sportivnaya"]},
    {"id": "st-sokol", "kind": "station", "name": "Сокол", "name_in": "у метро Сокол",
     "name_gen": "метро Сокол", "stations": ["st:2:sokol"]}
  ]
}
```

```go
// api/internal/service/zones/zones_test.go
package zones

import (
	"os"
	"slices"
	"testing"
)

func miniDirectory(t *testing.T) *Directory {
	t.Helper()
	raw, err := os.ReadFile("testdata/mini.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := New(raw)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return d
}

func TestDirectoryGet(t *testing.T) {
	d := miniDirectory(t)

	z, ok := d.Get("d-khamovniki")
	if !ok {
		t.Fatal("d-khamovniki not found")
	}
	if z.NameIn != "в Хамовниках" || z.Kind != KindDistrict {
		t.Errorf("unexpected zone: %+v", z)
	}
	if _, ok := d.Get("d-nowhere"); ok {
		t.Error("unknown id must not be found")
	}
}

func TestDirectoryIntersecting(t *testing.T) {
	d := miniDirectory(t)

	got := d.Intersecting("d-khamovniki")
	// Себя — обязательно; красную ветку — по общим станциям; зелёную и Сокол — нет.
	if !slices.Contains(got, "d-khamovniki") || !slices.Contains(got, "line-1") {
		t.Errorf("Intersecting(d-khamovniki) = %v, want itself and line-1", got)
	}
	if slices.Contains(got, "line-2") || slices.Contains(got, "st-sokol") {
		t.Errorf("Intersecting(d-khamovniki) = %v, must not contain line-2/st-sokol", got)
	}

	// Симметрия: если A пересекает B, то B пересекает A.
	if !slices.Contains(d.Intersecting("line-1"), "d-khamovniki") {
		t.Error("intersection must be symmetric")
	}
}

func TestDirectoryRejectsBrokenFiles(t *testing.T) {
	tests := []struct{ name, raw string }{
		{"unknown station ref", `{"version":1,"stations":[],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":["ghost"]}]}`},
		{"duplicate zone id", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":["s"]},
			{"id":"z","kind":"line","name":"y","name_in":"y","name_gen":"y","stations":["s"]}]}`},
		{"empty stations", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"x","name_gen":"x","stations":[]}]}`},
		{"missing declension", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"line","name":"x","name_in":"","name_gen":"x","stations":["s"]}]}`},
		{"bad kind", `{"version":1,"stations":[{"id":"s","name":"S"}],"zones":[
			{"id":"z","kind":"galaxy","name":"x","name_in":"x","name_gen":"x","stations":["s"]}]}`},
		{"zero version", `{"version":0,"stations":[],"zones":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New([]byte(tt.raw)); err == nil {
				t.Error("New() accepted a broken file")
			}
		})
	}
}
```

- [ ] **Step 2: Запустить, убедиться что падает**

Run: `cd api && go test ./internal/service/zones/ -v`
Expected: FAIL — `undefined: Directory`.

- [ ] **Step 3: Реализация**

```go
// api/internal/service/zones/zones.go
package zones

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

type Kind string

const (
	KindDistrict Kind = "district"
	KindStation  Kind = "station"
	KindLine     Kind = "line"
)

// Zone is one pickable entry of the directory. Declensions are pre-baked
// strings from the data file — no runtime morphology (§8).
type Zone struct {
	ID      string   `json:"id"`
	Kind    Kind     `json:"kind"`
	Name    string   `json:"name"`
	NameIn  string   `json:"name_in"`  // «в Хамовниках» / «на красной ветке» / «у метро Сокол»
	NameGen string   `json:"name_gen"` // «Хамовников» — for «пул Хамовников»
	Stations []string `json:"stations"`
}

type Station struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Directory is the whole of Moscow, loaded once at startup. Zone intersection
// is precomputed: ~450 zones make that a few milliseconds.
type Directory struct {
	version      int
	raw          []byte
	zones        map[string]Zone
	order        []string            // stable All() order: file order
	intersecting map[string][]string // zone id → ids sharing ≥1 station, self included
}

type file struct {
	Version  int       `json:"version"`
	Stations []Station `json:"stations"`
	Zones    []Zone    `json:"zones"`
}

//go:embed moscow.json
var moscowJSON []byte

func Load() (*Directory, error) {
	return New(moscowJSON)
}

// New parses and validates a directory file, failing fast on anything broken:
// a bad file must kill the process at startup, not a request at noon.
func New(raw []byte) (*Directory, error) {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse zones file: %w", err)
	}
	if f.Version < 1 {
		return nil, fmt.Errorf("zones file version %d, want >= 1", f.Version)
	}

	stations := make(map[string]bool, len(f.Stations))
	for _, s := range f.Stations {
		if s.ID == "" || s.Name == "" {
			return nil, fmt.Errorf("station %q: empty id or name", s.ID)
		}
		if stations[s.ID] {
			return nil, fmt.Errorf("duplicate station id %q", s.ID)
		}
		stations[s.ID] = true
	}

	d := &Directory{
		version:      f.Version,
		raw:          raw,
		zones:        make(map[string]Zone, len(f.Zones)),
		intersecting: make(map[string][]string, len(f.Zones)),
	}
	byStation := make(map[string][]string)
	for _, z := range f.Zones {
		if err := validateZone(z, stations); err != nil {
			return nil, err
		}
		if _, dup := d.zones[z.ID]; dup {
			return nil, fmt.Errorf("duplicate zone id %q", z.ID)
		}
		d.zones[z.ID] = z
		d.order = append(d.order, z.ID)
		for _, st := range z.Stations {
			byStation[st] = append(byStation[st], z.ID)
		}
	}

	for id, z := range d.zones {
		seen := map[string]bool{}
		for _, st := range z.Stations {
			for _, other := range byStation[st] {
				seen[other] = true
			}
		}
		ids := make([]string, 0, len(seen))
		for other := range seen {
			ids = append(ids, other)
		}
		slices.Sort(ids)
		d.intersecting[id] = ids
	}

	return d, nil
}

func (d *Directory) Get(id string) (Zone, bool) {
	z, ok := d.zones[id]
	return z, ok
}

// Intersecting answers the pool boundary (§2.1): every zone sharing at least
// one station with the given one, itself included.
func (d *Directory) Intersecting(id string) []string {
	return d.intersecting[id]
}

func (d *Directory) All() []Zone {
	out := make([]Zone, 0, len(d.order))
	for _, id := range d.order {
		out = append(out, d.zones[id])
	}
	return out
}

func (d *Directory) Version() int { return d.version }

// Raw are the exact embedded bytes — what GET /api/v1/zones serves.
func (d *Directory) Raw() []byte { return d.raw }

func validateZone(z Zone, stations map[string]bool) error {
	if z.ID == "" || z.Name == "" || z.NameIn == "" || z.NameGen == "" {
		return fmt.Errorf("zone %q: empty id, name or declension", z.ID)
	}
	if z.Kind != KindDistrict && z.Kind != KindStation && z.Kind != KindLine {
		return fmt.Errorf("zone %q: unknown kind %q", z.ID, z.Kind)
	}
	if len(z.Stations) == 0 {
		return fmt.Errorf("zone %q: no stations — it would intersect nothing", z.ID)
	}
	for _, st := range z.Stations {
		if !stations[st] {
			return fmt.Errorf("zone %q: unknown station %q", z.ID, st)
		}
	}
	return nil
}
```

Временный `api/internal/service/zones/moscow.json` — копия `testdata/mini.json` (иначе embed не соберётся); настоящий появится в Task 4.

- [ ] **Step 4: Запустить тесты**

Run: `cd api && go test ./internal/service/zones/ -v`
Expected: PASS.

- [ ] **Step 5: Verify**

Run: `cd api && go build ./... && go vet ./... && go test ./...`
Expected: чисто. Не коммитить.

---

### Task 4: Генератор genzones + настоящий moscow.json

Задача с внешними данными: сначала руками посмотреть источники, потом кодировать. Результат — закоммиченный (пользователем) файл данных; генератор остаётся в репе для будущих обновлений.

**Files:**
- Create: `api/cmd/genzones/main.go`
- Create: `api/cmd/genzones/declensions.json` — ручная морфология районов
- Create: `api/cmd/genzones/lines.go` — таблица веток (разговорные имена и склонения)
- Modify: `api/internal/service/zones/moscow.json` — настоящие данные вместо заглушки
- Test: `api/internal/service/zones/moscow_test.go` — инварианты и спот-чеки на реальном файле

**Interfaces:**
- Consumes: `zones.New` из Task 3 (генератор в конце прогоняет валидацию сгенерированного файла через неё).
- Produces: `moscow.json` в формате Task 3.

- [ ] **Step 1: Разведка источников (руками, до кода)**

```bash
# Линии и станции с координатами. Смотрим, какие есть ветки, есть ли МЦК/МЦД:
curl -s https://api.hh.ru/metro/1 | jq '{lines: [.lines[] | {id, name, n: (.stations | length)}]}'
# Границы районов Москвы (муниципальные образования, GeoJSON, WGS84):
curl -sL 'https://gis-lab.info/data/mos-adm/mo.geojson' -o /tmp/mo.geojson || true
jq '.features[0].properties' /tmp/mo.geojson
```

Зафиксировать в комментарии `main.go`: точные id линий hh, имена свойств районов (`NAME`, `ABBREV`, округ — поле вида `OKRUG`/`ABBREV_AO`). Если МЦД (D1–D4) в hh нет — работаем без них и явно пишем это в отчёте задачи пользователю: формат файла позволяет досыпать ветку позже руками. Если `gis-lab` недоступен — запасной источник границ: Overpass (`admin_level=8` внутри Москвы) с конвертацией `osmtogeojson`; выбирается тот, что реально отвечает, факт фиксируется в комментарии.

- [ ] **Step 2: Таблица веток**

```go
// api/internal/cmd... нет — api/cmd/genzones/lines.go
package main

// lineMeta maps the official hh line name to what the product calls it (§6:
// «на оранжевой ветке»). Generation FAILS on an unmapped line, so a new line
// in the source data surfaces loudly instead of silently dropping stations.
type lineMeta struct {
	Slug    string
	Name    string // «красная ветка»
	NameIn  string // «на красной ветке»
	NameGen string // «красной ветки»
}

var linesByOfficialName = map[string]lineMeta{
	"Сокольническая":              {"line-1", "красная ветка", "на красной ветке", "красной ветки"},
	"Замоскворецкая":              {"line-2", "зелёная ветка", "на зелёной ветке", "зелёной ветки"},
	"Арбатско-Покровская":         {"line-3", "синяя ветка", "на синей ветке", "синей ветки"},
	"Филёвская":                   {"line-4", "голубая ветка", "на голубой ветке", "голубой ветки"},
	"Кольцевая":                   {"line-5", "кольцевая ветка", "на кольцевой ветке", "кольцевой ветки"},
	"Калужско-Рижская":            {"line-6", "оранжевая ветка", "на оранжевой ветке", "оранжевой ветки"},
	"Таганско-Краснопресненская":  {"line-7", "фиолетовая ветка", "на фиолетовой ветке", "фиолетовой ветки"},
	"Калининская":                 {"line-8", "жёлтая ветка", "на жёлтой ветке", "жёлтой ветки"},
	"Солнцевская":                 {"line-8a", "жёлтая ветка (Солнцевская)", "на жёлтой ветке", "жёлтой ветки"},
	"Серпуховско-Тимирязевская":   {"line-9", "серая ветка", "на серой ветке", "серой ветки"},
	"Люблинско-Дмитровская":       {"line-10", "салатовая ветка", "на салатовой ветке", "салатовой ветки"},
	"Большая кольцевая":           {"line-11", "БКЛ", "на БКЛ", "БКЛ"},
	"Бутовская":                   {"line-12", "Бутовская ветка", "на Бутовской ветке", "Бутовской ветки"},
	"Некрасовская":                {"line-15", "розовая ветка", "на розовой ветке", "розовой ветки"},
	"Троицкая":                    {"line-16", "Троицкая ветка", "на Троицкой ветке", "Троицкой ветки"},
	"Московское центральное кольцо": {"line-mck", "МЦК", "на МЦК", "МЦК"},
	"МЦД-1": {"line-d1", "МЦД-1", "на МЦД-1", "МЦД-1"},
	"МЦД-2": {"line-d2", "МЦД-2", "на МЦД-2", "МЦД-2"},
	"МЦД-3": {"line-d3", "МЦД-3", "на МЦД-3", "МЦД-3"},
	"МЦД-4": {"line-d4", "МЦД-4", "на МЦД-4", "МЦД-4"},
}
```

(Ключи сверить со Step 1 — hh может писать «Московское центральное кольцо» иначе; править ключи, не поведение. Монорельс, если встретится, — в явный skip-список.)

- [ ] **Step 3: Генератор**

`api/cmd/genzones/main.go`, только stdlib. Флаги: `-metro` (URL или путь, default `https://api.hh.ru/metro/1`), `-districts` (default `/tmp/mo.geojson`), `-declensions` (default рядом с бинарём), `-out api/internal/service/zones/moscow.json`. Алгоритм по шагам, каждый — маленькая функция:

1. **fetchMetro** → структуры hh: `lines[] {name, stations[] {name, lat, lng}}`. Каждая станция получает id `st:{lineSlug}:{translit(name)}` (свой транслит-мап ~35 букв: `а→a … я→ya, ё→e, й→i, ь/ъ→""`, пробел/дефис→`-`).
2. **loadDistricts** → GeoJSON FeatureCollection; фильтр Новой Москвы: отбрасываем фичи, чей округ — Троицкий/Новомосковский (по свойству из Step 1). Полигоны — `[][][2]float64` (внешние кольца, дырки игнорируем — для станций это шум).
3. **stationsForDistrict**: станция принадлежит району, если точка внутри полигона (ray casting, ~25 строк) ИЛИ ближе 400 м к его границе (расстояние точка-отрезок в метрах через локальную проекцию: `dx = Δlng·111320·cos(lat)`, `dy = Δlat·110540`). Безметрошному району — 3 ближайшие станции к центроиду полигона; такие районы перечисляются в отчёте stdout.
4. **stationZones**: группировка станций по нормализованному имени (`lower`, `ё→е`) → зона `st-{translit}` со всеми одноимёнными платформами; `name_in = «у метро {Name}»`, `name_gen = «метро {Name}»`.
5. **districtZones**: id `d-{translit}`, склонения — из `declensions.json` (`{"Хамовники": {"in": "в Хамовниках", "gen": "Хамовников"}, …}`); если района нет в файле склонений — генератор ПАДАЕТ, перечислив недостающие. Файл заполняется руками при первом прогоне (все ~125 районов; исполнитель пишет их сам, это час внимательной работы, а не задача «потом»).
6. **write**: зоны сортируются (`kind`, затем `name`), станции — по id; `version: 1`; в конце генератор прогоняет результат через `zones.New(raw)` и падает, если сам себе сгенерировал невалидное.
7. **report** на stdout: линий, станций, зон каждого вида; районы без станций (со списком приписанных); станции вне всех районов.

- [ ] **Step 4: Прогнать и вычитать**

Run: `cd api && go run ./cmd/genzones -out internal/service/zones/moscow.json`
Expected: отчёт без падения. Вычитка руками:

```bash
jq '{v: .version, stations: (.stations|length), zones: (.zones|length),
     byKind: (.zones | group_by(.kind) | map({(.[0].kind): length}) | add)}' \
  api/internal/service/zones/moscow.json
jq '.zones[] | select(.id=="d-khamovniki")' api/internal/service/zones/moscow.json
```

Спот-чеки глазами: Хамовники содержат Фрунзенскую/Спортивную/Парк культуры; «Сокол» — станция зелёной ветки и районов Сокол/Аэропорт; у БКЛ ≥25 станций.

- [ ] **Step 5: Тесты инвариантов на реальном файле**

```go
// api/internal/service/zones/moscow_test.go
package zones

import (
	"slices"
	"strings"
	"testing"
)

func TestMoscowLoads(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	byKind := map[Kind]int{}
	for _, z := range d.All() {
		byKind[z.Kind]++
	}
	// Порядки величин настоящей Москвы, не точные числа: файл будут обновлять.
	if byKind[KindLine] < 12 {
		t.Errorf("only %d lines", byKind[KindLine])
	}
	if byKind[KindDistrict] < 100 {
		t.Errorf("only %d districts", byKind[KindDistrict])
	}
	if byKind[KindStation] < 200 {
		t.Errorf("only %d station zones", byKind[KindStation])
	}
}

func TestMoscowSpotChecks(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	kham, ok := d.Get("d-khamovniki")
	if !ok {
		t.Fatal("no d-khamovniki")
	}
	if kham.NameIn != "в Хамовниках" || kham.NameGen != "Хамовников" {
		t.Errorf("wrong declensions: %+v", kham)
	}
	// Красная ветка проходит через Хамовники — базовый факт географии,
	// на котором держится вся модель пересечения.
	if !slices.Contains(d.Intersecting("d-khamovniki"), "line-1") {
		t.Error("line-1 must intersect d-khamovniki")
	}

	for _, z := range d.All() {
		switch z.Kind {
		case KindStation:
			if !strings.HasPrefix(z.NameIn, "у метро ") {
				t.Errorf("station zone %s: name_in %q", z.ID, z.NameIn)
			}
		case KindDistrict:
			if !strings.HasPrefix(z.NameIn, "в ") && !strings.HasPrefix(z.NameIn, "на ") {
				t.Errorf("district %s: name_in %q", z.ID, z.NameIn)
			}
		}
	}
}
```

Run: `cd api && go test ./internal/service/zones/ -v`
Expected: PASS.

- [ ] **Step 6: Verify**

Run: `cd api && go build ./... && go vet ./... && go test ./...`
Expected: чисто. Показать пользователю отчёт генератора и размер файла. Не коммитить.

---

### Task 5: searchesrepo — CRUD с лимитом

**Files:**
- Modify: `api/internal/repo/searchesrepo/repo.go` (уже есть с Task 2)
- Create: `api/internal/repo/searchesrepo/model.go`
- Create: `api/internal/repo/searchesrepo/postgres.go`
- Test: `api/internal/repo/searchesrepo/repo_test.go`

**Interfaces:**
- Consumes: `postgres.InTx`, `entity.Search/SearchParams/ErrSearchLimit/ErrSearchClosed/ErrNotFound`, `pgtest.NewPool`.
- Produces (сигнатуры, на которые смотрят Task 6 и 8):

```go
func (r *Repo) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
func (r *Repo) ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
func (r *Repo) UpdateParams(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
func (r *Repo) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
func (r *Repo) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
func (r *Repo) MarkPoolSeen(ctx context.Context, userID uuid.UUID, zoneID string) error
```

- [ ] **Step 1: model.go**

```go
// api/internal/repo/searchesrepo/model.go
package searchesrepo

import (
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const searchColumns = `id, user_id, zone_id, price_min, price_max, rooms_min,
	rooms_max, move_in, term, has_stuff, status, close_reason, pool_seen_at,
	created_at, updated_at`

type searchRow struct {
	ID          uuid.UUID  `db:"id"`
	UserID      uuid.UUID  `db:"user_id"`
	ZoneID      string     `db:"zone_id"`
	PriceMin    *int       `db:"price_min"`
	PriceMax    int        `db:"price_max"`
	RoomsMin    *int       `db:"rooms_min"`
	RoomsMax    *int       `db:"rooms_max"`
	MoveIn      *time.Time `db:"move_in"`
	Term        *string    `db:"term"`
	HasStuff    []string   `db:"has_stuff"`
	Status      string     `db:"status"`
	CloseReason *string    `db:"close_reason"`
	PoolSeenAt  time.Time  `db:"pool_seen_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

func (r searchRow) toEntity() entity.Search {
	s := entity.Search{
		ID:     r.ID,
		UserID: r.UserID,
		SearchParams: entity.SearchParams{
			ZoneID:   r.ZoneID,
			PriceMin: r.PriceMin,
			PriceMax: r.PriceMax,
			RoomsMin: r.RoomsMin,
			RoomsMax: r.RoomsMax,
			MoveIn:   r.MoveIn,
		},
		Status:     entity.ParseSearchStatus(r.Status),
		PoolSeenAt: r.PoolSeenAt,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
	if r.Term != nil {
		term := entity.ParseTerm(*r.Term)
		s.Term = &term
	}
	for _, c := range r.HasStuff {
		s.HasStuff = append(s.HasStuff, entity.ParseStuffChip(c))
	}
	if r.CloseReason != nil {
		s.CloseReason = entity.ParseCloseReason(*r.CloseReason)
	}
	return s
}
```

- [ ] **Step 2: Падающие тесты**

```go
// api/internal/repo/searchesrepo/repo_test.go — ключевые кейсы (полный файл
// пишет исполнитель по этим образцам, seedUser/cleanTestUsers уже есть в
// visibility_test.go этого же пакета)
package searchesrepo

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func testParams(zone string) entity.SearchParams {
	move := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return entity.SearchParams{
		ZoneID:   zone,
		PriceMin: pointer.To(35000),
		PriceMax: 45000,
		MoveIn:   &move,
		Term:     pointer.To(entity.TermYear),
		HasStuff: []entity.StuffChip{entity.StuffFurniture},
	}
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
}

func TestCreateEnforcesLimitUnderConcurrency(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	user := seedUser(t, pool, 1, "active", "male", "any")

	// 5 параллельных созданий: активных должно стать ровно 3 (§2.1),
	// остальные — entity.ErrSearchLimit. FOR UPDATE в Create делает исход
	// детерминированным.
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = repo.Create(t.Context(), user, testParams("d-khamovniki"))
		}()
	}
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
		updated.Term != nil || len(updated.HasStuff) != 0 {
		t.Errorf("old params leaked through: %+v", updated)
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
}
```

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/repo/searchesrepo/ -run 'TestCreate|TestClose|TestUpdate|TestOwnership|TestMark' -v`
Expected: FAIL — методов нет.

- [ ] **Step 3: postgres.go**

```go
// api/internal/repo/searchesrepo/postgres.go
package searchesrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres"
)

// Create inserts an active search after counting the user's active ones under
// a row lock: two concurrent creations must not slip past the limit of 3.
func (r *Repo) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	var out entity.Search
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockAndCheckSlots(ctx, tx, userID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO searches (user_id, zone_id, price_min, price_max,
				rooms_min, rooms_max, move_in, term, has_stuff)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+searchColumns,
			userID, p.ZoneID, p.PriceMin, p.PriceMax, p.RoomsMin, p.RoomsMax,
			p.MoveIn, termArg(p.Term), stuffArg(p.HasStuff))
		if err != nil {
			return err
		}
		out, err = collectSearch(rows)
		return err
	})
	if err != nil {
		return entity.Search{}, fmt.Errorf("create search for user %s: %w", userID, err)
	}
	return out, nil
}

func (r *Repo) ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	return r.one(ctx, `SELECT `+searchColumns+` FROM searches WHERE id = $1 AND user_id = $2`, id, userID)
}

// ListByUser: active first, newest first — the order «Мои поиски» shows.
func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+` FROM searches
		WHERE user_id = $1
		ORDER BY (status = 'active') DESC, created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list searches of user %s: %w", userID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[searchRow])
	if err != nil {
		return nil, fmt.Errorf("list searches of user %s: %w", userID, err)
	}
	out := make([]entity.Search, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

// UpdateParams overwrites the whole parameter set: the form always submits it
// in full, so a cleared field must become NULL rather than survive (§3 —
// изменение параметров/зоны свободное).
func (r *Repo) UpdateParams(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	return r.one(ctx, `
		UPDATE searches SET zone_id = $3, price_min = $4, price_max = $5,
			rooms_min = $6, rooms_max = $7, move_in = $8, term = $9,
			has_stuff = $10, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'closed'
		RETURNING `+searchColumns,
		id, userID, p.ZoneID, p.PriceMin, p.PriceMax, p.RoomsMin, p.RoomsMax,
		p.MoveIn, termArg(p.Term), stuffArg(p.HasStuff))
}

func (r *Repo) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	return r.one(ctx, `
		UPDATE searches SET status = 'closed', close_reason = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'closed'
		RETURNING `+searchColumns, id, userID, string(reason))
}

// Reactivate re-runs the slot check: while the search sat in the archive the
// slots may have been taken (§3 — реактивация из «Моих поисков»).
func (r *Repo) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	var out entity.Search
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockAndCheckSlots(ctx, tx, userID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			UPDATE searches SET status = 'active', close_reason = NULL,
				pool_seen_at = now(), updated_at = now()
			WHERE id = $1 AND user_id = $2 AND status <> 'active'
			RETURNING `+searchColumns, id, userID)
		if err != nil {
			return err
		}
		out, err = collectSearch(rows)
		return err
	})
	if err != nil {
		return entity.Search{}, fmt.Errorf("reactivate search %s: %w", id, err)
	}
	return out, nil
}

func (r *Repo) MarkPoolSeen(ctx context.Context, userID uuid.UUID, zoneID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE searches SET pool_seen_at = now()
		WHERE user_id = $1 AND zone_id = $2 AND status = 'active'`, userID, zoneID)
	if err != nil {
		return fmt.Errorf("mark pool seen for user %s zone %s: %w", userID, zoneID, err)
	}
	return nil
}

// lockAndCheckSlots serialises slot accounting: FOR UPDATE on the user's
// search rows makes concurrent creations queue behind each other.
func lockAndCheckSlots(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT status FROM searches WHERE user_id = $1 FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	statuses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	active := 0
	for _, s := range statuses {
		if s == "active" {
			active++
		}
	}
	if active >= entity.MaxActiveSearches {
		return entity.ErrSearchLimit
	}
	return nil
}

func (r *Repo) one(ctx context.Context, query string, args ...any) (entity.Search, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return entity.Search{}, err
	}
	return collectSearch(rows)
}

// collectSearch maps «no row matched» of the guarded UPDATEs: the row either
// is not yours / does not exist (ErrNotFound) or is closed (ErrSearchClosed) —
// the second query tells which.
func collectSearch(rows pgx.Rows) (entity.Search, error) {
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[searchRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Search{}, entity.ErrNotFound
		}
		return entity.Search{}, err
	}
	return row.toEntity(), nil
}

func termArg(t *entity.Term) *string {
	if t == nil {
		return nil
	}
	return pointer.To(string(*t))
}

func stuffArg(chips []entity.StuffChip) []string {
	out := make([]string, 0, len(chips))
	for _, c := range chips {
		out = append(out, string(c))
	}
	return out
}
```

Тонкость: `Close`/`UpdateParams` по уже закрытому поиску отдают 0 строк → `ErrNotFound`, а тест хочет `ErrSearchClosed`. Разрешение — в `one()` для этих двух методов: если строк 0, дочитать `SELECT status FROM searches WHERE id AND user_id`; статус `closed` → `entity.ErrSearchClosed`, нет строки → `entity.ErrNotFound`. Оформить хелпером `r.oneEditable(ctx, id, userID, query, args...)`, который делают Close и UpdateParams.

- [ ] **Step 4: Прогнать тесты**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/repo/searchesrepo/ -v`
Expected: PASS все, включая конкурентный лимит.

- [ ] **Step 5: Verify**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL=... go test ./...`
Expected: чисто. Не коммитить.

---

### Task 6: Сервис searches

**Files:**
- Create: `api/internal/service/searches/service.go`
- Test: `api/internal/service/searches/service_test.go`
- Modify: `api/internal/service/searches/doc.go` — убрать «Empty on purpose»

**Interfaces:**
- Consumes: `searchesrepo`-сигнатуры из Task 5 (как интерфейс `Repository`, объявленный здесь), `*zones.Directory` (конкретный тип — это статические данные, фейкается через `zones.New` с фикстурой), `entity.*`.
- Produces (их зовёт хендлер в Task 9):

```go
package searches

type Service struct { repo Repository; dir *zones.Directory }
func New(repo Repository, dir *zones.Directory) *Service
var ErrInvalidParams = errors.New("search params are invalid") // оборачивается в entity.FieldsError
func (s *Service) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
func (s *Service) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
func (s *Service) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
```

- [ ] **Step 1: Падающий тест**

```go
// api/internal/service/searches/service_test.go
package searches

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type fakeRepo struct {
	created *entity.SearchParams
}

func (f *fakeRepo) Create(_ context.Context, _ uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	f.created = &p
	return entity.Search{SearchParams: p, Status: entity.SearchActive}, nil
}
func (f *fakeRepo) ByID(context.Context, uuid.UUID, uuid.UUID) (entity.Search, error) {
	return entity.Search{}, entity.ErrNotFound
}
func (f *fakeRepo) ListByUser(context.Context, uuid.UUID) ([]entity.Search, error) {
	return nil, nil
}
func (f *fakeRepo) UpdateParams(_ context.Context, _, _ uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	f.created = &p
	return entity.Search{SearchParams: p}, nil
}
func (f *fakeRepo) Close(context.Context, uuid.UUID, uuid.UUID, entity.CloseReason) (entity.Search, error) {
	return entity.Search{}, nil
}
func (f *fakeRepo) Reactivate(context.Context, uuid.UUID, uuid.UUID) (entity.Search, error) {
	return entity.Search{}, nil
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

func TestCreateRejectsUnknownZone(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-atlantis", PriceMax: 40000})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Contains(fields.Fields, "zone_id") {
		t.Errorf("Create() = %v, want ErrInvalidParams on zone_id", err)
	}
	if repo.created != nil {
		t.Error("repo must not be called on invalid params")
	}
}

func TestCreateRejectsBadParams(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 0})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !slices.Contains(fields.Fields, "price_max") {
		t.Errorf("Create() = %v, want price_max complaint", err)
	}
}

func TestCreateNormalisesMoveInToMonthStart(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	mid := time.Date(2026, 9, 17, 13, 45, 0, 0, time.UTC)
	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 40000, MoveIn: &mid})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if repo.created.MoveIn == nil || !repo.created.MoveIn.Equal(want) {
		t.Errorf("MoveIn = %v, want %v", repo.created.MoveIn, want)
	}
}

func TestCreateValidPassesThrough(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	got, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "line-1", PriceMax: 40000})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if got.Status != entity.SearchActive || repo.created == nil {
		t.Errorf("create did not reach the repo: %+v", got)
	}
}
```

Run: `cd api && go test ./internal/service/searches/ -v`
Expected: FAIL — пакета нет.

- [ ] **Step 2: Реализация**

```go
// api/internal/service/searches/service.go
package searches

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
	ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	UpdateParams(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
	Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
	Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
}

type Service struct {
	repo Repository
	dir  *zones.Directory
}

var ErrInvalidParams = errors.New("search params are invalid")

func New(repo Repository, dir *zones.Directory) *Service {
	return &Service{repo: repo, dir: dir}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	p, err := s.validated(p)
	if err != nil {
		return entity.Search{}, err
	}
	return s.repo.Create(ctx, userID, p)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	p, err := s.validated(p)
	if err != nil {
		return entity.Search{}, err
	}
	return s.repo.UpdateParams(ctx, userID, id, p)
}

func (s *Service) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	if reason == entity.CloseReasonUnknown || reason == "" {
		return entity.Search{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"reason"}}
	}
	return s.repo.Close(ctx, userID, id, reason)
}

func (s *Service) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	return s.repo.Reactivate(ctx, userID, id)
}

// validated normalises and checks params. The zone check lives here and not
// in entity: only the directory knows which slugs exist.
func (s *Service) validated(p entity.SearchParams) (entity.SearchParams, error) {
	if p.MoveIn != nil {
		monthStart := time.Date(p.MoveIn.Year(), p.MoveIn.Month(), 1, 0, 0, 0, 0, time.UTC)
		p.MoveIn = &monthStart
	}

	bad := p.InvalidFields()
	if _, ok := s.dir.Get(p.ZoneID); p.ZoneID != "" && !ok {
		bad = append(bad, "zone_id")
	}
	if len(bad) > 0 {
		return entity.SearchParams{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: bad}
	}
	return p, nil
}
```

В `doc.go` заменить последнюю строку «Empty on purpose — built in §12 step 3.» на «Built in §12 step 3.»

- [ ] **Step 3: Прогнать тесты**

Run: `cd api && go test ./internal/service/searches/ -v`
Expected: PASS.

- [ ] **Step 4: Verify**

Run: `cd api && go build ./... && go vet ./... && go test ./...`
Expected: чисто. Не коммитить.

---

### Task 7: pools — чипы и секции (чистая логика) + SharedAnswers

**Files:**
- Create: `api/internal/service/pools/verdicts.go`
- Test: `api/internal/service/pools/verdicts_test.go`
- Modify: `api/internal/entity/profile.go` — метод `SharedAnswers`
- Test: `api/internal/entity/profile_shared_test.go`

**Interfaces:**
- Produces (их зовёт Task 8 и сериализует Task 9):

```go
package pools

type Verdict string // "green" | "edge" | "grey" | "none" (none = «не указана» у него)
type Param string   // "price" | "rooms" | "move_in" | "term"
type SectionID string // "all" | "partial" | "none" — как в свёрстанном фронте
type Chip struct { Param Param; Verdict Verdict }
func Compare(mine, theirs entity.SearchParams) ([]Chip, SectionID)
func BetterSection(a, b SectionID) bool // строгий порядок all > partial > none

// entity
func (p Profile) SharedAnswers(o Profile) int
```

- [ ] **Step 1: Падающие тесты**

```go
// api/internal/service/pools/verdicts_test.go
package pools

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

func month(m time.Month) *time.Time {
	t := time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
	return &t
}

func params(priceMin, priceMax int, moveIn *time.Time, term *entity.Term) entity.SearchParams {
	p := entity.SearchParams{PriceMax: priceMax, MoveIn: moveIn, Term: term}
	if priceMin > 0 {
		p.PriceMin = pointer.To(priceMin)
	}
	return p
}

func chipOf(chips []Chip, param Param) Verdict {
	for _, c := range chips {
		if c.Param == param {
			return c.Verdict
		}
	}
	return "missing"
}

func TestCompareSections(t *testing.T) {
	sept, dec := month(time.September), month(time.December)
	year := pointer.To(entity.TermYear)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		want         SectionID
	}{
		// «35–45» и «40–55» совпали — канонический пример §2.1.
		{"overlap is a match", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), SectionAll},
		// NULL = «неважно»: минимальные параметры совпадают со всеми.
		{"both minimal", params(0, 45000, nil, nil), params(0, 40000, nil, nil), SectionAll},
		// Незаданные у него параметры не мешают полному совпадению.
		{"their unspecified ok", params(35000, 45000, sept, year), params(40000, 55000, nil, nil), SectionAll},
		// Касание границ — «краем», но для секций это пересечение.
		{"edge counts as match", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), SectionAll},
		// Цена мимо, заезд совпал → частично.
		{"price off move on", params(0, 40000, sept, nil), params(60000, 80000, sept, nil), SectionPartial},
		// Цена совпала, заезд мимо → частично.
		{"price on move off", params(35000, 45000, sept, nil), params(40000, 55000, dec, nil), SectionPartial},
		// Мягкий параметр (комнаты) ломает «полностью», но не «частично».
		{"soft param breaks full", 
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(1)},
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(3), RoomsMax: pointer.To(4)},
			SectionPartial},
		// Ни цена, ни заезд → не совпали.
		{"both hard off", params(0, 40000, sept, nil), params(60000, 80000, dec, nil), SectionNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := Compare(tt.mine, tt.theirs); got != tt.want {
				t.Errorf("Compare() section = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareChips(t *testing.T) {
	sept, oct, dec := month(time.September), month(time.October), month(time.December)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		param        Param
		want         Verdict
	}{
		{"price overlap", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), ParamPrice, VerdictGreen},
		{"price touch", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), ParamPrice, VerdictEdge},
		{"price miss", params(0, 40000, nil, nil), params(60000, 80000, nil, nil), ParamPrice, VerdictGrey},
		{"same month", params(0, 40000, sept, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictGreen},
		{"next month", params(0, 40000, sept, nil), params(0, 40000, oct, nil), ParamMoveIn, VerdictEdge},
		{"far month", params(0, 40000, sept, nil), params(0, 40000, dec, nil), ParamMoveIn, VerdictGrey},
		{"their unset month", params(0, 40000, sept, nil), params(0, 40000, nil, nil), ParamMoveIn, VerdictNone},
		// Моё «неважно» совпадает со всем — его чип зелёный.
		{"my unset month", params(0, 40000, nil, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictGreen},
		{"term equal", params(0, 40000, nil, pointer.To(entity.TermYear)), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictGreen},
		{"term neighbour", params(0, 40000, nil, pointer.To(entity.TermHalfYear)), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictEdge},
		{"term far", params(0, 40000, nil, pointer.To(entity.TermHalfYear)), params(0, 40000, nil, pointer.To(entity.TermOpen)), ParamTerm, VerdictGrey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chips, _ := Compare(tt.mine, tt.theirs)
			if got := chipOf(chips, tt.param); got != tt.want {
				t.Errorf("chip %s = %q, want %q", tt.param, got, tt.want)
			}
		})
	}
}

// Инвариант спеки: в секции «Совпали по всем» не бывает серого чипа.
func TestFullSectionNeverHasGreyChips(t *testing.T) {
	sept := month(time.September)
	cases := []entity.SearchParams{
		params(35000, 45000, sept, pointer.To(entity.TermYear)),
		params(0, 40000, nil, nil),
		params(20000, 90000, sept, nil),
	}
	for _, mine := range cases {
		for _, theirs := range cases {
			chips, section := Compare(mine, theirs)
			if section != SectionAll {
				continue
			}
			for _, c := range chips {
				if c.Verdict == VerdictGrey {
					t.Errorf("grey chip %s in full section: mine=%+v theirs=%+v", c.Param, mine, theirs)
				}
			}
		}
	}
}
```

```go
// api/internal/entity/profile_shared_test.go
package entity

import "testing"

func TestSharedAnswers(t *testing.T) {
	a := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseQuiet}
	b := Profile{Schedule: ScheduleOwl, Smoking: SmokingNo, Noise: NoiseLoud}
	if got := a.SharedAnswers(b); got != 2 {
		t.Errorf("SharedAnswers() = %d, want 2", got)
	}
	// Пустые поля не совпадение: две недозаполненные анкеты дают 0, не 9.
	if got := (Profile{}).SharedAnswers(Profile{}); got != 0 {
		t.Errorf("empty SharedAnswers() = %d, want 0", got)
	}
}
```

(Точные имена констант `ScheduleOwl`/`SmokingNo`/`NoiseQuiet`/`NoiseLoud` сверить с `profile.go` — использовать существующие.)

Run: `cd api && go test ./internal/service/pools/ ./internal/entity/ -v`
Expected: FAIL.

- [ ] **Step 2: Реализация verdicts.go**

```go
// api/internal/service/pools/verdicts.go
package pools

import (
	"math"

	"flatmatch/api/internal/entity"
)

type Verdict string

const (
	VerdictGreen Verdict = "green" // пересеклись
	VerdictEdge  Verdict = "edge"  // «краем»: границы касаются / соседние ступени
	VerdictGrey  Verdict = "grey"  // не пересеклись
	VerdictNone  Verdict = "none"  // у него не указан — «не указана», не участвует
)

type Param string

const (
	ParamPrice  Param = "price"
	ParamRooms  Param = "rooms"
	ParamMoveIn Param = "move_in"
	ParamTerm   Param = "term"
)

// SectionID values match the vested frontend (ZonePoolScreen): «Совпали по
// всем параметрам» / «Пересекаются частично» / «Не совпали».
type SectionID string

const (
	SectionAll     SectionID = "all"
	SectionPartial SectionID = "partial"
	SectionNone    SectionID = "none"
)

type Chip struct {
	Param   Param
	Verdict Verdict
}

var sectionRank = map[SectionID]int{SectionNone: 0, SectionPartial: 1, SectionAll: 2}

// Compare scores their search against mine: one chip per parameter plus the
// section by §2.1. Chips first, section derived from them, so a card can
// never sit in «Совпали по всем» wearing a grey chip.
func Compare(mine, theirs entity.SearchParams) ([]Chip, SectionID) {
	chips := []Chip{
		{ParamPrice, priceVerdict(mine, theirs)},
		{ParamRooms, roomsVerdict(mine, theirs)},
		{ParamMoveIn, moveInVerdict(mine, theirs)},
		{ParamTerm, termVerdict(mine, theirs)},
	}

	// «Пересёкся» = зелёный, «краем» или не участвует. Жёсткие — цена и заезд.
	ok := func(p Param) bool {
		for _, c := range chips {
			if c.Param == p {
				return c.Verdict != VerdictGrey
			}
		}
		return true
	}
	priceOK, moveOK := ok(ParamPrice), ok(ParamMoveIn)

	switch {
	case priceOK && moveOK && ok(ParamRooms) && ok(ParamTerm):
		return chips, SectionAll
	case priceOK || moveOK:
		return chips, SectionPartial
	default:
		return chips, SectionNone
	}
}

func BetterSection(a, b SectionID) bool {
	return sectionRank[a] > sectionRank[b]
}

// priceVerdict: «до» is mandatory, so price always participates; a nil «от»
// is 0. Ranges overlap → green, touch exactly → edge («35–45» и «45–55»).
func priceVerdict(mine, theirs entity.SearchParams) Verdict {
	aMin, aMax := orZero(mine.PriceMin), mine.PriceMax
	bMin, bMax := orZero(theirs.PriceMin), theirs.PriceMax
	return rangeVerdict(aMin, aMax, bMin, bMax)
}

func roomsVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.RoomsMin == nil && theirs.RoomsMax == nil {
		return VerdictNone
	}
	if mine.RoomsMin == nil && mine.RoomsMax == nil {
		return VerdictGreen // моё «неважно» совпадает со всем (§2.1)
	}
	const wide = math.MaxInt32
	aMin, aMax := orOne(mine.RoomsMin), orWide(mine.RoomsMax, wide)
	bMin, bMax := orOne(theirs.RoomsMin), orWide(theirs.RoomsMax, wide)
	return rangeVerdict(aMin, aMax, bMin, bMax)
}

func moveInVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.MoveIn == nil {
		return VerdictNone
	}
	if mine.MoveIn == nil {
		return VerdictGreen
	}
	a, b := *mine.MoveIn, *theirs.MoveIn
	apart := (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
	switch abs(apart) {
	case 0:
		return VerdictGreen
	case 1:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

// termVerdict: the ladder полгода+ → год+ → бессрочно; neighbours are «краем».
func termVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.Term == nil {
		return VerdictNone
	}
	if mine.Term == nil {
		return VerdictGreen
	}
	rank := map[entity.Term]int{entity.TermHalfYear: 0, entity.TermYear: 1, entity.TermOpen: 2}
	switch abs(rank[*mine.Term] - rank[*theirs.Term]) {
	case 0:
		return VerdictGreen
	case 1:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

func rangeVerdict(aMin, aMax, bMin, bMax int) Verdict {
	switch {
	case aMax > bMin && bMax > aMin:
		return VerdictGreen
	case aMax == bMin || bMax == aMin:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

func orZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func orOne(v *int) int {
	if v == nil {
		return 1
	}
	return *v
}

func orWide(v *int, wide int) int {
	if v == nil {
		return wide
	}
	return *v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

`SharedAnswers` в `api/internal/entity/profile.go` (после `MissingFields`):

```go
// SharedAnswers counts questionnaire fields both people filled in equally —
// the «N совпадений анкеты» number (§6). A compatibility score is deferred
// (§9), this is deliberately a plain count.
func (p Profile) SharedAnswers(o Profile) int {
	pairs := [][2]string{
		{string(p.Schedule), string(o.Schedule)},
		{string(p.Smoking), string(o.Smoking)},
		{string(p.PetsOwn), string(o.PetsOwn)},
		{string(p.PetsOK), string(o.PetsOK)},
		{string(p.Noise), string(o.Noise)},
		{string(p.Guests), string(o.Guests)},
		{string(p.Cleaning), string(o.Cleaning)},
		{string(p.Alcohol), string(o.Alcohol)},
		{string(p.Expenses), string(o.Expenses)},
	}
	n := 0
	for _, pair := range pairs {
		if pair[0] != "" && pair[0] == pair[1] {
			n++
		}
	}
	return n
}
```

- [ ] **Step 3: Прогнать тесты**

Run: `cd api && go test ./internal/service/pools/ ./internal/entity/ -v`
Expected: PASS.

- [ ] **Step 4: Verify**

Run: `cd api && go build ./... && go vet ./... && go test ./...`
Expected: чисто. Не коммитить.

---

### Task 8: pools — сервис, выборка кандидатов, интеграционный тест

**Files:**
- Create: `api/internal/service/pools/service.go`
- Modify: `api/internal/service/pools/doc.go` — убрать «Empty on purpose»
- Modify: `api/internal/entity/search.go` — добавить `entity.PoolCandidate`
- Modify: `api/internal/repo/searchesrepo/model.go`, `postgres.go` — `PoolCandidates`
- Test: `api/internal/service/pools/service_db_test.go`

**Interfaces:**
- Consumes: `Compare`/`BetterSection` (Task 7), `zones.Directory` (Task 3), `Repo.ListByUser/MarkPoolSeen` (Task 5), `visible_people` (Task 2).
- Produces (сериализует Task 9):

```go
// entity (рядом с Search):
type PoolCandidate struct { User User; Search Search }

// searchesrepo:
func (r *Repo) PoolCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]entity.PoolCandidate, error)

// pools:
type Repository interface {
	PoolCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]entity.PoolCandidate, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	MarkPoolSeen(ctx context.Context, userID uuid.UUID, zoneID string) error
}
type PhotoURLs interface{ URL(key string) string }

type Person struct {
	User          entity.User
	PhotoURL      string
	SharedAnswers int
	Chips         []Chip           // nil в неранжированном пуле
	Params        entity.SearchParams
	SearchCreated time.Time
}
type Section struct { ID SectionID; People []Person }
type Pool struct {
	Zone     zones.Zone
	MySearch *entity.Search // активный поиск смотрящего ровно этой зоны, если есть
	Ranked   bool
	Sections []Section // ranked: всегда 3 в порядке all/partial/none
	People   []Person  // unranked: один список
}
type Counts struct {
	People    int
	FullMatch int
	NewCount  int
	Avatars   []string // URL, максимум 4
}

func New(repo Repository, dir *zones.Directory, photos PhotoURLs) *Service
func (s *Service) ZonePool(ctx context.Context, viewer entity.User, zoneID string) (Pool, error)
func (s *Service) Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (Counts, error)
```

- [ ] **Step 1: PoolCandidates в репо**

`entity.PoolCandidate` — в `search.go` после `Search`. В `searchesrepo/model.go` — `candidateRow` с алиасами двух таблиц (все u_-поля, которые нужны мини-карточке и SharedAnswers):

```go
type candidateRow struct {
	searchRow // без изменений: колонки s.* идут без префикса

	UID        uuid.UUID `db:"u_id"`
	UName      string    `db:"u_name"`
	UAge       *int      `db:"u_age"`
	UGender    *string   `db:"u_gender"`
	USchedule  *string   `db:"u_schedule"`
	USmoking   *string   `db:"u_smoking"`
	UPetsOwn   *string   `db:"u_pets_own"`
	UPetsOK    *string   `db:"u_pets_ok"`
	UNoise     *string   `db:"u_noise"`
	UGuests    *string   `db:"u_guests"`
	UCleaning  *string   `db:"u_cleaning"`
	UAlcohol   *string   `db:"u_alcohol"`
	UExpenses  *string   `db:"u_expenses"`
	UPhotoKey  *string   `db:"u_photo_key"`
	ULastSeen  time.Time `db:"u_last_seen"`
}
```

`toEntity()` собирает `entity.PoolCandidate` через существующие `entity.Parse*` (образец маппинга — `usersrepo/model.go`). Запрос:

```sql
SELECT s.id, s.user_id, s.zone_id, s.price_min, s.price_max, s.rooms_min,
       s.rooms_max, s.move_in, s.term, s.has_stuff, s.status, s.close_reason,
       s.pool_seen_at, s.created_at, s.updated_at,
       u.id AS u_id, u.name AS u_name, u.age AS u_age, u.gender AS u_gender,
       u.schedule AS u_schedule, u.smoking AS u_smoking,
       u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
       u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
       u.expenses AS u_expenses, u.photo_key AS u_photo_key,
       u.last_seen AS u_last_seen
FROM searches s
JOIN users u ON u.id = s.user_id
WHERE s.status = 'active'
  AND s.zone_id = ANY($2)
  AND s.user_id IN (SELECT visible_people($1))
ORDER BY s.created_at DESC
```

- [ ] **Step 2: service.go**

```go
// api/internal/service/pools/service.go — ядро (импорты и мелочи очевидны)
package pools

const (
	avatarCap      = 4  // стек аватаров промо-блока
	noneSectionCap = 20 // хвост «не совпавших» отсекается топ-N (§6)
)

type Service struct {
	repo   Repository
	dir    *zones.Directory
	photos PhotoURLs
}

func New(repo Repository, dir *zones.Directory, photos PhotoURLs) *Service {
	return &Service{repo: repo, dir: dir, photos: photos}
}

// ZonePool computes the pool of one zone for one viewer (§2.1): candidates
// from every intersecting zone, one card per person by their best verdict,
// ranked under the viewer's own search when they have one here.
func (s *Service) ZonePool(ctx context.Context, viewer entity.User, zoneID string) (Pool, error) {
	zone, ok := s.dir.Get(zoneID)
	if !ok {
		return Pool{}, entity.ErrNotFound
	}

	mine, err := s.repo.ListByUser(ctx, viewer.ID)
	if err != nil {
		return Pool{}, err
	}
	var mySearch *entity.Search // exact zone first, else freshest intersecting
	intersecting := s.dir.Intersecting(zoneID)
	for i, m := range mine {
		if m.Status != entity.SearchActive {
			continue
		}
		if m.ZoneID == zoneID {
			mySearch = &mine[i]
			break
		}
		if mySearch == nil && slices.Contains(intersecting, m.ZoneID) {
			mySearch = &mine[i]
		}
	}

	cands, err := s.repo.PoolCandidates(ctx, viewer.ID, intersecting)
	if err != nil {
		return Pool{}, err
	}

	pool := Pool{Zone: zone, MySearch: exactOnly(mySearch, zoneID), Ranked: mySearch != nil}
	if mySearch == nil {
		pool.People = unrankedPeople(s.photos, viewer, cands)
	} else {
		pool.Sections = rankedSections(s.photos, viewer, *mySearch, cands)
	}

	// Side effect of looking: the «M новых» badge of this zone's search resets.
	if err := s.repo.MarkPoolSeen(ctx, viewer.ID, zoneID); err != nil {
		return Pool{}, err
	}
	return pool, nil
}

// Counts backs every counter that must agree with the pool: the feed promo
// block, «Мои поиски» cards, the empty-zone state. Same candidates, same
// Compare — the numbers cannot diverge from the sections by construction.
func (s *Service) Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (Counts, error) {
	cands, err := s.repo.PoolCandidates(ctx, viewerID, s.dir.Intersecting(params.ZoneID))
	if err != nil {
		return Counts{}, err
	}

	best := bestPerUser(params, cands) // map[uuid.UUID]scored, keeps insertion order separately
	c := Counts{People: len(best.order)}
	for _, userID := range best.order {
		sc := best.byUser[userID]
		if sc.section == SectionAll {
			c.FullMatch++
		}
		if newSince != nil && sc.cand.Search.CreatedAt.After(*newSince) {
			c.NewCount++
		}
		if len(c.Avatars) < avatarCap && sc.cand.User.PhotoKey != "" {
			c.Avatars = append(c.Avatars, s.photos.URL(sc.cand.User.PhotoKey))
		}
	}
	return c, nil
}
```

Приватные хелперы (все в этом же файле): `bestPerUser(params, cands)` — группировка по `User.ID`, лучший по `BetterSection`, при равенстве — свежайший поиск; `rankedSections` — три секции в порядке all/partial/none, внутри сортировка по `Search.CreatedAt` убыв., секция none режется `noneSectionCap`; `unrankedPeople` — сортировка «заполненность параметров (счёт non-nil полей), затем свежесть», без чипов; `exactOnly(s, zoneID)` — nil, если поиск не ровно этой зоны (в `MySearch` попадает только точное совпадение — для плашки «Ты в пуле»; ранжирование при этом работает и от пересекающегося). `Person` собирается: `PhotoURL = photos.URL(User.PhotoKey)`, `SharedAnswers = viewer.Profile.SharedAnswers(cand.User.Profile)`.

- [ ] **Step 3: Интеграционный тест**

```go
// api/internal/service/pools/service_db_test.go — скелет; фикстура зон из
// Task 3, seed-хелперы по образцу searchesrepo (продублировать локально
// seedUser/cleanTestUsers с диапазоном testTGBase = -997000000).
func TestZonePoolEndToEnd(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := searchesrepo.New(pool)
	svc := New(repo, testDir(t), staticPhotos{}) // staticPhotos{}.URL(k) = "/media/" + k

	viewer := seedActiveUser(t, pool, 1, "male", "any")   // + анкета: сова, не курит
	full := seedActiveUser(t, pool, 2, "female", "any")   // поиск: Хамовники 40–55к
	partial := seedActiveUser(t, pool, 3, "male", "any")  // поиск: красная ветка, 60–80к, заезд совпал
	invisible := seedActiveUser(t, pool, 4, "female", "same") // отсечена полом
	mustCreate(t, repo, viewer, params("d-khamovniki", 35000, 45000, sept))
	mustCreate(t, repo, full, params("d-khamovniki", 40000, 55000, sept))
	mustCreate(t, repo, partial, params("line-1", 60000, 80000, sept))
	mustCreate(t, repo, invisible, params("d-khamovniki", 40000, 45000, sept))

	viewerUser := loadUser(t, pool, viewer)
	got, err := svc.ZonePool(t.Context(), viewerUser, "d-khamovniki")
	if err != nil {
		t.Fatalf("ZonePool() failed: %v", err)
	}

	if !got.Ranked || got.MySearch == nil {
		t.Fatalf("viewer has a search here, pool must be ranked: %+v", got)
	}
	assertSectionHas(t, got, SectionAll, full)       // пересечение вилок
	assertSectionHas(t, got, SectionPartial, partial) // пришёл через пересечение зон (line-1)
	assertNowhere(t, got, invisible)                  // visible_people отработала
	assertNowhere(t, got, viewer)                     // себя в пуле нет

	counts, err := svc.Counts(t.Context(), viewer, entity.SearchParams{ZoneID: "d-khamovniki", PriceMin: pointer.To(35000), PriceMax: 45000, MoveIn: sept}, nil)
	if err != nil {
		t.Fatalf("Counts() failed: %v", err)
	}
	if counts.People != 2 || counts.FullMatch != 1 {
		t.Errorf("Counts = %+v, want people=2 full=1", counts)
	}
}
```

Плюс тест `TestZonePoolUnrankedWithoutOwnSearch` (смотрящий без поиска → `Ranked=false`, один список, чипов нет) и `TestPoolSeenResetsNewCount` (Counts с `newSince` до/после `ZonePool`). Хелперы `assertSectionHas`/`assertNowhere`/`mustCreate`/`loadUser`/`staticPhotos` — писать полностью, они тривиальны.

- [ ] **Step 4: Прогнать**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/service/pools/ -v`
Expected: PASS.

- [ ] **Step 5: Verify**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL=... go test ./...`
Expected: чисто. Не коммитить.

---

### Task 9: HTTP-слой и wiring

**Files:**
- Create: `api/internal/api/http/zonesapi/handler.go`
- Create: `api/internal/api/http/searchesapi/handler.go`, `dto.go`, `errors.go`
- Create: `api/internal/api/http/poolsapi/handler.go`, `dto.go`
- Modify: `api/internal/httpx/respond.go` — коды
- Modify: `api/cmd/api/main.go` — сборка и роуты

**Interfaces:**
- Consumes: `searches.Service` (Task 6), `pools.Service` (Task 8), `zones.Directory` (Task 3), `authmw.FromContext`, `httpx`.
- Produces: ручки из дизайн-спеки §4 (см. таблицу ниже) — их будет дёргать фронт.

- [ ] **Step 1: Коды ошибок в httpx**

```go
	CodeSearchLimit  Code = "search_limit"
	CodeSearchClosed Code = "search_closed"
```

(`unknown_zone` отдельным кодом не нужен: незнакомая зона — это `invalid_field` с `fields: ["zone_id"]`, фронт подсвечивает поле формы. Дизайн-спека упоминала `unknown_zone` — упрощаем до одного кода, отметить в отчёте.)

- [ ] **Step 2: zonesapi**

```go
// api/internal/api/http/zonesapi/handler.go
// Package zonesapi serves the zone directory: the embedded file, byte for
// byte. The picker searches client-side (§ дизайн-спека /zones).
package zonesapi

import (
	"fmt"
	"net/http"

	"flatmatch/api/internal/service/zones"
)

type Handler struct {
	dir *zones.Directory
}

func New(dir *zones.Directory) *Handler { return &Handler{dir: dir} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	etag := fmt.Sprintf(`"zones-v%d"`, h.dir.Version())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(h.dir.Raw())
}
```

- [ ] **Step 3: searchesapi**

`dto.go` — запрос и ответ:

```go
type paramsRequest struct {
	ZoneID   string   `json:"zone_id"`
	PriceMin *int     `json:"price_min"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min"`
	RoomsMax *int     `json:"rooms_max"`
	MoveIn   *string  `json:"move_in"` // «2026-09»
	Term     *string  `json:"term"`
	HasStuff []string `json:"has_stuff"`
}

func (b paramsRequest) toParams() (entity.SearchParams, error) // move_in: time.Parse("2006-01"); ошибка → FieldsError{move_in}

type zoneDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameIn  string `json:"name_in"`
	NameGen string `json:"name_gen"`
}

type searchResponse struct {
	ID        uuid.UUID `json:"id"`
	Zone      zoneDTO   `json:"zone"`
	PriceMin  *int      `json:"price_min,omitempty"`
	PriceMax  int       `json:"price_max"`
	RoomsMin  *int      `json:"rooms_min,omitempty"`
	RoomsMax  *int      `json:"rooms_max,omitempty"`
	MoveIn    *string   `json:"move_in,omitempty"` // «2026-09»
	Term      *string   `json:"term,omitempty"`
	HasStuff  []string  `json:"has_stuff"`
	Status    string    `json:"status"`
	CloseReason *string `json:"close_reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Счётчики пула — только в GET-списке (в ответах мутаций нули не врут,
	// они просто не считаются: фронт после мутаций перезапрашивает список).
	People      int `json:"people"`
	FullMatch   int `json:"full_match"`
	NewCount    int `json:"new_count"`
	PointsCount int `json:"points_count"` // всегда 0 до §12 шага 5
}

type closeRequest struct {
	Reason string `json:"reason"`
}
```

`handler.go` — интерфейсы потребителя и роуты:

```go
type Searches interface {
	Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
	List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	Update(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
	Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
	Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
}
type Pools interface {
	Counts(ctx context.Context, viewerID uuid.UUID, params entity.SearchParams, newSince *time.Time) (pools.Counts, error)
}
type ZoneDir interface{ Get(id string) (zones.Zone, bool) }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)          // счётчики: по Counts на каждый активный поиск
	r.Post("/", h.create)       // 201
	r.Patch("/{id}", h.update)
	r.Post("/{id}/close", h.close)
	r.Post("/{id}/reactivate", h.reactivate)
	return r
}
```

`list` для каждого активного поиска зовёт `Pools.Counts(userID, s.SearchParams, &s.PoolSeenAt)`; для paused/closed счётчики нулевые (пул им не считаем — они вне пулов). `errors.go`:

```go
func handleError(w http.ResponseWriter, r *http.Request, err error) {
	var fields *entity.FieldsError
	switch {
	case errors.Is(err, entity.ErrNotFound):
		httpx.Error(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such search", err)
	case errors.Is(err, entity.ErrSearchLimit):
		httpx.BadRequest(w, r, httpx.CodeSearchLimit, "поиски заняты 3 из 3", nil, err)
	case errors.Is(err, entity.ErrSearchClosed):
		httpx.BadRequest(w, r, httpx.CodeSearchClosed, "search is closed, reactivate it first", nil, err)
	case errors.As(err, &fields):
		httpx.BadRequest(w, r, httpx.CodeInvalidField, "value outside the allowed set", fields.Fields, err)
	default:
		httpx.Internal(w, r, err)
	}
}
```

- [ ] **Step 4: poolsapi**

`dto.go`:

```go
type promoDTO struct {
	Zone       zoneDTO   `json:"zone"`
	People     int       `json:"people"`
	FullMatch  int       `json:"full_match"`
	Avatars    []string  `json:"avatars"`
	MySearchID *uuid.UUID `json:"my_search_id,omitempty"`
	SlotsUsed  int       `json:"slots_used"`
	SlotsTotal int       `json:"slots_total"` // = entity.MaxActiveSearches
}

type feedResponse struct {
	Promo  promoDTO `json:"promo"`
	Points []any    `json:"points"` // всегда [] до §12 шага 5
}

type personDTO struct {
	UserID        uuid.UUID  `json:"user_id"`
	Name          string     `json:"name"`
	Age           int        `json:"age"`
	PhotoURL      string     `json:"photo_url"`
	Schedule      string     `json:"schedule"`
	Smoking       string     `json:"smoking"`
	PetsOwn       string     `json:"pets_own"`
	SharedAnswers int        `json:"shared_answers"`
	Chips         []chipDTO  `json:"chips,omitempty"`
	Params        paramsDTO  `json:"params"` // его вилки — фронт рисует подписи чипов
}
type chipDTO struct {
	Param   string `json:"param"`
	Verdict string `json:"verdict"`
}
// paramsDTO is the outbound twin of searchesapi's paramsRequest: same fields
// and encodings (move_in «2026-09»), zone omitted — the pool's zone is on top.
type paramsDTO struct {
	PriceMin *int     `json:"price_min,omitempty"`
	PriceMax int      `json:"price_max"`
	RoomsMin *int     `json:"rooms_min,omitempty"`
	RoomsMax *int     `json:"rooms_max,omitempty"`
	MoveIn   *string  `json:"move_in,omitempty"`
	Term     *string  `json:"term,omitempty"`
	HasStuff []string `json:"has_stuff,omitempty"`
}

type poolResponse struct {
	Zone       zoneDTO      `json:"zone"`
	MySearchID *uuid.UUID   `json:"my_search_id,omitempty"`
	Ranked     bool         `json:"ranked"`
	Sections   []sectionDTO `json:"sections,omitempty"` // ranked
	People     []personDTO  `json:"people,omitempty"`   // unranked
}
type sectionDTO struct {
	ID     string      `json:"id"` // all | partial | none
	People []personDTO `json:"people"`
}
```

`handler.go`: `GET /feed` — параметры из query (`zone_id`, `price_min`, `price_max`, `rooms_min`, `rooms_max`, `move_in=2026-09`, `term`), обязателен только `zone_id` + `price_max`; парс-ошибки → `invalid_field`. Зовёт `Searches.List` (slots_used = число active; `my_search_id` = active с точным `zone_id`) и `Pools.Counts`. `GET /pools/zone/{zoneID}` → `pools.ZonePool(viewer, zoneID)` (viewer целиком из `authmw.FromContext` — `SharedAnswers` нужен профиль). Интерфейсы `Pools`/`Searches` объявляет поsвоему усечённо.

- [ ] **Step 5: Wiring в main.go**

```go
	dir, err := zones.Load()
	if err != nil {
		return err
	}
	searchesRepo := searchesrepo.New(pool)
	searchesService := searches.New(searchesRepo, dir)
	poolsService := pools.New(searchesRepo, dir, photos)
```

В `router()` (сигнатуру расширить):

```go
	pool := poolsapi.New(poolsService, searchesService, dir)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.New(validator, accounts))
		r.Mount("/", usersapi.New(accounts).Routes())
		r.Method(http.MethodGet, "/zones", zonesapi.New(dir))
		r.Mount("/searches", searchesapi.New(searchesService, poolsService, dir).Routes())
		// chi allows only one Mount per pattern, so poolsapi's two routes are
		// registered directly instead of via a second Mount("/", …).
		r.Get("/feed", pool.Feed)
		r.Get("/pools/zone/{zoneID}", pool.ZonePool)
	})
```

(`poolsapi.Handler` поэтому экспортирует методы `Feed` и `ZonePool` вместо `Routes()`.)

- [ ] **Step 6: Ручная проходка curl**

Стенд: `docker compose up -d postgres && cd api && go run ./cmd/api` (BOT_POLLING_ENABLED=false достаточно). Авторизация — заголовок из `go run ./cmd/devinitdata`.

```bash
AUTH="Authorization: tma $(go run ./cmd/devinitdata)"
# Справочник: 200 + ETag; повтор с If-None-Match → 304.
curl -si -H "$AUTH" localhost:8080/api/v1/zones | head -5
# Создание: 201 с телом поиска.
curl -si -H "$AUTH" -X POST localhost:8080/api/v1/searches \
  -d '{"zone_id":"d-khamovniki","price_min":35000,"price_max":45000,"move_in":"2026-09"}'
# Четвёртый активный: 400 search_limit (создать ещё 3).
# Незнакомая зона: 400 invalid_field fields=["zone_id"].
# Выдача: promo с my_search_id и slots_used=N.
curl -s -H "$AUTH" 'localhost:8080/api/v1/feed?zone_id=d-khamovniki&price_max=45000' | jq
# Пул: ranked=true, три секции (пустые — сидера ещё нет).
curl -s -H "$AUTH" localhost:8080/api/v1/pools/zone/d-khamovniki | jq
# Закрыть/реактивировать: статусы и причина в ответе.
```

Expected: коды и формы ответов как в комментариях; расхождения чинить до перехода дальше.

- [ ] **Step 7: Verify**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL=... go test ./...`
Expected: чисто. Не коммитить.

---

### Task 10: Сидер devseed

**Files:**
- Create: `api/cmd/devseed/main.go`
- Modify: `README.md` — раздел «Сидер фейковых людей» + абзац про genzones в существующий раздел про локальный запуск

**Interfaces:**
- Consumes: `config.Load`, `postgres.NewPool`, `photostore.NewDir`, прямой SQL (dev-инструмент, репозитории не обязательны).

- [ ] **Step 1: Сидер**

`main.go`: `config.Load()` → pool + photostore. Данные — таблица в коде:

```go
// Deterministic fake people: fixed tg_ids, upsert — reruns converge to the
// same state. Searches are wiped and re-inserted each run.
const seedTGBase int64 = 900000001

type seedPerson struct {
	n        int64
	name     string
	age      int
	gender   string // male / female
	pref     string // any / same
	schedule string // owl / lark / floats — точные значения из entity сверить
	smoking  string
	petsOwn  string
	zone     string
	priceMin int // 0 = NULL
	priceMax int
	moveIn   string // "" = NULL, иначе "2026-09-01"
	term     string // "" = NULL
}

var people = []seedPerson{
	// Полные совпадения с вилкой 35–45к/сентябрь в Хамовниках:
	{1, "Катя", 22, "female", "any", "owl", "no", "none", "d-khamovniki", 40000, 55000, "2026-09-01", "year"},
	{2, "Олег", 25, "male", "any", "lark", "no", "cat", "d-khamovniki", 35000, 45000, "2026-09-01", ""},
	// Пересечение через ветку (красная проходит через Хамовники):
	{3, "Ира", 23, "female", "any", "owl", "balcony", "none", "line-1", 30000, 42000, "2026-10-01", "half_year"},
	// «Краем» по цене (граница 45к):
	{4, "Дима", 27, "male", "any", "floats", "no", "dog", "d-khamovniki", 45000, 60000, "2026-09-01", ""},
	// Частично: цена мимо, заезд совпал:
	{5, "Настя", 24, "female", "any", "owl", "no", "none", "d-khamovniki", 60000, 80000, "2026-09-01", "year"},
	// Не совпали: и цена, и заезд мимо:
	{6, "Глеб", 30, "male", "any", "lark", "home", "none", "d-khamovniki", 70000, 90000, "2026-12-01", ""},
	// Минимальный поиск — только «до» (всё остальное «не указана»):
	{7, "Лена", 21, "female", "any", "floats", "no", "none", "d-khamovniki", 0, 50000, "", ""},
	// Обоюдная невидимость для м-смотрящего:
	{8, "Аня", 26, "female", "same", "owl", "no", "cat", "d-khamovniki", 35000, 45000, "2026-09-01", ""},
	// Соседние зоны/ветки для плотности (11–20 по образцу выше — исполнитель
	// добивает вариациями: line-5, st-sokol, d-arbat, разные возрасты и
	// привычки; каждая строка должна отличаться хотя бы двумя полями):
	// …
}
```

Алгоритм: (1) один PNG-плейсхолдер: `image.NewRGBA(128×128)` заливка серым, `png.Encode` → `photos.Put(ctx, "seed/avatar.png", …)`; (2) на каждого — `INSERT INTO users (tg_id, name, age, gender, gender_pref, occupation, schedule, smoking, pets_own, pets_ok, noise, guests, cleaning, alcohol, expenses, about, photo_key, photo_status, about_status, status) VALUES (…, 'ok', 'ok', 'active') ON CONFLICT (tg_id) DO UPDATE SET …все поля` (заполнить и остальные привычки — циклом по 2-3 вариантам, чтобы SharedAnswers давал разные счётчики; about — короткая строка «Сид для разработки»); (3) `DELETE FROM searches WHERE user_id IN (сиды)`, затем INSERT поисков из таблицы; (4) вывод: сколько людей/поисков.

- [ ] **Step 2: Прогнать и проверить руками**

```bash
cd api && go run ./cmd/devseed
psql "$TEST_DATABASE_URL" -c "SELECT count(*) FROM users WHERE tg_id >= 900000001; SELECT status, count(*) FROM searches GROUP BY 1"
AUTH="Authorization: tma $(go run ./cmd/devinitdata)"
curl -s -H "$AUTH" 'localhost:8080/api/v1/feed?zone_id=d-khamovniki&price_min=35000&price_max=45000&move_in=2026-09' | jq .promo
# Ожидаемо: people ≥ 6, full_match ≥ 2, avatars непустые.
curl -s -H "$AUTH" localhost:8080/api/v1/pools/zone/d-khamovniki | jq '{sections: [.sections[] | {id, n: (.people|length)}]}'
# Ожидаемо: непустые all и partial, в none — Глеб. Аня (same) НЕ видна
# м-смотрящему нигде; проверить и обратное — сменить пол dev-юзера PATCH'ем.
```

Второй прогон `devseed` — идемпотентность: счётчики те же.

- [ ] **Step 3: README**

Короткий раздел: зачем сидер, команда, диапазон tg_id, как снести (`DELETE FROM users WHERE tg_id >= 900000001`). Абзац про genzones: когда перегенерировать, откуда данные, что склонения — ручной файл.

- [ ] **Step 4: Verify**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL=... go test ./...`
Expected: чисто. Не коммитить.

---

### Task 11: Финальная проходка

**Files:**
- Modify: ничего нового — только если проходка найдёт баги.

- [ ] **Step 1: Полный прогон**

Run: `cd api && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...`
Expected: PASS всё.

- [ ] **Step 2: Сквозной сценарий по DoD §13 (бэк-часть)**

На стенде с сидером, curl'ом, фиксируя ответы:
1. справочник отдаётся, ETag работает;
2. выдача по параметрам НЕ создала поиск (`GET /searches` пуст до POST);
3. создание → промо `my_search_id`; 4-й актив → `search_limit`;
4. пул: три секции, чипы с вердиктами, «не указана» у минимального поиска Лены; Аня невидима;
5. `new_count` в `GET /searches` обнуляется после открытия пула;
6. закрытие с причиной → слот освободился; реактивация при занятых слотах → `search_limit`;
7. правка параметров поиска меняет секции пула соответственно.

- [ ] **Step 3: Отчёт пользователю**

Свод: что готово (ручки, данные, тесты — с числами), что осталось от шага 3 (подключение фронта: лента, форма параметров, промо-блок, экран пула, «Мои поиски»), отклонения от дизайн-спеки, если появились. §12/§13 в CLAUDE.md не трогать: шаг 3 не закрыт, пока нет фронта. Не коммитить — пользователь смотрит и коммитит сам.
