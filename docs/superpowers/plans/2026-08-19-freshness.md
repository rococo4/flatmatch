# Свежесть квартир (§12 шаг 7a) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Задачи трекаются чекбоксами.

**Goal:** Краудсорс-свежесть квартир с рынка: голоса «живая/сдана» после перехода по ссылке, порог скрытия, «Проверил — она живая» у создателя, пуш пулу «похоже, сдана», еженедельный пинг создателю (user_link и «к себе») с ответами в боте, TTL-архив, строка свежести на карточке и статусы в «Моих поисках».

**Architecture:** Голоса — таблица `point_votes`, порог считается в транзакции с локом строки точки (repo), сервис points после коммита зовёт пуш; пинг и архив — часовой скан существующего воркера notifications; колбэки `pnt:` рядом с `srch:`/`poll:`. Спека — `docs/superpowers/specs/2026-08-19-freshness-design.md` (истина при конфликте с планом; CLAUDE.md на диске — истина продукта; снапшот CLAUDE.md в контексте сабагента может быть устаревшим — читать с диска).

**Tech Stack:** Go 1.26, pgx v5, goose, chi; React+TS+Vite; Telegram Bot API (callback_query).

**Spec:** docs/superpowers/specs/2026-08-19-freshness-design.md

## Global Constraints

- **Комментариев в Go и SQL НЕТ ВООБЩЕ** (ни doc-комментов, ни «почему»); исключения — только `//go:`, `//nolint`, `-- +goose`. Порядок объявлений в Go-файле: константы и типы наверху, функции ниже, публичные раньше приватных. Фронт (TS/TSX) — комментарии допустимы, как в соседнем коде.
- Русская микрокопия — ДОСЛОВНО из спеки §4 и §6.
- Интерфейсы на стороне потребителя; вызовы пушей — best-effort после коммита (`context.WithoutCancel`, warn в лог), как в points/notifications сейчас.
- Каждая задача: тесты → реализация → `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...` (фронт: `cd web && npx tsc -b && npm run -s lint && npm run build`) → коммит в main (короткое русское сообщение + trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`), без push.
- Строку users tg_id=188993596 в БД не трогать. Тесты репо ходят в общую дев-БД: чистить только свои строки (по `testZone` / `testTGBase`, как в соседних тестах) и не полагаться на пустоту чужих таблиц (фильтровать выборки по своим id).
- Стенд: api поднят через `nohup go run ./cmd/api > ../.dev/api.log` — после последней задачи перезапустить (`pkill -f "exe/api"; pkill -f "go run ./cmd/api"` из каталога api, затем nohup снова).

---

### Task 1: Миграция 0008, entity, колонки репо

**Files:**
- Create: `api/migrations/0008_freshness.sql`
- Modify: `api/internal/entity/point.go` (+ `point_test.go` новый), `api/internal/entity/notification.go`, `api/internal/entity/errors.go`, `api/internal/repo/pointsrepo/model.go`, `api/internal/repo/pointsrepo/postgres.go` (только чтение новых колонок), `api/internal/repo/notificationsrepo/model.go` (нет правок, если personalTypes читает entity)

**Миграция** (дословно по спеке §1; плюс расширение CHECK типов пушей — он перечислен в 0007):
```sql
-- +goose Up
ALTER TABLE points
    ADD COLUMN last_seen_alive timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN gone_count      smallint    NOT NULL DEFAULT 0,
    ADD COLUMN ping_asked_at   timestamptz NULL;
UPDATE points SET last_seen_alive = created_at;

CREATE TABLE point_votes (
    user_id    uuid NOT NULL REFERENCES users (id),
    point_id   uuid NOT NULL REFERENCES points (id),
    alive      boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, point_id)
);
CREATE INDEX point_votes_gone_idx ON point_votes (point_id) WHERE NOT alive;

ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK (type IN (
    'zone_new', 'pool_new', 'liked', 'matched',
    'profile_ok', 'photo_rejected', 'about_rejected',
    'point_ok', 'point_rejected',
    'still_searching', 'post_match',
    'point_gone', 'point_ping'));

-- +goose Down
ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK (type IN (
    'zone_new', 'pool_new', 'liked', 'matched',
    'profile_ok', 'photo_rejected', 'about_rejected',
    'point_ok', 'point_rejected',
    'still_searching', 'post_match'));
DROP TABLE point_votes;
ALTER TABLE points DROP COLUMN ping_asked_at, DROP COLUMN gone_count, DROP COLUMN last_seen_alive;
```
Имя constraint проверить в БД: `docker compose exec -T postgres psql -U flatmatch -d flatmatch -Atc "\d notifications"` (postgres именует inline CHECK как `notifications_type_check`; если иначе — подставить реальное).

**Produces (entity):**
```go
// point.go
type PointCloseReason string
const (
	CloseReasonClosed   PointCloseReason = "closed"
	CloseReasonRejected PointCloseReason = "rejected"
	CloseReasonGone     PointCloseReason = "gone"
	CloseReasonExpired  PointCloseReason = "expired"
	CloseReasonFound    PointCloseReason = "found"
)
const (
	GoneVoteFloor  = 2
	GoneVoteShare  = 0.3
	PingEveryDays  = 7
	PointTTLDays   = 14
	PingAnswerDays = 3
)
// Point получает поля: LastSeenAlive time.Time; GoneCount int; PingAskedAt *time.Time
func GoneThreshold(pool int) int  // max(GoneVoteFloor, ceil(GoneVoteShare*pool))
func (p Point) Votable() bool     // Type == PointFlat && URLNorm != nil
// errors.go
var ErrPointNotActive = errors.New("point is not active")
var ErrPointNotPaused = errors.New("point is not paused")
var ErrNotVotable = errors.New("only market flats with a link take votes")
// notification.go
NotifPointGone NotificationType = "point_gone"
NotifPointPing NotificationType = "point_ping"
// ParseNotificationType знает оба; personalPushTypes += NotifPointGone (point_ping — нет)
```
**Produces (pointsrepo):** `pointColumns`/`joinedPointColumns` + `last_seen_alive, gone_count, ping_asked_at`; `pointRow` + поля; `pointRow.toEntity` их маппит. `feedRow` + `GoneVotes int db:"gone_votes"`, `MyVote *bool db:"my_vote"`; `FeedPoint` + `GoneVotes int; MyVote *bool`. В `feed()` в SELECT добавить:
```sql
(SELECT count(*) FROM point_votes v WHERE v.point_id = p.id AND NOT v.alive) AS gone_votes,
(SELECT v.alive FROM point_votes v WHERE v.point_id = p.id AND v.user_id = $1) AS my_vote
```
Тест-хелпер `cleanTestUsers` в pointsrepo/repo_test.go: добавить `DELETE FROM point_votes WHERE point_id IN (SELECT id FROM points WHERE zone_id = $1)` и по created_by тест-юзеров ПЕРЕД удалением points.

- [ ] Тесты: `entity/point_test.go` — таблица `GoneThreshold`: 0→2, 1→2, 2→2, 3→2, 4→2, 5→2, 6→2, 7→3, 10→3, 20→6; `Votable` (flat с url — да, flat без url — нет, intent_home — нет). pointsrepo: `TestCreateRoundtrip` дополнить проверкой `LastSeenAlive` не нулевой и `GoneCount == 0`; `CardByID` отдаёт `GoneVotes 0`, `MyVote nil` у свежей точки.
- [ ] Verify + коммит «Свежесть: схема, entity, колонки».

### Task 2: Репо — голоса, восстановление, закрытия

**Files:**
- Modify: `api/internal/repo/pointsrepo/postgres.go`, `api/internal/repo/pointsrepo/model.go`, `api/internal/repo/pointsrepo/repo_test.go`

**Produces:**
```go
type VoteOutcome struct {
	GoneVotes int
	Hidden    bool   // порог достигнут этим голосом: точка ушла в paused или closed
	Closed    bool   // финально закрыта (tg_channel/manual или второе скрытие user_link)
}
func (r *Repo) Vote(ctx, userID, pointID uuid.UUID, alive bool) (VoteOutcome, error)
func (r *Repo) Restore(ctx, userID, pointID uuid.UUID) error
func (r *Repo) TouchAlive(ctx, pointID uuid.UUID) error
func (r *Repo) CloseWithReason(ctx, pointID uuid.UUID, reason entity.PointCloseReason) (bool, error)
func (r *Repo) AllPoolMemberIDs(ctx, pointID uuid.UUID) ([]uuid.UUID, error)
```
Логика `Vote` — одна транзакция:
1. `SELECT status, source, gone_count FROM points WHERE id = $1 FOR UPDATE` → нет строки → `entity.ErrNotFound`; `status <> 'active'` → `entity.ErrPointNotActive`.
2. `INSERT INTO point_votes (user_id, point_id, alive) VALUES ($1, $2, $3) ON CONFLICT (user_id, point_id) DO UPDATE SET alive = EXCLUDED.alive, created_at = now()`.
3. `alive` → `UPDATE points SET last_seen_alive = now(), ping_asked_at = NULL WHERE id = $1`; вернуть `{GoneVotes: count(NOT alive)}`.
4. иначе `gone = count(*) FROM point_votes WHERE point_id = $1 AND NOT alive`, `pool = count(*) FROM likes_points WHERE point_id = $1`; `gone < entity.GoneThreshold(pool)` → вернуть `{GoneVotes: gone}`.
5. `source = 'user_link' AND gone_count = 0` → `UPDATE points SET status = 'paused', gone_count = 1, edited_at = now()` → `{gone, Hidden: true}`; иначе `UPDATE points SET status = 'closed', close_reason = 'gone', gone_count = gone_count + 1, edited_at = now()` → `{gone, Hidden: true, Closed: true}`.

`Restore`: в транзакции `UPDATE points SET status = 'active', last_seen_alive = now(), ping_asked_at = NULL, edited_at = now() WHERE id = $1 AND created_by = $2 AND status = 'paused'`; 0 строк → если `EXISTS (id=$1 AND created_by=$2)` → `entity.ErrPointNotPaused`, иначе `entity.ErrNotFound`; затем `DELETE FROM point_votes WHERE point_id = $1`. `gone_count` не трогать.

`TouchAlive`: `UPDATE points SET last_seen_alive = now(), ping_asked_at = NULL WHERE id = $1 AND status = 'active'` (без ошибки, если 0 строк).

`CloseWithReason`: `UPDATE points SET status = 'closed', close_reason = $2, edited_at = now() WHERE id = $1 AND status IN ('active','paused')` → `RowsAffected() > 0`.

`AllPoolMemberIDs`: `SELECT user_id FROM likes_points WHERE point_id = $1 ORDER BY created_at, user_id` (без visible_people — объект общий).

Правки существующего:
- `Mine`: `entered` — `p.status IN ('active','paused')` вместо `= 'active'`; `created` — `(p.status <> 'closed' OR p.close_reason IN ('rejected','gone','expired','found'))`.
- `Close` (ручное «Скрыть/Закрыть» создателя): условие `AND (status <> 'closed' OR close_reason <> 'closed')` вместо `= 'rejected'` — закрытую системой строку создатель может убрать из списка.

- [ ] Тесты (все с `cleanTestUsers`, свой `testZone`; голосующие — `seedUser(t, pool, n)`): (a) `TestVoteUpsertsAndRefreshesAlive` — голос «да» двигает `last_seen_alive` вперёд и оставляет active; повторный голос того же юзера не плодит строку (count по point_votes = 1); (b) `TestVotesHideAUserLinkFlatOnce` — user_link flat, создатель + 2 членa пула (пул 3 → порог 2): первый «сдана» → `Hidden=false`, второй → `Hidden=true, Closed=false`, статус paused, gone_count 1; третий голос → `ErrPointNotActive`; (c) `TestSecondHidingIsFinal` — после Restore (голоса стёрты, статус active, gone_count 1) два новых «сдана» → `Closed=true`, статус closed, close_reason gone, gone_count 2; (d) `TestChannelFlatClosesAtOnce` — tg_channel без создателя, 2 голоса «сдана» при пуле 0..2 → closed/gone; (e) `TestRestoreNeedsPaused` — на active → `ErrPointNotPaused`, чужой → `ErrNotFound`; (f) `TestCloseWithReasonOnlyLive` — на closed → false; (g) `TestMineKeepsPausedAndSystemClosed` — вошедший видит paused-точку в `entered`, создатель видит closed/expired в `created`, а closed/closed — нет; (h) `TestTouchAliveClearsPing` — после `UPDATE points SET ping_asked_at = now()` TouchAlive обнуляет и двигает last_seen_alive.
- [ ] Verify + коммит «Свежесть: голоса, порог, восстановление — репо».

### Task 3: Репо — пинг и архив

**Files:**
- Modify: `api/internal/repo/pointsrepo/postgres.go`, новый `api/internal/repo/pointsrepo/ttl_test.go`

**Produces:**
```go
func (r *Repo) PointsForPing(ctx, every time.Duration) ([]entity.Point, error)
func (r *Repo) MarkPingAsked(ctx, pointID uuid.UUID) error
func (r *Repo) Expire(ctx, quiet, afterPing time.Duration) ([]uuid.UUID, error)
```
SQL:
```sql
-- PointsForPing
SELECT <pointColumns> FROM points
WHERE status = 'active' AND created_by IS NOT NULL
  AND (source = 'user_link' OR type = 'intent_home')
  AND last_seen_alive < now() - make_interval(secs => $1)
  AND (ping_asked_at IS NULL OR ping_asked_at < now() - make_interval(secs => $1))
ORDER BY last_seen_alive, id
-- MarkPingAsked
UPDATE points SET ping_asked_at = now() WHERE id = $1
-- Expire
UPDATE points SET status = 'closed', close_reason = 'expired', edited_at = now()
WHERE status IN ('active', 'paused')
  AND (last_seen_alive < now() - make_interval(secs => $1)
       OR (ping_asked_at IS NOT NULL
           AND ping_asked_at < now() - make_interval(secs => $2)
           AND last_seen_alive < ping_asked_at
           AND EXISTS (SELECT 1 FROM notifications n
                       WHERE n.type = 'point_ping'
                         AND n.context_key = points.id::text
                         AND n.sent_at IS NOT NULL
                         AND n.sent_at >= points.ping_asked_at - interval '1 minute')))
RETURNING id
```
- [ ] Тесты (`ttl_test.go`; время подкручивать `UPDATE points SET last_seen_alive = now() - interval '8 days'` и т.п.; в `Expire` фильтровать результат по своим id — в дев-БД могут архивироваться чужие): свежая user_link не в PointsForPing; старше 7 дней — в списке; после MarkPingAsked — не в списке; intent_home старше 7 дней — в списке; tg_channel старше 7 дней — НЕТ (создателя нет); Expire: 15 дней без подтверждений → closed/expired (и active, и paused); 8 дней + ping_asked_at 4 дня назад без отправленного пуша → НЕ архив; то же с вставленной строкой notifications (type point_ping, context_key = id, sent_at = now()-3d, user_id = создатель) → архив; pending-точка старше 15 дней — не тронута.
- [ ] Verify + коммит «Свежесть: пинг и архив — репо».

### Task 4: Сервис points — Vote/Restore + пуш «похоже, сдана»

**Files:**
- Modify: `api/internal/service/points/service.go`, `api/internal/service/points/service_test.go` (fakeRepo + новые тесты), `api/internal/service/points/pushes_test.go` (fakePushes)

**Produces:**
```go
// Repository += 
Vote(ctx, userID, pointID uuid.UUID, alive bool) (pointsrepo.VoteOutcome, error)
Restore(ctx, userID, pointID uuid.UUID) error
// Pushes +=
PointGone(ctx context.Context, point entity.Point, lastVoter uuid.UUID)
// Service
func (s *Service) Vote(ctx, viewerID, pointID uuid.UUID, alive bool) (pointsrepo.FeedPoint, error)
func (s *Service) Restore(ctx, userID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
```
`Vote`: `card, err := s.repo.CardByID(ctx, viewerID, pointID)` (404 если не видна) → `!card.Point.Votable()` → `entity.ErrNotVotable` → `card.Point.Status != PointActive` → `entity.ErrPointNotActive` (ранний ответ; репо повторит под локом) → `out, err := s.repo.Vote(...)` → перечитать `CardByID` → если `out.Hidden && s.pushes != nil` → `s.pushes.PointGone(ctx, card.Point, viewerID)` (card — уже перечитанная, с новым статусом) → вернуть карточку.
`Restore`: `s.repo.Restore` → `CardByID` → вернуть.
`Point()`: неактивную видит не только создатель — `paused` открыта всем, кто видит карточку: условие `Status != PointActive && Status != PointPaused && (CreatedBy == nil || *CreatedBy != viewerID)` → 404.

- [ ] Тесты (fakeRepo с полем `card pointsrepo.FeedPoint`, `voteOut pointsrepo.VoteOutcome`, счётчиками вызовов): intent_home → `ErrNotVotable`; flat без url → `ErrNotVotable`; paused → `ErrPointNotActive` без вызова repo.Vote; `Hidden=false` → PointGone не зван; `Hidden=true` → зван один раз с id голосовавшего; Restore возвращает перечитанную карточку; `Point()` отдаёт paused чужому, но не pending чужому.
- [ ] Verify + коммит «Свежесть: голоса и восстановление — сервис».

### Task 5: Сервис notifications — фан-аут point_gone, тексты, сообщения воркера

**Files:**
- Modify: `api/internal/service/notifications/service.go` (интерфейс `Points` += `AllPoolMemberIDs`, `ByID`), `api/internal/service/notifications/fanout.go` (+`PointGone`), `api/internal/service/notifications/texts.go` (+тексты), `api/internal/service/notifications/worker.go` (кейсы `personalMessage`), `api/internal/service/notifications/callbacks.go` (кнопки пинга — только сборка кнопок; обработчик — Task 7), тесты `service_test.go`, `texts_test.go`, `worker_test.go`

**Produces:**
```go
// service.go
type Points interface {
	PoolMemberIDs(ctx, viewerID, pointID uuid.UUID) ([]uuid.UUID, error)
	AllPoolMemberIDs(ctx, pointID uuid.UUID) ([]uuid.UUID, error)
}
const payloadOwn = "own"
// fanout.go
func (s *Service) PointGone(ctx, point entity.Point, lastVoter uuid.UUID)
//   members := AllPoolMemberIDs; для каждого m != lastVoter:
//   Notification{UserID: m, Type: NotifPointGone, ActorKey: fmt.Sprintf("%s:%d", point.ID, point.GoneCount),
//                ContextKey: point.ID.String(), Payload: {payloadOwn: point.CreatedBy != nil && *point.CreatedBy == m}}
//   → s.many. Создатель, если он не в likes_points (вышел из своего пула), строки не получает — так и задумано: пул его не касается.
// texts.go
func PointGoneText(nameIn string) string      // "Квартира " + nameIn + ", похоже, сдана"
func PointGoneOwnText(nameIn string) string   // "Твою квартиру " + nameIn + " отметили как сданную — проверь"
func PointPingText(kind entity.PointType, nameIn string) string
//   flat: "Квартира " + nameIn + " ещё актуальна?"; intent_home: "Комната " + nameIn + " ещё свободна?"
// callbacks.go
const opPointAlive = "alive"; opPointGone = "gone"
func pointPingButtons(kind entity.PointType, pointID uuid.UUID) []tgbot.PushButton
//   flat: {"Да, живая", pnt:alive:<id>}, {"Уже сдана", pnt:gone:<id>}; intent_home: {"Да", ...alive}, {"Нашёл соседа", ...gone}
func pointCallback(op string, pointID uuid.UUID) string   // tgbot.PointCallbackPrefix + ":" + op + ":" + id
// worker.go
const buttonCheck = "Проверить"
// personalMessage:
case entity.NotifPointGone:
	point, zone, err := w.pointContext(ctx, n.ContextKey)   // uuid.Parse → w.points.ByID → w.dir.Get(point.ZoneID); ErrNotFound → errUnknownContext
	if payloadBool(n.Payload, payloadOwn) { return PointGoneOwnText(zone.NameIn), deepLink(buttonCheck, linkMine), nil }
	return PointGoneText(zone.NameIn), deepLink(buttonOpen, linkPointPrefix+n.ContextKey), nil
case entity.NotifPointPing:
	point, zone, err := w.pointContext(ctx, n.ContextKey)
	return PointPingText(point.Type, zone.NameIn), pointPingButtons(point.Type, point.ID), nil
func payloadBool(payload map[string]any, key string) bool
```
`tgbot.PointCallbackPrefix = "pnt"` добавить в `api/internal/tgbot/pushes.go` уже здесь (маршрутизация — Task 7).
Существующий кейс `NotifPoolNew` в `aggregateMessage` можно переписать через `pointContext` — не обязательно.

- [ ] Тесты: `PointGone` — 3 члена, один из них голосовал последним → 2 строки, у создателя `own=true`, у члена `own=false`, actor = `<id>:<gone_count>`; тексты трёх функций (station «у метро Спортивная» и district «в Хамовниках»); worker `personalMessage` для point_gone (own → кнопка «Проверить»/`mine`; чужой → «Открыть»/`point_<id>`), point_ping (flat → 2 callback-кнопки `pnt:alive:<id>`/`pnt:gone:<id>` с текстами «Да, живая»/«Уже сдана»; intent_home → «Да»/«Нашёл соседа»); неизвестная точка → giveUp (failed).
- [ ] Verify + коммит «Свежесть: пуши „похоже, сдана" и пинг — тексты и фан-аут».

### Task 6: Воркер — часовой скан пинга и архива

**Files:**
- Modify: `api/internal/service/notifications/worker.go`, `api/internal/service/notifications/worker_test.go`

**Produces:**
```go
type PointZones interface {
	ByID(ctx, id uuid.UUID) (entity.Point, error)
	PointsForPing(ctx, every time.Duration) ([]entity.Point, error)
	MarkPingAsked(ctx, id uuid.UUID) error
	Expire(ctx, quiet, afterPing time.Duration) ([]uuid.UUID, error)
}
func (w *Worker) pingPoints(ctx)   // для каждой точки: queue.Insert(Notification{UserID: *point.CreatedBy, Type: NotifPointPing,
                                    //   ActorKey: pingActor(point) = fmt.Sprintf("%s:%d", point.ID, point.LastSeenAlive.Unix()),
                                    //   ContextKey: point.ID.String()}) → при ошибке warn и continue; затем MarkPingAsked
func (w *Worker) expirePoints(ctx) // ids := Expire(PointTTLDays*24h, PingAnswerDays*24h); len>0 → log.Info("points expired", "count")
```
В `pass()` после `askPostMatch`: `w.pingPoints(ctx); w.expirePoints(ctx)` — под тем же `answersReachUs`/`scanEvery` гейтом.

- [ ] Тесты (fakePointZones += слайсы `forPing []entity.Point`, `asked []uuid.UUID`, `expired []uuid.UUID`, `expireCalls int`): часовой скан вставляет point_ping создателю с actor `<id>:<unix>` и зовёт MarkPingAsked; точка без CreatedBy пропускается без вставки; Expire зовётся раз в скан; при `WithoutAnswers` ни пинга, ни архива.
- [ ] Verify + коммит «Свежесть: воркер — пинг создателю и архив».

### Task 7: Колбэки `pnt:` + маршрутизация в боте

**Files:**
- Modify: `api/internal/service/notifications/callbacks.go`, `api/internal/service/notifications/callbacks_test.go`, `api/internal/tgbot/bot.go` (интерфейс `Pushes` += `HandlePointCallback`, `case PointCallbackPrefix`), `api/internal/tgbot/bot_test.go` (fake += метод), `api/cmd/api/main.go` (`NewCallbacks(searchesRepo, matchesRepo, usersRepo, pointsRepo)`)

**Produces:**
```go
type PointAnswers interface {
	ByID(ctx, id uuid.UUID) (entity.Point, error)
	TouchAlive(ctx, id uuid.UUID) error
	CloseWithReason(ctx, id uuid.UUID, reason entity.PointCloseReason) (bool, error)
}
func NewCallbacks(searches SearchAnswers, polls PollAnswers, users Accounts, points PointAnswers) *Callbacks
func (c *Callbacks) HandlePointCallback(ctx, tgID int64, data string) (string, error)
const (
	ackPointAlive    = "✓ Отмечено: актуальна"
	ackRoomAlive     = "✓ Отмечено: свободна"
	ackPointClosed   = "✓ Объявление закрыто"
	ackPointInactive = "объявление уже неактивно"
	ackNotYourPoint  = "не твоё объявление"
)
```
Логика: parse (`splitCallback(tgbot.PointCallbackPrefix, data)` → op, id) → `users.ByTelegramID` (ErrNotFound → ackNotYourPoint) → `points.ByID` (ErrNotFound → ackNotYourPoint) → `CreatedBy == nil || *CreatedBy != user.ID` → ackNotYourPoint → по op:
- `alive`: `Status != PointActive` → ackPointInactive; `TouchAlive`; intent_home → ackRoomAlive, иначе ackPointAlive.
- `gone`: reason = `CloseReasonGone`, intent_home → `CloseReasonFound`; `closed, err := CloseWithReason` → `!closed` → ackPointInactive; иначе ackPointClosed.
- иной op → error.
В `bot.go`: `case SearchCallbackPrefix, PollCallbackPrefix, PointCallbackPrefix:` → `handlePushAnswer`; внутри выбор обработчика по префиксу через `strings.Cut(query.Data, ":")` (switch на три префикса вместо двух if).

- [ ] Тесты: чужой tg → «не твоё объявление»; alive на active flat → touch зван, ack «✓ Отмечено: актуальна»; alive на intent_home → «✓ Отмечено: свободна»; alive на closed → «объявление уже неактивно» без touch; gone на flat → CloseWithReason(gone), ack «✓ Объявление закрыто»; gone на intent_home → reason found; gone повторно (fake возвращает false) → «объявление уже неактивно»; bot_test: `pnt:` роутится в HandlePointCallback (по образцу существующего теста для `srch:`).
- [ ] Verify + коммит «Свежесть: ответы на пинг в боте».

### Task 8: HTTP — vote/restore, DTO, статусы; мэтч продлевает «к себе»

**Files:**
- Modify: `api/internal/api/http/pointsapi/handler.go` (интерфейс `Points` += `Vote`, `Restore`; роуты `POST /{id}/vote`, `POST /{id}/restore`), `dto.go`, `errors.go`, `handler_test.go`, `dto_test.go`, `api/internal/httpx/respond.go` (коды), `api/internal/service/likes/service.go` (+`Points` интерфейс, TouchAlive при мэтче с point-контекстом), `api/internal/service/likes/service_test.go`, `api/cmd/api/main.go` (`likes.New(likesRepo, dir, pushes, pointsRepo)`)

**Produces:**
```go
// httpx
CodePointNotActive Code = "point_not_active"
CodePointNotPaused Code = "point_not_paused"
CodeNotVotable     Code = "not_votable"
// pointsapi
type voteRequest struct { Alive *bool `json:"alive"` }   // nil → 400 invalid_field ["alive"]
// Card +=
LastSeenAlive time.Time `json:"last_seen_alive"`
GoneVotes     int       `json:"gone_votes"`
MyVote        *bool     `json:"my_vote,omitempty"`
Restorable    bool      `json:"restorable,omitempty"`   // fp.Point.Status == paused && fp.Point.CreatedBy == viewer — toCard получает viewerID (или Handler.toCard знает user из контекста: добавить параметр viewerID во все вызовы toCard/Cards)
// cardStatus: closed + reason gone → "gone", expired → "expired", found → "found", rejected → "rejected" (как было), иначе string(status)
// handleError +=
ErrPointNotActive → 409 CodePointNotActive "Голосовать можно только по активной квартире"
ErrPointNotPaused → 409 CodePointNotPaused "Квартира и так активна"
ErrNotVotable     → 422 CodeNotVotable "Отметка живости — только у квартир с рынка"
// likes
type Points interface { TouchAlive(ctx, pointID uuid.UUID) error }
func New(repo Repository, dir *zones.Directory, pushes Pushes, points Points) *Service
// announce: res.Match != nil && res.Match.ContextPoint != nil && s.points != nil → s.points.TouchAlive(context.WithoutCancel(ctx), *res.Match.ContextPoint) (ошибка — только warn через slog.Default().Warn или молча игнор — как у соседних best-effort)
```
`vote` handler: user из контекста, id, decode `voteRequest`, `h.points.Vote(ctx, user.ID, id, *body.Alive)` → 200 `h.toCard(card)`. `restore` — без тела → 200 карточка. Ошибки — `handleError`.
`Cards(list, photos, dir)` используется из searchesapi для ленты — там `Restorable` всегда false (лента не показывает paused); проще всего: `toCard(fp, photos, zone, viewerID uuid.UUID)` и `Cards(list, photos, dir, viewerID)`; searchesapi передаёт user.ID.

- [ ] Тесты: `handler_test`: POST vote `{"alive":false}` → 200 с `gone_votes`; без поля → 400 invalid_field; fake возвращает ErrPointNotActive → 409 `point_not_active`; ErrNotVotable → 422; POST restore → 200; ErrPointNotPaused → 409. `dto_test`: `cardStatus` для gone/expired/found; `Restorable` true только у paused своей; JSON содержит `last_seen_alive`. `likes/service_test`: мэтч с ContextPoint → TouchAlive зван с этим id; мэтч с зоной → не зван; nil points → не паникует.
- [ ] Verify + коммит «Свежесть: HTTP голосов и восстановления, статусы карточки».

### Task 9: Фронт — строка свежести, шторка, восстановление, статусы

**Files:**
- Modify: `web/src/api/client.ts`, `web/src/api/format.ts`, `web/src/screens/FlatPoolScreen.tsx`, `web/src/screens/FlatPoolView.tsx`, `web/src/screens/MySearchesScreen.tsx`, `web/src/App.tsx`

**client.ts:**
```ts
export type PointStatus = 'unknown' | 'pending' | 'active' | 'paused' | 'closed' | 'rejected' | 'gone' | 'expired' | 'found';
// PointDTO +=
last_seen_alive: string;      // RFC3339
gone_votes: number;
my_vote?: boolean;
restorable?: boolean;
export function votePoint(id: string, alive: boolean): Promise<PointDTO>   // POST /api/v1/points/{id}/vote {alive}
export function restorePoint(id: string): Promise<PointDTO>                // POST /api/v1/points/{id}/restore
```
**format.ts:**
```ts
POINT_STATUS_LABEL: paused → 'похоже, сдана', gone → 'сдана', expired → 'истёк срок', found → 'нашёл соседа', closed → 'закрыта', rejected → 'отклонена'
export function freshnessLabel(p: Pick<PointDTO,'last_seen_alive'|'created_days_ago'|'gone_votes'|'status'>): string
//   daysSince(last_seen_alive) < created_days_ago (подтверждали после добавления) → 'подтверждена сегодня' | 'подтверждена вчера' | `подтверждена ${n} ${plural(n,'день','дня','дней')} назад`
//   иначе → 'добавлена сегодня, не проверялась' | 'добавлена вчера, не проверялась' | `добавлена ${n} … назад, не проверялась`
//   status === 'active' && gone_votes >= 1 → + ' · кто-то отметил как сданную'
export function goneBanner(goneVotes: number): string   // `похоже, сдана — ${n} ${plural(n,'человек','человека','человек')} ${n===1?'отметил':'отметили'}`
```
`daysSince(iso)` — по календарным дням в локальном времени, как `addedLabel` (точность до дня).

**FlatPoolScreen.tsx** (вёрстка): `FlatPoolCard.addedAt` → переименовать в `freshness: string`; новые пропсы `FlatPoolCard.gone?: { banner: string; restorable: boolean }` (paused), `vote?: { asked: boolean; myVote: boolean | null; busy: boolean }`; колбэки `onVote?: (alive: boolean) => void`, `onRestore?: () => void`, `onChangeVote?: () => void`. Рендер:
- шапка карточки: вместо `flat.addedAt` — `flat.freshness` (та же позиция/стиль).
- если `flat.gone` — под фактами жёлтая плашка (`background: 'color-mix(in srgb, var(--type-home-fg) 12%, var(--card-bg))'` или существующий жёлтый токен вердикта «краем» из VERDICT_UI, посмотреть в primitives) с текстом `gone.banner`; при `restorable` под ней кнопка «Проверил — она живая» (стиль как `moreButton`, но цвет акцентный) → `onRestore`.
- блок-вопрос под кнопкой источника, когда `vote` задан и (`vote.asked` или `vote.myVote !== null`): если `myVote === null` — заголовок «Объявление ещё живое?», две кнопки «Да» / «Уже сдана» (вторая — destructive-цвет), подпись «Помоги остальным в пуле — сданные мы скрываем»; если `myVote !== null` и не `asked` (только что ответил или пришёл с голосом) — строка «Ты отметил: живая · изменить» / «Ты отметил: сдана · изменить», где «изменить» — ссылка → `onChangeVote` (снова показывает кнопки). После ответа в этой же сессии — «Спасибо, отметили · изменить». Кнопки disabled при `vote.busy`.
**FlatPoolView.tsx**: состояние `asked` (после `onOpenSource` → `setAsked(true)`, если `point.type==='flat' && point.url && point.status==='active'`), `voteBusy`, `justVoted`; `vote(alive)` → `votePoint(pointId, alive)` → `setData({...data, point})` (карточка из ответа) → `onChanged?.()` (списки перечитать) → `setJustVoted(true)`; `restore()` → `restorePoint` → та же замена point + `onChanged`. В `toCard(p)` — `freshness: freshnessLabel(p)`, `gone: p.status==='paused' ? { banner: goneBanner(p.gone_votes), restorable: !!p.restorable } : undefined`, `status: pointStatusLabel(p.status) || undefined` (для paused строка статуса не нужна — плашка её заменяет: передавать `status` только если не paused).
**MySearchesScreen.tsx**: `FlatRow` — `StatusRow` уже покажет «похоже, сдана»/«сдана»/«истёк срок»; для своей paused-точки вместо `PointActions` с «Закрыть» — две кнопки: primary-link «Проверил — она живая» → `onRestore?.(id)` и красная «Закрыть» (как сейчас); для своей closed (gone/expired) — `PointActions action="Скрыть"` (как у rejected). `HomeRow` — статусы `found`/`expired` через тот же `StatusRow` (проверить, что HomeRow его рендерит; если нет — добавить) и «Скрыть». Новый проп `onRestorePoint?: (id: string) => void` у `MySearchesScreen`, у вошедших paused-строк — «Выйти из пула» как было.
**App.tsx**: `restoreMyPoint(id)` → `restorePoint(id)` → `reloadPoints(); void refreshFeed()`; ошибки — `setSearchesError`; проп в `MySearchesScreen`; `FlatPoolView` получает `onChanged` (уже есть) — этого достаточно.
`FeedScreen` — без правок (в ленте только active).

- [ ] Verify: `cd web && npx tsc -b && npm run -s lint && npm run build`; коммит «Фронт: свежесть квартир — шторка, статусы, восстановление».

### Task 10: Спека и приёмка

**Files:**
- Modify: `CLAUDE.md` (§7 таблица типов + `point_gone`/`point_ping` с текстами; §8 — механика как реализовано: кто голосует, порог, второе скрытие финальное, «к себе» тоже пингуется, TTL 14д/3д молчания, лейблы статусов; §11 — points.last_seen_alive/gone_count/ping_asked_at, point_votes, словарь close_reason; §12 — 7a сделано, 7b парсер — следующий; §13 — DoD 7b), `.superpowers/sdd/2026-08-19-freshness/progress.md` (леджер задач — создать в Task 1 и вести).

- [ ] Спека правится в тон существующему тексту, без «мы решили» — правила как факт.
- [ ] Прогнать полный `go test ./...` и фронт-сборку; перезапустить api на стенде; проверить `docker compose exec -T postgres psql -U flatmatch -d flatmatch -Atc "SELECT column_name FROM information_schema.columns WHERE table_name='points' AND column_name IN ('last_seen_alive','gone_count','ping_asked_at')"` — 3 строки.
- [ ] Коммит «Спека: шаг 7a закрыт — свежесть квартир».

## Self-review

- Покрытие спеки: §1 схема — Task 1; §2 голоса/порог/restore — Tasks 2, 4, 8; §3 пинг/TTL/колбэки/мэтч → TouchAlive — Tasks 3, 6, 7, 8; §4 тексты и типы — Tasks 1, 5; §5 HTTP — Task 8; §6 фронт — Task 9; §7 тесты — в каждой задаче; правки CLAUDE.md — Task 10.
- Имена сквозные: `VoteOutcome{GoneVotes,Hidden,Closed}`, `Vote/Restore/TouchAlive/CloseWithReason/AllPoolMemberIDs/PointsForPing/MarkPingAsked/Expire`, `PointGone`, `NotifPointGone/NotifPointPing`, `PointCallbackPrefix="pnt"`, `HandlePointCallback`, `CodePointNotActive/CodePointNotPaused/CodeNotVotable`, DTO `last_seen_alive/gone_votes/my_vote/restorable`, статусы `gone/expired/found`.
- Плейсхолдеров нет; микрокопия — из спеки §6 и таблицы §4.
