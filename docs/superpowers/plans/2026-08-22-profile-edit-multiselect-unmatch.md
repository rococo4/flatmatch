# Шаг 10 — наборы в поиске, экран анкеты, удаление мэтча — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Задачи трекаются чекбоксами.

**Goal:** Комнатность, срок и месяц заезда поиска становятся наборами; анкета редактируется отдельным экраном-списком (макет B); мэтч удаляется любой стороной, блок стирает мэтч и лайки пары.

**Architecture:** Go-монолит `api/` (handler → service → storage, pgx, goose, chi), React+TS `web/`. Спека — `docs/superpowers/specs/2026-08-22-profile-edit-multiselect-unmatch-design.md` (истина при конфликте с планом; CLAUDE.md на диске — истина продукта, снапшот в контексте сабагента может быть устаревшим). Макет экрана анкеты: вариант B на холсте (описан в спеке §2 словами — этого достаточно).

**Tech Stack:** Go 1.26, pgx v5, goose, chi; React 19 + TS + Vite, @telegram-apps/sdk-react.

**Spec:** docs/superpowers/specs/2026-08-22-profile-edit-multiselect-unmatch-design.md

## Global Constraints

- Go и SQL — **ноль комментариев** (только `//go:`, `//nolint`, `-- +goose`). Порядок в Go-файле: константы и типы наверху, функции ниже, публичные раньше приватных. Интерфейсы — на стороне потребителя. Фронт — комментарии в существующем стиле (объясняют «почему»).
- Русская микрокопия — ДОСЛОВНО из спеки (таблица «Решения», §2, §3) и CLAUDE.md §4 (вопросы и варианты анкеты).
- Верификация Go: `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...` (локальный Postgres: `docker compose up -d postgres` из корня). Миграции к дев-БД применяет сам api при старте; руками — `cd api && go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$TEST_DATABASE_URL" up`. Фронт: `cd web && npx tsc -b && npm run -s lint && npm run build`. Тесты репо — общая дев-БД: чистить только свои строки, не трогать users tg_id=188993596.
- Коммиты в main (короткое русское сообщение + trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`), затем `git push origin main`. Прод НЕ деплоить из задач — это делает контроллер.
- В этом шаге НЕТ: нескольких зон в поиске, пуша об удалении мэтча, восстановления мэтча, редактирования точек.

---

### Task 1: Наборы в поиске — бэкенд

**Files:**
- Create: `api/migrations/0011_search_sets.sql`
- Modify: `api/internal/entity/search.go` (+`search_test.go`), `api/internal/repo/searchesrepo/{model,postgres}.go` (+`repo_test.go`), `api/internal/repo/pointsrepo/model.go` (memberColumns + `toSearch`), `api/internal/service/pools/verdicts.go` (+`verdicts_test.go`), `api/internal/service/matches/summary.go` (+тест), `api/internal/api/http/searchesapi/dto.go`, `api/internal/api/http/poolsapi/dto.go`, `api/internal/api/http/pointsapi/dto.go`, `api/internal/api/http/matchesapi/dto.go`, и все места, где компилятор укажет на `RoomsMin/RoomsMax/MoveIn/Term` (devseed, тесты хендлеров).

**Interfaces (Produces):**
```go
// entity
type SearchParams struct {
	ZoneID       string
	PriceMin     *int
	PriceMax     int
	Rooms        []int        // подмножество {1,2,3,4}; 4 = «4+»; пусто = неважно
	Terms        []Term       // пусто = неважно
	MoveInMonths []time.Time  // первое число месяца, UTC; пусто = неважно
	HasStuff     []StuffChip
}
const RoomsSetCap = 4
func (p SearchParams) Normalized() SearchParams   // сортирует и дедуплицирует наборы
func (p SearchParams) InvalidFields() []string    // "rooms" | "terms" | "move_in" | прежние
// matches
type ParamOverlap struct {
	Param    string
	Min, Max int       // только price
	Values   []string  // rooms: "2","3"; move_in: "2026-09"; term: "year"
}
// JSON (searches/pools/points/matches DTO)
// params: {"rooms":[2,3],"terms":["year"],"move_in":["2026-09","2026-10"]}; пустой массив = неважно
// overlap: {"param":"rooms","values":["2","3"]}
```

- [ ] **Шаг 1. Миграция** — `api/migrations/0011_search_sets.sql`, SQL дословно из спеки §1 «Схема» (`-- +goose Up` / `-- +goose Down`; Down — обратное: добавить старые колонки, `rooms_min = (SELECT min(x) FROM unnest(rooms) x)`, `rooms_max = (SELECT max(x) …)` с `NULL` при `max = 4`, `term = terms[1]`, `move_in = move_in_months[1]`, дроп массивов). Применить к дев-БД, проверить на существующих поисках: `select id, rooms, terms, move_in_months from searches limit 5`.

- [ ] **Шаг 2. Entity — тесты первыми** (`search_test.go`, в стиле соседних):

```go
func TestSearchParamsSetsValidation(t *testing.T) {
	sept := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		p    entity.SearchParams
		want []string
	}{
		{"empty sets are fine", entity.SearchParams{ZoneID: "z", PriceMax: 45000}, nil},
		{"rooms above cap", entity.SearchParams{ZoneID: "z", PriceMax: 45000, Rooms: []int{5}}, []string{"rooms"}},
		{"rooms below one", entity.SearchParams{ZoneID: "z", PriceMax: 45000, Rooms: []int{0}}, []string{"rooms"}},
		{"unknown term", entity.SearchParams{ZoneID: "z", PriceMax: 45000, Terms: []entity.Term{entity.TermUnknown}}, []string{"terms"}},
		{"month not first day", entity.SearchParams{ZoneID: "z", PriceMax: 45000, MoveInMonths: []time.Time{sept.AddDate(0, 0, 3)}}, []string{"move_in"}},
	}
	// … цикл, сравнение slices.Equal(got, tc.want)
}

func TestSearchParamsNormalizedSortsAndDedupes(t *testing.T) {
	p := entity.SearchParams{Rooms: []int{3, 2, 3}, Terms: []entity.Term{entity.TermYear, entity.TermYear}}
	got := p.Normalized()
	// want Rooms {2,3}, Terms {year}
}
```

Реализация: `Normalized` — `slices.Sort` + `slices.Compact` по каждому набору (месяцы — по `Unix()`), возвращает копию; `InvalidFields`: `"rooms"` если элемент `< 1 || > RoomsSetCap`; `"terms"` если элемент `== TermUnknown`; `"move_in"` если `Day() != 1`. Старые проверки `rooms_min/rooms_max/term` убрать. `RoomsCap` (20) остаётся для точек.

- [ ] **Шаг 3. Репозиторий.** `searchesrepo`: `searchColumns`/`candidateColumns` — `rooms, terms, move_in_months` вместо четырёх колонок; `searchRow{Rooms []int32 \`db:"rooms"\`; Terms []string \`db:"terms"\`; MoveInMonths []time.Time \`db:"move_in_months"\`}` (pgx отдаёт `int[]` как `[]int32` — конвертировать в `[]int`); `Create`/`UpdateParams` пишут три массива (`p = p.Normalized()` перед INSERT/UPDATE; `termsArg([]Term) []string`); `toEntity` собирает наборы (`ParseTerm` на каждый). `pointsrepo.memberColumns` и `memberRow.toSearch` — те же три колонки. Тест `repo_test.go`: round-trip — `Create` с `Rooms{3,2}`, `Terms{TermYear}`, `MoveInMonths{sept, oct}` → `ByID` возвращает `{2,3}`, `{year}`, `{sept, oct}` (сравнивать месяцы через `Equal`).

- [ ] **Шаг 4. Вердикты — тесты первыми** (`verdicts_test.go`; хелперы `params`/`withRooms` переписать под наборы):

```go
func TestSetVerdicts(t *testing.T) {
	sept, oct, dec := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		param        Param
		want         Verdict
	}{
		{"their rooms unset", entity.SearchParams{Rooms: []int{2}}, entity.SearchParams{}, ParamRooms, VerdictNone},
		{"my rooms unset", entity.SearchParams{}, entity.SearchParams{Rooms: []int{2}}, ParamRooms, VerdictGreen},
		{"rooms intersect", entity.SearchParams{Rooms: []int{2, 3}}, entity.SearchParams{Rooms: []int{3, 4}}, ParamRooms, VerdictGreen},
		{"rooms disjoint", entity.SearchParams{Rooms: []int{1}}, entity.SearchParams{Rooms: []int{3, 4}}, ParamRooms, VerdictGrey},
		{"months neighbour is grey not edge", entity.SearchParams{MoveInMonths: []time.Time{sept}}, entity.SearchParams{MoveInMonths: []time.Time{oct}}, ParamMoveIn, VerdictGrey},
		{"months intersect", entity.SearchParams{MoveInMonths: []time.Time{sept, oct}}, entity.SearchParams{MoveInMonths: []time.Time{oct, dec}}, ParamMoveIn, VerdictGreen},
		{"terms intersect", entity.SearchParams{Terms: []entity.Term{entity.TermYear, entity.TermOpen}}, entity.SearchParams{Terms: []entity.Term{entity.TermOpen}}, ParamTerm, VerdictGreen},
		{"terms disjoint is grey not edge", entity.SearchParams{Terms: []entity.Term{entity.TermHalfYear}}, entity.SearchParams{Terms: []entity.Term{entity.TermYear}}, ParamTerm, VerdictGrey},
	}
	// … chips, _ := Compare(withPrice(tc.mine), withPrice(tc.theirs)); chipOf(chips, tc.param) == tc.want
}
```

Реализация в `verdicts.go`:

```go
func setVerdict(mine, theirs []string) Verdict {
	if len(theirs) == 0 {
		return VerdictNone
	}
	if len(mine) == 0 {
		return VerdictGreen
	}
	for _, v := range mine {
		if slices.Contains(theirs, v) {
			return VerdictGreen
		}
	}
	return VerdictGrey
}

func roomsVerdict(mine, theirs entity.SearchParams) Verdict  { return setVerdict(roomKeys(mine.Rooms), roomKeys(theirs.Rooms)) }
func moveInVerdict(mine, theirs entity.SearchParams) Verdict { return setVerdict(monthKeys(mine.MoveInMonths), monthKeys(theirs.MoveInMonths)) }
func termVerdict(mine, theirs entity.SearchParams) Verdict   { return setVerdict(termKeys(mine.Terms), termKeys(theirs.Terms)) }
```
(`roomKeys` — `strconv.Itoa`, `monthKeys` — `Format("2006-01")`, `termKeys` — `string`). `termRank`, `roomsWide`, `orOne`, `orWide`, `abs` — удалить, если больше не используются. `priceVerdict` и `Compare` без изменений; `TestCompareSections` переписать на наборы с теми же ожиданиями секций.

- [ ] **Шаг 5. Саммари мэтча.** `summary.go`: `ParamOverlap` += `Values []string`; `paramsOverlap`: комнаты/месяцы/сроки — пересечение наборов в порядке `mine`, строка только если у ОБОИХ набор непуст и пересечение непусто; цена как была. Тест: `{2,3}×{3,4}` → `Values{"3"}`; `{sept,oct}×{oct}` → `{"2026-10"}`; пусто у одного → строки нет. `matchesapi/dto.go`: `paramOverlapDTO` += `Values []string \`json:"values,omitempty"\``, `toParamOverlap` копирует; поля `Month`/`Term` убрать.

- [ ] **Шаг 6. DTO.** `searchesapi.paramsRequest{Rooms []int \`json:"rooms"\`; Terms []string \`json:"terms"\`; MoveIn []string \`json:"move_in"\`}` → `toParams`: месяцы `time.Parse("2006-01")` поэлементно (ошибка → `FieldsError{Fields:{"move_in"}}`), сроки `ParseTerm`; `searchResponse`, `poolsapi.paramsDTO`, `pointsapi.paramsDTO`: `Rooms []int \`json:"rooms"\``, `Terms []string \`json:"terms"\``, `MoveIn []string \`json:"move_in"\`` — всегда массивы (`make(..., 0)`), без omitempty; старые поля удалить. Тест хендлера `searchesapi`: `POST /searches` с `{"rooms":[2,3],"terms":["year"],"move_in":["2026-09"]}` → 201 и те же массивы в ответе; `{"rooms":[7]}` → 400 `invalid_field` с `fields:["rooms"]`.

- [ ] **Шаг 7.** Devseed (`api/cmd/devseed` или где сиды) и остальные компиляционные хвосты. Верификация Go полностью. Коммит «Шаг 10: наборы в поиске — бэкенд», push.

---

### Task 2: Наборы в поиске — фронт

**Files:**
- Modify: `web/src/api/client.ts` (`SearchParams`, `ParamOverlap`), `web/src/api/format.ts` (`chipLabel`, `paramChips`, `overlapRow`, новые `roomsSetLabel`, `monthsLabel`, `termsLabel`; удалить `roomsShort`/`roomsLong`, если не осталось потребителей), `web/src/screens/SearchParamsScreen.tsx` (`SearchDraft`, `EMPTY_DRAFT`, `isDraftReady`, `draftToParams`, `paramsToDraft`, чипы), `web/src/screens/MatchView.tsx`, `web/src/screens/ZonePoolView.tsx`, `web/src/screens/FlatPoolView.tsx`, `web/src/App.tsx` (`paramChips`), mockEnv/фикстуры, где есть `rooms_min`.

**Interfaces (Consumes Task 1 JSON; Produces):**
```ts
export interface SearchParams { zone_id: string; price_min?: number | null; price_max: number;
  rooms: number[]; terms: Term[]; move_in: string[]; has_stuff?: string[] }
export interface ParamOverlap { param: 'price'|'rooms'|'move_in'|'term'; min?: number; max?: number; values?: string[] }
export type SearchDraft = { zoneId: string | null; priceMin: string; priceMax: string;
  rooms: number[]; terms: Term[]; moveIn: string[] }
// format.ts
export function roomsSetLabel(rooms: number[]): string | null   // [] → null; [2] → «2 комн.»; [2,3] → «2–3 комн.»; [4] → «4+ комн.»; [2,3,4] → «от 2 комн.»; [1,3] → «1, 3 комн.»; [1,2,3,4] → «любая комнатность»
export function monthsLabel(months: string[], form: 'name' | 'in'): string | null  // [] → null; ['2026-09'] → «сентябрь»/«в сентябре»; ['2026-09','2026-10'] → «сентябрь или октябрь»/«в сентябре или октябре»; 3+ → через запятую
export function termsLabel(terms: Term[]): string | null  // [] → null; join ' / ' по TERM_LABEL
```

- [ ] **Шаг 1.** Типы и `format.ts`: `chipLabel` — `rooms` → `roomsSetLabel(params.rooms) ?? NOT_SET.rooms`, `move_in` → `monthsLabel(params.move_in, 'name')`, `term` → `termsLabel(params.terms)`; `paramChips` — `roomsSetLabel` (полная форма: «2–3 комнаты» не нужна, оставить «2–3 комн.»), `заезд ${monthsLabel(p.move_in, 'in')}`, `termsLabel`; `overlapRow` — `rooms`: `у обоих ${roomsSetLabel(values.map(Number))}`, `move_in`: `${monthsLabel(values, 'name')} у обоих`, `term`: `${termsLabel(values)} у обоих`; `null`, когда `values` пуст.

- [ ] **Шаг 2.** Форма: `ROOMS_PRESETS` → `ROOM_CHIPS = [1, 2, 3, 4]` с лейблами «1», «2», «3», «4+»; `TERM_CHIPS` — три значения; месяцы — `monthOptions(6)`. В каждом ряду первый чип «неважно» (`active = набор пуст`, тап очищает), остальные — переключатели: `toggle(list, v)` добавляет/убирает. Подписи под рядами дословно: комнатность — «Можно несколько: «2» и «3» — обе подойдут», заезд — «Несколько месяцев — если дата плавает». `isDraftReady` без изменений; `draftToParams`: `rooms: d.rooms, terms: d.terms, move_in: d.moveIn`; `paramsToDraft` обратно (`s.rooms ?? []`).

- [ ] **Шаг 3.** Потребители: `ZonePoolView`/`FlatPoolView` (`chipLabel` — сигнатура та же), `MatchView` (`overlapRow`), `App.tsx` (`paramChips`), `mockEnv`/фикстуры. `npx tsc -b` покажет все хвосты. Верификация фронта. Ручная приёмка на стенде (`.claude/skills/running-locally`): сохранить поиск «2 и 3, сентябрь или октябрь» → карточка в «Моих поисках» и чипы выдачи показывают наборы; пул — вердикты ✓/серый без «краем» у комнат. Коммит «Шаг 10: наборы в поиске — фронт», push.

---

### Task 3: Удаление мэтча и блок — бэкенд

**Files:**
- Modify: `api/internal/repo/matchesrepo/postgres.go` (+`repo_test.go`), `api/internal/repo/usersrepo/postgres.go` (`Block`; +тест), `api/internal/repo/notificationsrepo/postgres.go` (`FailPendingByContext`; +тест), `api/internal/service/matches/service.go` (`Delete`; +тест), `api/internal/api/http/matchesapi/handler.go` (`DELETE /{id}`; +тест), `api/cmd/api/main.go` (wiring `Queue` в matches).

**Interfaces (Produces):**
```go
// matchesrepo
func (r *Repo) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error)
// usersrepo — Block теперь транзакция: снос пары + блок
func (r *Repo) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
// notificationsrepo
func (r *Repo) FailPendingByContext(ctx context.Context, contextKey string) error
// matches service
type Repository interface { ...; Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) }
type Queue interface { FailPendingByContext(ctx context.Context, contextKey string) error }
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error  // entity.ErrNotFound, если не участник / нет
// HTTP: DELETE /api/v1/matches/{id} → 204 | 404 not_found
```

- [ ] **Шаг 1. `matchesrepo.Delete` — тест первым** (`repo_test.go`, свои пользователи через существующий `seedUser`/хелперы пакета; лайки и мэтч вставлять SQL-ом как в `ttl_test.go` searchesrepo):

```go
func TestDeleteRemovesMatchLikesAndPolls(t *testing.T) {
	// a, b — активные; likes_users a→b и b→a (spent true/false); matches (least, greatest); match_polls одна строка
	ok, err := repo.Delete(ctx, a, matchID)            // true, nil
	// likes_users между a и b: 0 строк; match_polls по matchID: 0; matches по id: 0
	ok, err = repo.Delete(ctx, a, matchID)             // false, nil — повтор
}

func TestDeleteByOutsiderDoesNothing(t *testing.T) {
	// c — третий; repo.Delete(ctx, c, matchID) → false; мэтч и лайки на месте
}
```

Реализация — одна транзакция (`postgres.InTx`):

```sql
SELECT a_id, b_id FROM matches WHERE id = $1 AND $2::uuid IN (a_id, b_id) FOR UPDATE;   -- нет строки → false
DELETE FROM likes_users WHERE (from_id, to_id) IN (($a, $b), ($b, $a));
DELETE FROM match_polls WHERE match_id = $1;
DELETE FROM matches WHERE id = $1;
```

- [ ] **Шаг 2. `usersrepo.Block` — тест первым:** пара a↔b с мэтчем и двумя лайками; `Block(a, b)` → строка в `blocks`, мэтча нет, лайков нет, `match_polls` нет; повтор — без ошибки. Реализация — транзакция: `DELETE FROM match_polls WHERE match_id IN (SELECT id FROM matches WHERE a_id = least($1,$2) AND b_id = greatest($1,$2))`; `DELETE FROM matches WHERE a_id = least(...) AND b_id = greatest(...)`; `DELETE FROM likes_users WHERE (from_id, to_id) IN (($1,$2),($2,$1))`; `INSERT INTO blocks ... ON CONFLICT DO NOTHING`.

- [ ] **Шаг 3. `FailPendingByContext`** — тест: две строки `notifications` с `context_key = 'match:<id>'` (одна sent, одна pending) + чужая pending; после вызова pending по ключу — `failed_at` стоит, чужая — нет. SQL: `UPDATE notifications SET failed_at = now() WHERE context_key = $1 AND sent_at IS NULL AND failed_at IS NULL`. Ключ контекста мэтча — тот же, что пишет `Matched`/`PostMatch` в `api/internal/service/notifications/pushes.go` (посмотреть там константу/формат и переиспользовать через экспортированную функцию `notifications.MatchContextKey(id uuid.UUID) string`, если её нет — добавить рядом с тем местом, где ключ формируется, и использовать в обоих).

- [ ] **Шаг 4. Сервис и хендлер.** `matches.Service.Delete`: `ok, err := s.repo.Delete(ctx, userID, id)`; `!ok` → `entity.ErrNotFound`; затем best-effort `s.queue.FailPendingByContext(context.WithoutCancel(ctx), notifications.MatchContextKey(id))` с `Warn` в лог при ошибке (как `pushVerdict` в moderation). `matchesapi.Matches` += `Delete`; `r.Delete("/{id}", h.remove)` → 204 / 404 через общий `handleError` пакета. Тест хендлера: 204 у участника (фейк возвращает nil), 404 у `ErrNotFound`. `main.go`: `matches.New(matchesRepo, searchesRepo, usersRepo, pointsRepo, notificationsRepo, dir)`.

- [ ] **Шаг 5.** Проверка поведения после удаления — тест в `searchesrepo` или `likesrepo`: после `Delete` пара видна в `PoolCandidates` (раньше исключалась `NOT EXISTS matches`), повторный `likesrepo.Create(a → b)` даёт `Like.Spent == true` (встречного лайка уже нет). Верификация Go полностью. Коммит «Шаг 10: удаление мэтча и блок — бэкенд», push.

---

### Task 4: Удаление мэтча и блок — фронт

**Files:**
- Modify: `web/src/api/client.ts` (+`deleteMatch(id)`), `web/src/screens/EdgeStates.tsx` (+`UnmatchSheet`; `BlockPersonSheet` += `note?: string`), `web/src/screens/MatchScreen.tsx` (строка действий + пропсы), `web/src/screens/MatchView.tsx` (шторки и вызовы), `web/src/App.tsx` (перечитка мэтчей/входящих после удаления/блока, возврат назад).

**Interfaces:**
```ts
export function deleteMatch(id: string): Promise<void>   // DELETE /api/v1/matches/{id}, 204
// MatchScreen props +=
onUnmatch?: () => void; onBlock?: () => void;
// EdgeStates
export function UnmatchSheet({ name, busy, error, onCancel, onConfirm }: {...})
// BlockPersonSheet += note?: string  (строка под предупреждением: «Мэтч тоже удалится»)
```

- [ ] **Шаг 1.** `client.ts`: `deleteMatch`. `EdgeStates.tsx`: `UnmatchSheet` по образцу `BlockPersonSheet` — заголовок «Удалить мэтч с {name}?», текст «Он пропадёт у обоих, лайки сгорят, и вы снова увидите друг друга в пулах. Отменить нельзя.», кнопки «Отмена» (disabled при busy) и красная «Удалить»; `onDismiss={busy ? undefined : onCancel}`. `BlockPersonSheet`: проп `note`, рендерится второй строкой подписи.

- [ ] **Шаг 2.** `MatchScreen`: под саммари (перед `L.fixed`) строка действий как в `PersonProfileScreen` (`L.actions`/`L.quiet` — скопировать стили): «Удалить мэтч» · «Заблокировать»; рендерится, когда передан хотя бы один обработчик. `MatchView`: состояние `sheet: 'unmatch' | 'block' | null`, `busy`, `error`; «Удалить» → `deleteMatch(id)` → `onRemoved()`; «Заблокировать» → `blockPerson(match.peer.user_id)` → `onRemoved()`; ошибки — в шторку (`messageOf`). Новый проп `onRemoved: () => void`.

- [ ] **Шаг 3.** `App.tsx`: `onRemoved` → `back()` к «Мои мэтчи»/профилю, затем `reloadMatches()` и `reloadLikes()` (лайк второго больше не входящий), при блоке — ещё `refreshFeed()`/`reloadSearches()` как в существующем `onBlocked`. Верификация фронта; ручная приёмка на стенде с двумя сидовыми пользователями: удалить мэтч → у обоих список пуст, оба снова в зонном пуле друг у друга; повторный лайк тратит слот. Коммит «Шаг 10: удаление мэтча и блок — фронт», push.

---

### Task 5: Экран редактирования анкеты (макет B)

**Files:**
- Create: `web/src/screens/ProfileEditScreen.tsx`
- Modify: `web/src/screens/OnboardingScreens.tsx` (экспортировать `OWN_PETS`, `OTHER_PETS`, лейблы пола/предпочтения из `BasicsStep` как константы; `AboutStep`/`PhotoStep` не трогать), `web/src/screens/EdgeStates.tsx` (+`ChoiceSheet`, `AboutSheet`, `AgeSheet`), `web/src/App.tsx` (вью `editProfile` → новый экран для не-draft; `fixProfile`; диплинки `photo`/`about`; убрать `editStep`/`initialStep`), `web/src/screens/OnboardingFlow.tsx` (убрать проп `initialStep`, если больше не нужен), `web/src/screens/MyProfileScreen.tsx` (без изменений API; проверить `onEdit`/`onFix`).

**Interfaces:**
```ts
export function ProfileEditScreen({
  profile, autoOpen, onProfile, onBack,
}: {
  profile: Profile;
  /** Открыть шторку сразу: строка статуса «фото не подошло» ведёт сюда с нужной шторкой. */
  autoOpen?: 'photo' | 'about';
  onProfile: (next: Profile) => void;   // ответ сервера — истина (patchProfile/uploadPhoto/submitProfile)
  onBack: () => void;
})
type EditRow = { key: 'age' | 'gender' | 'gender_pref' | 'pets_own' | 'pets_ok' | QuestionField; label: string; value: string }
```

- [ ] **Шаг 1. Экран.** Шапка «Анкета» + `BackButton` Telegram (как в `OnboardingFlow`). Карточка 1: `Avatar` 72 (`profile.photo_url` — своё фото всегда), «{name}, {age}», кнопка «Заменить фото» (`<input type="file" accept="image/*">` скрытый, `uploadPhoto(file)` → `onProfile`), при `photo_status === 'pending'` — `LimitChip` «уйдёт на проверку», при `'rejected'` — строка `PHOTO_REJECTED` из `MyProfileScreen` (вынести константы в `format.ts` или экспортировать); «О себе»: строка-превью (`about` или «Не заполнено», `{len}/200`), тап → `AboutSheet` (textarea, счётчик, подпись из §4 «Диета, религия, температура в комнате — если важно, напиши здесь. Пройдёт проверку вместе с фото», кнопка «Сохранить» → `patchProfile({about})`). Карточка 2 «Анкета»: строки `EditRow` в порядке: Возраст, Пол, С кем готов жить, затем `QUESTION_STEPS[0..2]` (занятость, активен, курение), Свои животные, Чужие животные, `QUESTION_STEPS[3..]`; значение — лейбл выбранного варианта (`choices.find(c => c.id === value)?.label ?? 'Не указано'`); тап → `ChoiceSheet` (заголовок — `title` вопроса дословно, подпись — `why`, варианты как `ChoiceStep`, выбор → `patchProfile({[field]: id})` сразу, шторка закрывается по ответу сервера); Возраст → `AgeSheet` (числовое поле, 16–100, «Сохранить»). Пол/предпочтение — `ChoiceSheet` с вариантами из `BasicsStep` (лейблы дословно те же) и подписью «Это взаимно: если ты „только свой пол", другой пол тебя не увидит нигде» у предпочтения. Имя — строка без стрелки с подписью «из Telegram». Ошибка запроса — текст в шторке, значение не меняется.

- [ ] **Шаг 2. Отправка на проверку.** Внизу экрана, когда `(profile.status === 'active' || profile.status === 'draft') && (profile.photo_status === 'pending' || profile.about_status === 'pending')`: `S.filledButton` «Отправить на проверку» → `submitProfile()` → `onProfile`; подпись `S.caption` «Фото и „о себе" уйдут модератору вместе; привычки уже сохранены». У `status === 'pending'` кнопки нет.

- [ ] **Шаг 3. Оболочка.** `App.tsx`: условие `OnboardingFlow` → только `profile.status === 'draft' && !rejected || finishing`; `view === 'editProfile'` → `ProfileEditScreen` с `autoOpen` из состояния `editTarget: 'photo' | 'about' | null`; `openEditor(target?)` ставит `editTarget`; `fixProfile` → `openEditor(target === 'draft' ? undefined : target)`; диплинки `startTarget.kind === 'photo' | 'about'` → `openEditor(kind)`. `editStep`, `PHOTO_STEP`/`ABOUT_STEP` из App убрать; `OnboardingFlow.initialStep` удалить вместе с экспортами, если потребителей не осталось. `onBack` → `setView('tab')` (таб «Профиль»), `fetchMe` для свежего статуса.

- [ ] **Шаг 4.** Верификация фронта. Ручная приёмка на стенде: сменить «Гости» → в пуле у второго пользователя строка привычек меняется без модерации и без карточки в модер-чат; заменить фото → метка «уйдёт на проверку», кнопка «Отправить на проверку» → карточка в модер-чат одна; пуш «Фото не подошло» (`startapp=photo`) открывает экран с шторкой фото. Коммит «Шаг 10: экран анкеты», push.

---

### Task 6: Спека и деплой (контроллер)

- [ ] CLAUDE.md: правки по списку спеки «Спека (CLAUDE.md)» — §2.1 (наборы, «краем» только у цены), §2.3 (мэтч удаляется; сгорание лайков только при удалении мэтча и блоке), §3 (параметры поиска), §5 (блок стирает мэтч и лайки; «Контакт недоступен» — только бан), §6 (форма поиска — чипы-наборы; мэтч — строка действий; профиль свой — «Редактировать» → экран анкеты списком, описание экрана), §9 (формулировка «ОТМЕНЕНО»), §11 (`searches(rooms int[], terms text[], move_in_months date[])`, `matches` удаляется с лайками пары), §12 (шаг 10 сделано).
- [ ] README/`operating-prod` — ничего не меняется (миграцию применит api на деплое; `DELETE /matches` — в README раздел API, если он там ведётся).
- [ ] Коммит «Шаг 10: спека», push, `scripts/deploy.sh`, приёмка на проде по спеке (форма поиска, пул, экран анкеты, удаление мэтча на двух аккаунтах).
