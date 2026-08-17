# Пуши (§12 шаг 6) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Задачи трекаются чекбоксами.

**Goal:** Уведомления через бота: notifications-лог, событийные пуши с агрегацией зонных/пуловых, доставка воркером с диплинками, ответы кнопками («ещё ищешь?», пост-мэтч), фронт: диплинки + замкнутая петля реджекта фото/«о себе».

**Architecture:** Событие пишется строкой в `notifications` (best-effort после коммита), фоновый воркер в api группирует/шлёт через `tgbot.Client`, колбэки маршрутизируются по префиксу. Спека — `docs/superpowers/specs/2026-08-14-notifications-design.md` (истина при конфликте с планом; CLAUDE.md на диске — истина продукта).

**Tech Stack:** Go 1.26, pgx v5, goose, chi; React+TS+Vite; Telegram Bot API (sendMessage, editMessageText, callback_query).

**Spec:** docs/superpowers/specs/2026-08-14-notifications-design.md

## Global Constraints

- Комментарии в Go — только неочевидные «почему», скупо. Русская микрокопия — ДОСЛОВНО из таблицы §2 спеки (тексты правил пользователь).
- Интерфейсы на стороне потребителя; visible_people — единственная точка видимости; смэтченные исключаются из discovery-фан-аутов.
- Emit — best-effort после коммита (warn, не ошибка наверх), на context.WithoutCancel — как модерация.
- Каждая задача: тесты → реализация → `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...` (фронт: `cd web && npx tsc -b --force && npm run build`) → коммит в main (`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`), без push.
- Аккаунт tg_id=188993596 в БД не трогать (кроме приёмки пушей — только чтение/отправка).

---

### Task 1: Миграция 0007, entity, репо notifications

**Files:**
- Create: `api/migrations/0007_notifications.sql`, `api/internal/entity/notification.go` (+test), `api/internal/repo/notificationsrepo/{repo.go,model.go,postgres.go,repo_test.go}`
- Modify: `api/internal/repo/searchesrepo/postgres.go` (last_active_at/ttl_asked_at в колонках и методы Touch/MarkTTLAsked/StaleForTTL/PauseUnanswered), `api/internal/repo/usersrepo/postgres.go` (MarkPushBlocked)

**Produces:**
```go
// entity
type NotificationType string // NotifZoneNew, NotifPoolNew, NotifLiked, NotifMatched, NotifProfileOK, NotifPhotoRejected, NotifAboutRejected, NotifPointOK, NotifPointRejected, NotifStillSearching, NotifPostMatch (+Parse)
type Notification struct { ID, UserID uuid.UUID; Type NotificationType; ActorKey, ContextKey string; Payload map[string]any; CreatedAt time.Time; SentAt, FailedAt *time.Time; Attempts int }
const MaxPersonalPushesPerDay = 10; AggregateWindow = 24h; TTLQuietDays = 7; TTLAfterMatchDays = 3; TTLAnswerDays = 3; PostMatchDays = 5
// notificationsrepo
Insert(ctx, n entity.Notification) (inserted bool, err)             // ON CONFLICT DO NOTHING по actor-индексу
InsertMany(ctx, ns []entity.Notification) (int, error)              // фан-аут одним батчем
Pending(ctx, limit int) ([]entity.Notification, error)              // sent/failed NULL, по created_at, только адресаты без push_blocked_at и status<>banned
MarkSent(ctx, ids []uuid.UUID, at time.Time) error
MarkFailed(ctx, ids []uuid.UUID) error; Bump(ctx, id) error          // attempts++
LastSentAt(ctx, userID, typ, contextKey) (*time.Time, error)        // окно агрегата
SentPersonalCount(ctx, userID, since time.Time) (int, error)        // предохранитель (типы liked/matched/profile*/photo*/about*/point*)
FailAllPending(ctx, userID) error                                   // бот заблокирован
LastLikedTail(ctx, userID, since) (*TailMessage{ID, MessageID int, Count int}, error) / SetTail(...)  // хвост liked: payload хранит tg message_id и count
// searchesrepo
Touch(ctx, userID) error                                            // last_active_at=now() у активных поисков юзера
StaleForTTL(ctx, quiet, afterMatch time.Duration) ([]entity.Search, error)  // активные, last_active_at старше quiet, ttl_asked_at NULL или старше (quiet), ИЛИ у владельца мэтч свежее last_active_at и старше afterMatch
MarkTTLAsked(ctx, searchID) error
PauseUnanswered(ctx, olderThan time.Duration) ([]uuid.UUID, error)  // ttl_asked_at старше и last_active_at < ttl_asked_at → paused; вернуть id
// usersrepo
MarkPushBlocked(ctx, userID) error
```
match_polls таблица + `matchesrepo.SavePollAnswer(ctx, matchID, userID, answer) (bool changed, err)` и `MatchesForPostPoll(ctx, olderThan) []Match без ответа`.
- [ ] Тесты: дедуп по актору (второй Insert того же (user,type,actor) → false); Pending исключает banned/push_blocked; LastSentAt; SentPersonalCount по типам; StaleForTTL три ветки; PauseUnanswered; SavePollAnswer идемпотентен.
- [ ] Verify + коммит «Пуши: лог, TTL-поля и опрос — схема и репо».

### Task 2: Сервис notifications — Emit и фан-аут

**Files:**
- Create: `api/internal/service/notifications/{service.go,fanout.go,texts.go,service_test.go}` (заменить doc.go содержимым)

**Produces (потребители Task 3–5):**
```go
type Repository interface { Insert; InsertMany }
type Searches interface { ActiveIn(ctx, zoneIDs []string) ([]entity.Search, error) }   // подписчики: user_id+zone_id (новый метод searchesrepo, добавить с тестом)
type Points interface { PoolMemberIDs(ctx, viewerID, pointID) ([]uuid.UUID, error) }   // участники × visible × не смэтчены с viewer'ом=вошедшим (новый метод pointsrepo, реюз PoolMembers-предиката)
type People interface { VisibleBetween(ctx, a, b) (bool, error); IsMatched(ctx, a, b) (bool, error) } // usersrepo/matchesrepo
func New(repo, searches, points, people, dir *zones.Directory, log) *Service
func (s *Service) ZoneNew(ctx, search entity.Search)                 // фан-аут: dir.Intersecting(зона) → ActiveIn → для каждого подписчика (≠автор, VisibleBetween, !IsMatched) → Notification{user=подписчик, type=zone_new, actor=search.UserID, context=подписчик.ZoneID}
func (s *Service) PoolNew(ctx, pointID, enteredUserID uuid.UUID)
func (s *Service) Liked(ctx, toID, fromID uuid.UUID, fromName string, fromGender entity.Gender)
func (s *Service) Matched(ctx, match entity.Match)                    // обе стороны
func (s *Service) ProfileVerdict(ctx, userID, typ NotificationType)   // profile_ok/photo_rejected/about_rejected
func (s *Service) PointVerdict(ctx, point entity.Point, ok bool)
// texts.go — чистые функции текстов ДОСЛОВНО из §2 спеки, с plural: ZoneNewText(nameIn string, n int), PoolNewText(station string, n int), LikedText(name, gender), MatchedText(), ..., StillSearchingText(nameIn), PostMatchText(peerName)
```
Все методы best-effort: ошибка → warn. actor_key = uuid актора (zone_new/pool_new/liked), для matched actor = match.ID, для вердиктов — «<subject-id>:<type>:<created_at unix>» (правки могут повторяться — уникальность на актор не нужна, но индекс требует уникальности: используй время).
- [ ] Тесты (фейки): фан-аут zone_new — автор не получает, смэтченный не получает, невидимый не получает, поиск в непересекающейся зоне не получает, дубль зоны у одного подписчика (2 поиска пересекают) → ОДНА строка на подписчика (context = ближайшая... нет: по строке на КАЖДЫЙ его пересекающий поиск — контекст разный; проверить, что actor-индекс не схлопывает разные context — ЗНАЧИТ actor-индекс должен включать context_key: ПОПРАВЬ Task 1: UNIQUE (user_id, type, actor_key, context_key) с coalesce(context_key,'')); pool_new — вошедший не получает; тексты: таблица склонений (1/2/5/21 человек), род liked.
- [ ] Verify + коммит «Пуши: сервис событий и тексты».

### Task 3: Триггеры в доменных сервисах

**Files:**
- Modify: `api/internal/service/searches/service.go` (+test), `api/internal/service/points/service.go` (+test), `api/internal/service/likes/service.go` (+test — Like: после Create при !Already && !reply → Liked; при res.Match != nil → Matched ОДИН раз (гонка settledMatch: обе стороны видят Match — Matched должен быть идемпотентен по actor=match.ID: индекс это даёт)), `api/internal/service/moderation/callbacks.go` (+test: после вердикта → ProfileVerdict/PointVerdict), `api/internal/api/http/authmw` или users service: Touch раз в час (кэш last touch по user в памяти либо `WHERE last_active_at < now()-1h` в SQL — второе проще, сделай в Touch)
- Потребительские интерфейсы `Notifier` в каждом сервисе (nil-безопасно: nil → молча).
- [ ] Тесты: фейк-Notifier копит вызовы: создание поиска зовёт ZoneNew, правка — НЕ зовёт, реактивация — зовёт; Enter — PoolNew, повторный Enter (Already) — не зовёт (нужен признак из репо: Enter возвращает entered bool — добавь); Like spent → Liked, reply → нет Liked но Matched; вердикты.
- [ ] Verify + коммит «Пуши: триггеры в поисках, точках, лайках и модерации».

### Task 4: Воркер доставки + tgbot

**Files:**
- Create: `api/internal/service/notifications/worker.go`, `worker_test.go`, `api/internal/tgbot/pushes.go` (адаптер Sender: SendPush(ctx, chatID, text, button PushButton) (msgID int, err); EditPushText(ctx, chatID, msgID, text) error; PushButton{Text, StartParam string} | {Text, CallbackData string}...; распознавание ошибок blocked/chat-not-found → tgbot.ErrBlocked)
- Modify: `api/internal/tgbot/client.go` (EditMessageText), `api/internal/tgbot/bot.go` (маршрутизация колбэков по префиксу: mod:/srch:/poll:; StartWorker), `api/cmd/api/main.go`, `api/internal/config` (PUSH_WORKER_ENABLED default true, интервал)
- Воркер: `Run(ctx)` тик 30 с; проход по §4 спеки: личные поштучно с предохранителем (≥10 личных/24ч → liked в хвост через EditPushText по сохранённому message_id; matched/сервисные как есть); агрегаты по (user,type,context) с окном LastSentAt<24ч; раз в час — StaleForTTL → StillSearching-пуш с callback-кнопками, MarkTTLAsked; PauseUnanswered; MatchesForPostPoll → PostMatch с кнопками. Ошибки: ErrBlocked → MarkPushBlocked+FailAllPending; иная → Bump, ≥5 → MarkFailed.
- Диплинк-кнопка: `web_app: {url: APP_URL + "?startapp=" + param}` — В TMA web_app-кнопка в личке бота открывает Mini App; startapp прокидывается через URL как `tgWebAppStartParam`: ПРОВЕРЬ на реальном клиенте (Task 6); fallback — url `https://t.me/<bot>?startapp=` (как шеринг). Выбери web_app, зафиксируй.
- Колбэки: `srch:yes:<id>` → searches.Touch+MarkTTLAsked reset (ack «✓ Ищу дальше», метка); `srch:found:<id>` → searches.Close(found_here) (ack «✓ Поиск закрыт — удачи!»); владелец = tg_id тапнувшего ↔ searches.user_id, иначе ack «не твой поиск»; `poll:<match>:<answer>` → SavePollAnswer, участник проверяется, метка ответа.
- [ ] Тесты: воркер на фейковом Sender + фейк-репо с управляемыми часами: агрегация 3 строки → 1 сообщение с «ещё 3 человека»; окно 24ч держит; личные поштучно; хвост liked после 10; matched не хвостуется; ErrBlocked → блок + fail; 5 неудач → failed; TTL-скан шлёт и MarkTTLAsked; пауза по молчанию; колбэки — таблица (свой/чужой/повтор). tgbot: httptest фейк-API для editMessageText и web_app-кнопки.
- [ ] Verify + коммит «Пуши: воркер доставки, кнопки и колбэки».

### Task 5: Фронт — диплинки и петля реджекта

**Files:**
- Modify: `web/src/App.tsx` (словарь startapp: pool_<zone>/incoming/match_<uuid>/mine/photo/about/feed + point_ существующий), `web/src/api/client.ts` (startParam парсер расширить), `web/src/screens/MyProfileScreen.tsx` (photo_status/about_status: строки «Фото не подошло — загрузи другое, где видно лицо» / «Текст „о себе" не подошёл — поправь его» с переходом; заглушка при rejected у active; «новое фото на проверке» при pending у active), `web/src/App.tsx` (маппинг статуса не схлопывать в active|pending; открытие редактора на нужном шаге — реюз OnboardingFlow с начальным шагом фото/about: проверь, как открывается editProfile).
- [ ] Verify (`tsc -b --force`, build) + коммит «Фронт: диплинки пушей и петля реджекта».

### Task 6: devseed, приёмка, спека

- devseed: не трогать (пуши на сидов не уходят — у сидов фейковые tg_id, sendMessage упадёт с chat not found → push_blocked_at; ЭТО НОРМА, но проверь, что воркер не молотит по ним каждый тик: FailAllPending после ErrBlocked решает).
- Приёмка на стенде (api перезапустить; MODERATION_CHAT_ID и личка 188993596): лайк Дева (browser) → Сене приходит «Тебя лайкнул Дев»; ответ → «У тебя мэтч!» обоим (Деву не дойдёт — фейковый tg_id, ок); сохранение поиска Дева в Хамовниках → Сене «В Хамовниках ищет квартиру ещё один человек»; подкрутить last_active_at поиска Сени на -8 дней → «Всё ещё ищешь…?» с кнопками → тап «Да» → метка; вердикт по точке → «Твоё объявление одобрено».
- CLAUDE.md: §7 переписать (типы, пороги, тексты — по спеке; убрать 2–3/день), §3 TTL уточнить, §5 пуш реджекта, §11 таблицы, §12 (шаг 6 сделан, 7 текущий), §13 (свежесть + парсер).
- Леджер, финальное ревью ветки, фикс-волна.
- [ ] Коммит «Спека: шаг 6 закрыт — пуши».

## Self-review

Покрытие спеки: §1→T1; §2 тексты/типы→T2; Emit/фан-аут→T2; триггеры→T3; §4 воркер+колбэки→T4; §5 фронт→T5; §6 тесты — по задачам; приёмка/спека→T6. ВАЖНОЕ уточнение из T2 в T1: actor-индекс включает context_key (`UNIQUE (user_id, type, actor_key, coalesce(context_key,''))`) — иначе подписчик с двумя пересекающими поисками получит одну строку вместо двух контекстов. Типы согласованы: Notification/NotificationType (T1) — сервис (T2) — воркер (T4); Notifier-интерфейсы объявляются в потребителях (T3) сигнатурами методов сервиса T2.
