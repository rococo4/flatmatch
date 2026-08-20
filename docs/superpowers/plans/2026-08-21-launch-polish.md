# Шаг 8 — блок, жалоба, src-трекинг, пустые состояния — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Задачи трекаются чекбоксами.

**Goal:** Всё, без чего нельзя лить рекламу: трекинг источника, блок и жалоба (тем же ботом в модер-чат), пустые состояния с топом живых зон, экран «к себе» по диплинку, мелкая полировка, скрипты деплоя/бэкапа, спека → посев.

**Architecture:** Go-монолит `api/` (handler → service → storage, pgx, goose, chi), React+TS `web/`, Python `parser/`. Спека — `docs/superpowers/specs/2026-08-21-launch-polish-design.md` (истина при конфликте с планом; CLAUDE.md на диске — истина продукта, снапшот в контексте сабагента может быть устаревшим).

**Tech Stack:** Go 1.26, pgx v5, goose, chi; React 19 + TS + Vite; Telegram Bot API (inline callback кнопки).

**Spec:** docs/superpowers/specs/2026-08-21-launch-polish-design.md

## Global Constraints

- Go и SQL — **ноль комментариев** (только `//go:`, `//nolint`, `-- +goose`). Порядок в Go-файле: константы и типы наверху, функции ниже, публичные раньше приватных. Интерфейсы — на стороне потребителя. Фронт — комментарии в существующем стиле.
- Русская микрокопия — ДОСЛОВНО из спеки (таблица «Решения», §3, §4).
- visible_people — единственная точка видимости; блок проходит через неё, отдельных проверок не плодить.
- Верификация Go: `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...` (локальный Postgres поднят: `docker compose up -d postgres` из корня). Миграции к дев-БД: `cd api && go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$TEST_DATABASE_URL" up`. Фронт: `cd web && npx tsc -b && npm run -s lint && npm run build`. Тесты репо — общая дев-БД: чистить только свои строки, не трогать users tg_id=188993596.
- Коммиты в main (короткое русское сообщение + trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`), без push. Прод НЕ деплоить из задач — это делает контроллер скриптом.

---

### Task 1: Миграция 0010, источник, блок, жалоба — бэкенд

**Files:**
- Create: `api/migrations/0010_launch.sql` (SQL дословно из спеки §1)
- Modify: `api/internal/entity/user.go` (+`Source string`), `api/internal/entity/report.go` (новый: `Report{ID, ReporterID, TargetID uuid.UUID; Reason ReportReason; ContextKey string; CreatedAt; ResolvedAt *time.Time; Verdict *ReportVerdict}`, `ReportReason` spam/scam/abuse/other + Parse, `ReportVerdict` banned/skipped, `ReportReasonLabel()` → «Спам/реклама»/«Мошенничество»/«Оскорбления/угрозы»/«Другое»), `api/internal/entity/errors.go` (+`ErrSelfTarget`), `api/internal/repo/usersrepo/{model,postgres}.go` (колонка `source` в columns/row/toEntity; `SetSourceIfEmpty(ctx, id, source) error`; `Block(ctx, blockerID, blockedID) error` — `INSERT INTO blocks ... ON CONFLICT DO NOTHING`), новый `api/internal/repo/reportsrepo/{repo,postgres}.go` (`Insert(ctx, r entity.Report) (inserted bool, err)` по UNIQUE(reporter,target) `ON CONFLICT DO NOTHING RETURNING id`; `ByID`; `Resolve(ctx, id, verdict) (bool, error)` — только где `resolved_at IS NULL`; `Unresolved(ctx) ([]entity.Report, error)`), `api/internal/service/users/service.go` (+`SetSource(ctx, id, raw string) error` с валидацией `^[a-z0-9_]{1,32}$` → `entity.FieldsError{Fields:["source"]}`), новый `api/internal/service/people/service.go` ИЛИ расширение существующего сервиса профиля (посмотреть, что обслуживает `peopleapi` — там `Matches`/`dir`; добавить `Block(ctx, viewerID, targetID)` и `Report(ctx, viewer entity.User, targetID, reason entity.ReportReason, ctxKey string)` в тот сервис, который peopleapi уже использует; `Report`: цель должна быть видима или быть в мэтче (VisibleByID либо мэтч) — иначе 404; `targetID == viewer.ID` → `ErrSelfTarget` → 400), `api/internal/api/http/usersapi/handler.go` (`POST /me/source`), `api/internal/api/http/peopleapi/handler.go` (`POST /{id}/block`, `POST /{id}/report`; DTO `reportRequest{Reason string; Context *struct{ZoneID, PointID, MatchID string}}` → context_key `zone:<id>`/`point:<id>`/`match:<id>`), `api/cmd/api/main.go` (wiring), тесты: usersrepo (source первое касание; Block идемпотентен и VisibleByID после блока → ErrNotFound), reportsrepo (Insert/дубль/Resolve), service (self → ErrSelfTarget; невидимый → ErrNotFound), handler (коды: 204/400/404).

**Interfaces (Produces):**
```go
// peopleapi Service interface +=
Block(ctx, viewerID, targetID uuid.UUID) error
Report(ctx, viewer entity.User, targetID uuid.UUID, reason entity.ReportReason, contextKey string) error
// service.Report после вставки зовёт (best-effort, context.WithoutCancel):
type Moderation interface { ReportFiled(ctx context.Context, r entity.Report, reporter, target entity.User) }
```
`Moderation.ReportFiled` в этой задаче — интерфейс на стороне потребителя + фейк в тестах; реализация в moderation — Task 2 (но чтобы main.go собирался, в Task 1 временно передавать `nil` допустимо ТОЛЬКО если сервис проверяет `s.moderation != nil`; предпочтительно — сделать Task 1 и Task 2 одним реплеем, если имплементер видит, что так проще: тогда объединить).

- [ ] Тесты → реализация → миграция на дев-БД → верификация → коммит «Шаг 8: источник, блок, жалоба — бэкенд».

### Task 2: Модерация жалоб в боте

**Files:**
- Modify: `api/internal/service/moderation/service.go` (`ReportFiled(ctx, r, reporter, target)` → `send(subjectReport, r.ID, target.PhotoKey, reportCaption(...), [[«Забанить», mod:r:ban:<id>], [«Пропустить», mod:r:skip:<id>]])`; `SendQueue` досылает `reports.Unresolved`; интерфейсы `Users` += `SetStatus`?/`Ban`, новый `Reports` интерфейс (ByID/Resolve/Unresolved), `Pushes` += `FailAllPending(ctx, userID)`-эквивалент через существующий репо — посмотреть, как `notificationsrepo.FailAllPending` доступен: добавить в moderation интерфейс `Queue{FailAllPending}` и пробросить notificationsRepo в main.go), `api/internal/service/moderation/callbacks.go` (`subjectReport = "r"`, `opBan = "ban"`, `opSkip = "skip"`, `reportVerdict(ctx, op, id)`: resolved → «уже обработано»; ban: target banned → «уже забанен» (и всё равно resolve как banned? — нет: resolve, метка «✓ Забанен» — идемпотентно); иначе `users.SetStatus(banned)` + `queue.FailAllPending(target)` + `reports.Resolve(banned)` → «✓ Забанен»; skip → `reports.Resolve(skipped)` → «✓ Пропущено»), `api/cmd/api/main.go`, тесты `service_test.go`/`callbacks_test.go` (подпись содержит имена, причину-лейбл и контекст; ban идемпотентен; skip; /queue досылает нерешённые).

Подпись (дословно, три строки): `Жалоба: {reporter.Name} → {target.Name} ({target.Age}) · {label}` / `Контекст: {…}` / `О себе цели: {about|—}`. Контекст: `zone:<id>` → «пул {NameGen}»? — использовать `dir.Get(id).NameIn` в форме «пул {name_in}» (например «пул в Хамовниках»); `point:<id>` → «квартира {name_in зоны точки}» (точка через `Points.ByID`); `match:<id>` → «мэтч»; пусто → «—».

- [ ] Тесты → реализация → верификация → коммит «Шаг 8: жалобы в модер-чат».

### Task 3: Топ живых зон

**Files:**
- Modify: `api/internal/repo/searchesrepo/postgres.go` (+`ActiveZonesVisible(ctx, viewerID) ([]ZoneUsers{ZoneID string; UserIDs []uuid.UUID}, error)` — активные поиски людей из `visible_people($1)`, сгруппированные по zone_id поиска), `api/internal/service/zones/` или `api/internal/service/pools/` (+`Top(ctx, viewerID, limit) ([]ZoneTop{Zone zones.Zone; People int}, error)`: для каждой зоны справочника — множество людей, чьи поиски пересекают её (`dir.Intersecting(search.ZoneID)` содержит зону ⇔ пересечение); people = размер множества; сортировка по people desc, затем по имени; `people > 0`; limit ≤ 20), `api/internal/api/http/zonesapi/handler.go` (`GET /top?limit=5` → `{"zones":[{id,name,name_in,people}]}`; лимит default 5, max 20), `api/cmd/api/main.go`, тесты: repo (группировка, невидимые не попадают), service (район считает людей со станций внутри; человек с двумя поисками в пересекающихся зонах считается один раз), handler.

- [ ] Тесты → реализация → верификация → коммит «Шаг 8: топ живых зон».

### Task 4: Фронт — источник, профиль (блок/жалоба), пустые состояния, экран «к себе», paused-подпись

**Files:**
- Modify: `web/src/api/client.ts` (`StartTarget` += `{kind:'source'; code}` для `src_<код>`; `postSource(code)`, `blockPerson(id)`, `reportPerson(id, reason, context?)`, `fetchTopZones(limit)`; типы `ReportReason`, `TopZoneDTO`), `web/src/App.tsx` (обработка `source`: после ready → `postSource` best-effort, startTarget=null; onBlock/onReport пробросы в PersonProfileView; после блока — назад + `reloadLikes`/`refreshFeed`/перечитать пул; маршрут `point_<id>` для intent_home → `view='homePoint'`; ping-кнопка не требует фронта), `web/src/screens/PersonProfileScreen.tsx` + `PersonProfileView.tsx` (строка действий «Пожаловаться · Заблокировать»; шторки: блок — «Заблокировать {имя}? Вы перестанете видеть друг друга везде — в пулах, лайках и пушах. Отменить нельзя.» → красная «Заблокировать» / «Отмена»; жалоба — чипы «Спам/реклама», «Мошенничество», «Оскорбления/угрозы», «Другое» + «Отправить» → «Спасибо, разберёмся» + «Заблокировать {имя}» + «Готово»; шторки — в `EdgeStates.tsx` по образцу `LeavePoolSheet`), `web/src/screens/FeedScreen.tsx` (пустая выдача: `points.length === 0 && promo.people === 0` → блок «Пусто — растём из района в район» + промо как есть + «Где уже ищут» со строками `{name} · {people}` из `fetchTopZones(5)`, тап → `onPickZone(zoneId)` → `openLookForm({...EMPTY_DRAFT, zoneId})`; топ пуст → только промо; карточку «к себе» вынести в `HomePointCard` компонент), новый `web/src/screens/HomePointView.tsx` (загрузка `fetchPoint`, рендер `HomePointCard`, тот же лайк-поток, что в ленте — посмотреть, как FeedScreen зовёт onRespondHome/`respondedPoints`/`matchedAuthors`, и переиспользовать через пропсы из App), `web/src/screens/MyProfileScreen.tsx` или где «Мои мэтчи» (пустой текст «Мэтчей пока нет — ответный лайк на входящий даёт первый»; «Тебя лайкнули» уже имеет пустое состояние — оставить), `web/src/screens/FlatPoolScreen.tsx` (не-участник на paused: «в пуле N · вход закрыт, пока квартиру не подтвердят» вместо hint «увидишь после входа»), `web/src/api/format.ts` (лейблы причин), при необходимости `web/src/screens/EdgeStates.tsx`.
- Бэкенд для «Открыть» у пинга комнаты: `api/internal/service/notifications/callbacks.go` `pointPingButtons`: intent_home → `linkPointPrefix+id` вместо `linkMine` + правка теста worker_test (ожидание `point_<id>`). (Маленькая Go-правка в этой же задаче, чтобы кнопка и экран появились вместе.)

- [ ] Верификация фронта + `go test ./internal/service/notifications/` → коммит «Шаг 8: фронт — блок, жалоба, пустые состояния, экран „к себе"».

### Task 5: Каналы волны 2, скрипты, спека

**Files:**
- Modify: `parser/config.py` (DEFAULT_CHANNELS += `keystomoscow`, `lvngrm_msk` — ТОЛЬКО после ручной проверки `curl -s https://t.me/s/<канал> | grep -c tgme_widget_message` > 0 для каждого; не отдаёт превью — не добавлять и написать в отчёт), `parser/README.md` (список каналов), новые `scripts/deploy.sh`, `scripts/backup.sh` (по спеке §5; `deploy.sh` — bash, `set -euo pipefail`, хост-алиас `flatmatch` по умолчанию, `rsync` с исключениями из спеки, затем `ssh $host 'cd /opt/flatmatch && docker compose up -d --build && docker image prune -f && docker compose ps'`; `backup.sh` исполняется НА сервере: читает `/opt/flatmatch/.env`, `pg_dump` через `docker compose exec -T postgres`, gzip в `/opt/flatmatch/backups/`, `find -mtime +7 -delete`), `README.md` (раздел деплоя: теперь `scripts/deploy.sh`, бэкапы, где лежит .env, как смотреть логи `ssh flatmatch 'cd /opt/flatmatch && docker compose logs -f api'`), `.claude/skills/running-locally/SKILL.md` (абзац «Прод» со ссылкой на README), `CLAUDE.md` — правки из спеки §«Правки»: §5 (блок/жалоба как построено), §6 (пустая лента — «Где уже ищут»; строка действий профиля; экран «к себе»), §10 (деплой: VPS flatmatch.ru, compose, Caddy; deploy.sh/backup.sh), §11 (users.source, reports), §12 (шаг 8 — сделано), §13 — «Посев (ТЕКУЩАЯ ЗАДАЧА)»: BotFather на домен, src-ссылки по каналам, питч hse.tndr, первые посты в «ищу соседа»-чатах и пабликах ВШЭ, ежедневный SELECT по users.source.

- [ ] `bash -n` обоих скриптов; pytest парсера (конфиг); полный `go test ./...`; фронт-сборка → коммит «Шаг 8: каналы, деплой-скрипты, спека».

### Task 6: Выкатка и приёмка на проде (контроллер)

- [ ] `scripts/deploy.sh` → прод обновлён; `backup.sh` + cron на сервере; DNS/HTTPS живы.
- [ ] Приёмка по спеке §6: `src_test` → users.source; блок; жалоба → карточка → пропустить/забанить на сиде; пустая зона — топ-5; диплинк «к себе».
- [ ] Арсению: BotFather → `https://flatmatch.ru`; онбординг на проде; одобрение анкеты.

## Self-review

- Покрытие спеки: §1 — T1; §2 API — T1 (source/block/report), T3 (zones/top); §3 модерация — T2; §4 фронт — T4 (вкл. кнопка пинга комнаты); §5 скрипты — T5; §6 приёмка — T6; правки CLAUDE.md — T5.
- Сквозные имена: `SetSourceIfEmpty`, `Block`, `reportsrepo.{Insert,ByID,Resolve,Unresolved}`, `ReportFiled`, `subjectReport="r"`/`ban`/`skip`, `ActiveZonesVisible`, `Top`, `/zones/top`, DTO `{id,name,name_in,people}`, `StartTarget{kind:'source'}`, `postSource/blockPerson/reportPerson/fetchTopZones`, `HomePointView`/`HomePointCard`.
- Плейсхолдеров нет; микрокопия — из спеки.
