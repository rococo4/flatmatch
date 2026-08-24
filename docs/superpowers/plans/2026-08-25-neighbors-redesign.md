# Шаг 11 — редизайн «Соседи» — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Задачи трекаются чекбоксами.

**Goal:** Карточный подбор со свайпами вместо списков и ленты, вкладка «Лайки» с отклонением, пропуски, фильтры, до 5 фото анкеты, «к себе» с описанием, рыночные квартиры выключены (этап 1).

**Architecture:** Go-монолит `api/` (handler → service → storage, pgx, goose, chi), React+TS `web/`. Спека — `docs/superpowers/specs/2026-08-25-neighbors-redesign-design.md` (истина при конфликте с планом; CLAUDE.md на диске обновлён под шаг 11 — снапшот в контексте сабагента может быть устаревшим). Вёрстка и микрокопия фронта — макет https://claude.ai/code/artifact/2271b036-3b81-4523-892e-cde93c079a53, страница «Подбор · светлая» (сабагентам недоступен — все нужные тексты и параметры вёрстки продублированы в спеке §4 и CLAUDE.md §6, им верить).

**Tech Stack:** Go 1.26, pgx v5, goose, chi; React 19 + TS + Vite, @telegram-apps/sdk-react; свайп — pointer events руками, без библиотек.

**Spec:** docs/superpowers/specs/2026-08-25-neighbors-redesign-design.md

## Global Constraints

- Go и SQL — **ноль комментариев** (только `//go:`, `//nolint`, `-- +goose`). Порядок в Go-файле: константы и типы наверху, функции ниже, публичные раньше приватных. Интерфейсы — на стороне потребителя. Фронт — комментарии в существующем стиле («почему», не «что»).
- Русская микрокопия — ДОСЛОВНО из спеки и CLAUDE.md §4/§6.
- Верификация Go: `cd api && gofmt -l . && go build ./... && go vet ./... && TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' go test ./...` (Postgres: `docker compose up -d postgres`). Фронт: `cd web && npx tsc -b && npm run -s lint && npm run build`. Тесты — общая дев-БД: только свои строки, НЕ трогать users tg_id=188993596.
- Клиент один и деплоится вместе с api — обратная совместимость API между задачами НЕ нужна; главное, чтобы к концу шага фронт и бэк сходились.
- Коммиты в main (короткое русское сообщение + trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`), после коммита `git push origin main`. Прод не деплоить — контроллер.
- Рыночные квартиры: код НЕ удалять физически (этап 2) — убирать из роутера/UI, гейтить, помечать неиспользуемое, но пакеты/таблицы/тесты парсера остаются компилируемыми и зелёными.

---

### Task 1: Фундамент — миграция 0013, фото-набор, пропуски, описание комнаты

**Files:**
- Create: `api/migrations/0013_neighbors.sql` (SQL из спеки §1; имя CHECK occupation выяснить `\d users` на дев-базе — в 0001 он анонимный)
- Modify: `api/internal/entity/{profile,user,point}.go` (+тесты), `api/internal/repo/usersrepo/*` (photo_keys[], occupation none), `api/internal/repo/pointsrepo/*` (description), новый `api/internal/repo/passesrepo/{repo,postgres}.go` (+тест), `api/internal/service/users/service.go` (+тест), `api/internal/service/points/*` (description в форму «к себе»), `api/internal/service/moderation/*` (карточка модератору: все фото альбомом `sendMediaGroup`, вердикт один; описание «к себе» в подпись), `api/internal/api/http/usersapi/*` (фото-API), `api/internal/api/http/pointsapi/*` (description в DTO), все места `PhotoKey`/`photo_url` (компилятор покажет; DTO чужих людей → `photo_urls []string`, собственный профиль → `photo_urls`).

**Interfaces (Produces):**
```go
// entity.User: PhotoKey string → PhotoKeys []string (≤5, первый — главный)
// users service
func (s *Service) AddPhoto(ctx, id uuid.UUID, file io.ReadSeeker) (entity.User, error)      // >5 → entity.FieldsError{fields:["photo"]} c ErrPhotoLimit
func (s *Service) RemovePhoto(ctx, id uuid.UUID, index int) (entity.User, error)            // последний нельзя после первой отправки → ErrLastPhoto
func (s *Service) ReorderPhotos(ctx, id uuid.UUID, order []int) (entity.User, error)        // не перестановка → invalid_field
// любая из трёх → photo_status = pending (логика active/draft как у SetPhoto сейчас); ключ фото photos/<user_id>-<8 случайных hex>.<ext>
// HTTP: POST /me/photos (multipart photo), DELETE /me/photos/{index}, PUT /me/photos/order {"order":[2,0,1]}; старый POST /me/photo удалить
// photostore: URL(key) без изменений; PhotoURLs(u) []string в сервисе users
// passesrepo
func (r *Repo) Upsert(ctx, userID, targetID uuid.UUID) error                                 // INSERT ... ON CONFLICT DO UPDATE SET created_at = now()
func (r *Repo) ActiveTargets(ctx, userID uuid.UUID, within time.Duration) ([]uuid.UUID, error)
func (r *Repo) Clear(ctx, userID uuid.UUID) (int, error)
// points: entity.Point.Description *string; форма intent_home принимает description (≤400 → invalid_field), flat её не имеет
```

- [ ] Тесты первыми (entity: photo_keys лимиты/порядок; passesrepo: upsert двигает created_at, ActiveTargets режет по окну, Clear возвращает счёт; users service: add/remove/reorder → pending, ErrLastPhoto только после отправки — признак: status != draft ИЛИ анкета уже отправлялась (использовать существующее правило SetPhoto); points: description валидация и round-trip).
- [ ] Миграция на дев-БД; проверить бэкфилл `photo_keys = ARRAY[photo_key]` на существующих строках (кроме tg_id 188993596 — читать можно).
- [ ] Модерация: `sendMediaGroup` с подписью у первого фото; если Bot API клиент пакета не умеет — добавить метод в `tgbot/client.go` по образцу sendPhoto. Вердикт по анкете — один, как раньше.
- [ ] Верификация Go, коммит «Шаг 11: фото-набор, пропуски, описание комнаты — фундамент», push.

---

### Task 2: Подбор и лайки — бэкенд

**Files:**
- Create: `api/internal/service/deck/service.go` (+тест), `api/internal/api/http/deckapi/{handler,dto}.go` (+тест)
- Modify: `api/internal/repo/searchesrepo` (кандидаты по ВСЕМ зонам моих поисков с ЕГО поисками целиком), `api/internal/repo/pointsrepo` (intent_home для подбора), `api/internal/service/pools/verdicts.go` (экспорт Compare как есть — лучшая пара считается в deck), `api/internal/service/matches/summary.go` (лучшая пара вместо первого активного; +тест), `api/internal/api/http/likesapi` (incoming + pair, сортировка, исключение passes), `api/internal/api/http/peopleapi` (`POST /{id}/pass`), `api/internal/api/http/pointsapi` (`POST /{id}/pass`), `api/internal/api/http/usersapi` (`DELETE /me/passes` → `{"restored": N}`), `api/cmd/api/main.go`.

**Interfaces (Produces):**
```go
// GET /api/v1/deck?who=&age_min=&age_max=&pets=&smoking=&schedule=&occupation=
// параметры-списки через запятую; ответ {"cards":[card...], "no_search": bool, "total": N}
// card: {"kind":"person"|"room", person|room: {...}, pair: {zone, zone_match, section, chips, params}} — поля из спеки §2
// Лучшая пара: для кандидата перебрать (мой активный поиск × его активный поиск) с пересечением зон,
// Compare() из pools; выбрать max по (sectionRank, число green-чипов, mine.CreatedAt).
// Исключения: сам, matched, liked-by-me, passes(7d), visible_people; who=rooms — intent_home точки зон моих поисков.
// Фильтры: occupation working = office|hybrid|remote|shift; pets none→pets_own='none', ok_mine→pets_ok='ok';
// smoking no→'no', outside_only→IN('no','outside'); schedule owl|lark; возраст BETWEEN.
// Порядок: секция лучшей пары → last_seen/created DESC; комнаты ранжируются той же секцией (цена доли vs моя вилка = жёсткий, заезд при наличии).
// deck без активного поиска → 200 {"cards":[], "no_search":true}
// POST /people/{id}/pass, POST /points/{id}/pass → 204 (цель должна существовать; людям — visible ИЛИ matched ИЛИ лайкнул меня, точкам — активная intent_home)
// DELETE /me/passes → 200 {"restored": N}
// GET /likes/incoming: элементы += pair (лучшая против моих поисков; у room-контекста pair.zone = зона точки, room:true), sort по pair, минус passes(7d)
```

- [ ] TDD: deck-сервис на тестовой БД (дедуп по человеку при двух пересекающихся моих поисках; лучшая пара — секция/чипы/свежесть; passes прячет и протухает; фильтры; смэтченные/лайкнутые исключены; no_search; комната в общем порядке), pass-хендлеры (204/404), passes clear, incoming сортировка+pair.
- [ ] Саммари мэтча — лучшая пара (тест: два поиска у каждого, берётся совместимая пара).
- [ ] Верификация Go, коммит «Шаг 11: подбор и лайки — бэкенд», push.

---

### Task 3: Выключение рыночных квартир — бэкенд и инфра

**Files:**
- Modify: `api/cmd/api/main.go` (убрать из роутера: `GET /feed`, промо/пул-эндпоинты poolsapi, вход/выход/голоса flat в pointsapi — точечно, по существующим маршрутам; `GET /zones/top` остаётся), `api/internal/api/http/pointsapi` (создание `POST /points` принимает ТОЛЬКО intent_home → flat-тело 400 `invalid_field type`; карточка/диплинк intent_home живут), `api/internal/service/points` (гейт создания flat), `docker-compose.yml` (parser: `profiles: ["off"]`), `api/cmd/devseed` (не сеять flat), тесты затронутых пакетов (flat-тесты сервиса точек: создание через сервис → ожидать отказ; репо-тесты flat НЕ трогать — таблицы живут).
- Свежесть/пуши: убедиться (тестом или чтением), что воркеры пинга/TTL не шлют по закрытым flat; `pool_new`/`point_gone` не эмитятся без flat-событий — код не удалять.

**Interfaces:** `POST /api/v1/points` с `type:"flat"` → 400; роуты `/feed`, `/points/{id}/enter|leave|vote` и им подобные — 404 (сняты с роутера).

- [ ] Убрать маршруты и гейт, поправить devseed, compose; тесты хендлеров обновить (снятые роуты — удалить их тесты, гейт — новый тест).
- [ ] Верификация Go + `docker compose config` (parser не в дефолтном профиле), коммит «Шаг 11: рыночные квартиры выключены (этап 1)», push. Разовый SQL закрытия точек НЕ выполнять — контроллер при выкатке.

---

### Task 4: Подбор — фронт

**Files:**
- Create: `web/src/screens/DeckScreen.tsx` (карточка человека/комнаты, свайп, кнопки-иконки, точки лимита, состояния first/pending/end), `web/src/screens/FiltersSheet.tsx`, `web/src/ui/SwipeCard.tsx` (pointer events: drag, порог 35% ширины, поворот ±8°, возврат при отмене)
- Modify: `web/src/ui/TabBar.tsx` (4 вкладки: Соседи/Лайки/Поиски/Профиль, иконки people/heart/ring/profile, бейдж числа на «Лайках»), `web/src/App.tsx` (табы, состояние deck, deep links `pool_`→Соседи), `web/src/api/client.ts` (`fetchDeck(filters)`, `passPerson/passPoint`, `clearPasses`, типы DeckCard/DeckPair), `web/src/api/format.ts` (лейблы фильтров), `web/src/screens/SearchParamsScreen.tsx` (кнопка «Сохранить и смотреть», режим look удалить: `mode` больше не нужен), `web/src/screens/FeedScreen.tsx` и `FeedEntryScreen.tsx` — умирают (удалить вместе с потребителями; «Где уже ищут» и вход «С кем будешь жить?» переезжают в состояния DeckScreen).

**Вёрстка (из спеки/§6 CLAUDE.md, дословно):** шапка «Соседи · N в твоих зонах» + иконка-слайдеры; карточка: фото с сегментами-пейджером (тап по краям листает `photo_urls`), бейджи зоны/«совпали по всем параметрам» на фото, имя на градиенте, вердикт-чипы + привычки; комната — зелёная шапка «Комната · сосед уже живёт там», факты «Метро / Твоя доля / Комната», описание, чипы, строка автора; кнопки: пилюля со стрелкой (54px, radius 16) и синяя пилюля с сердцем + тень; 5 точек остатка; фильтры — шторка «Каким должен быть сосед» с чипами-мультивыбором и «Показать · N» (счётчик — длина отфильтрованной колоды); интро «Как это работает» (3 шага из §6); «Пока все» + «Показать пропущенных · N» + «Сохранить ещё поиск · слот K из 3»; pending-плашка. Фильтры — localStorage (`deck_filters_v1`), try/catch на чтении.

- [ ] Реализация + `tsc/lint/build`; ручной прогон свайпа на стенде (running-locally) — желательно, не блокер. Коммит «Шаг 11: подбор — фронт», push.

---

### Task 5: Лайки, поиски, комната — фронт

**Files:**
- Create: `web/src/screens/LikesScreen.tsx` (плитки), `web/src/screens/LikerProfileScreen.tsx` (анкета + стрелка/сердце)
- Modify: `web/src/App.tsx` (вкладка «Лайки», бейдж, decline → `passPerson` + перечитка), `web/src/screens/MyProfileScreen.tsx` (убрать «Тебя лайкнули»), `web/src/screens/IncomingLikesScreen.tsx` — удалить, `web/src/screens/MySearchesScreen.tsx` (кнопка «Смотреть в „Соседях“» → toTab('deck'); блоки рыночных квартир удалить; «к себе» блок живёт), `web/src/screens/PointFormScreens.tsx` (форма flat удалена; intent_home += поле «Описание» textarea ≤400 со счётчиком), `web/src/screens/HomePointView.tsx`/`FlatPool*` (flat-экраны удалить; экран комнаты — описание + пейджер фото), `web/src/screens/PersonProfileScreen.tsx` (фото-пейджер по `photo_urls`).

**Вёрстка:** плитка — фото 150px с бейджем «ищет: {зона}» (розовый `--type-search-fg` на белой подложке) или «на твою комнату» (зелёный + рамка `1.5px --type-home-fg`), имя на градиенте, ✓-чипы совпавших параметров под фото; анкета лайкнувшего — «лайкнул(а) тебя» в подзаголовке, внизу те же пилюли-иконки + подпись «Сердце — взаимный лайк: не тратит лимит и открывает контакты»; пустое состояние «Пока никто не лайкнул. Появись в подборе — сохрани поиск».

- [ ] Реализация + верификация фронта, коммит «Шаг 11: лайки, поиски, комната — фронт», push.

---

### Task 6: Фото-сетка и анкета — фронт

**Files:**
- Create: `web/src/ui/PhotoGrid.tsx` (сетка: большое главное с меткой «главное», маленькие с крестиками, слоты «+», drag-reorder на pointer events; заголовок «Фото · N из 5», одна жёлтая метка «уйдут на проверку» при изменённом наборе, подпись «Первое — главное, его видят в подборе. Зажми и перетащи, чтобы поменять порядок. Любое изменение отправляет все фото на проверку разом»)
- Modify: `web/src/api/client.ts` (`addPhoto`, `removePhoto(index)`, `reorderPhotos(order)`, Profile.photo_urls: string[]), `web/src/screens/ProfileEditScreen.tsx` (PhotoGrid вместо «Заменить фото»), `web/src/screens/OnboardingScreens.tsx` (PhotoStep → PhotoGrid, «до 5, первое — главное», хватает одного для «Дальше»; вопрос 1 += «Не работаю» (`none`); вопрос 9 — новая формулировка и лейблы из §4), `web/src/api/format.ts` (`occupationLabel` += «не работает»; лейблы expenses), `web/src/screens/*` — потребители `photo_url` → `photo_urls[0]`.

- [ ] Реализация + верификация фронта; ручная приёмка онбординга на стенде желательна. Коммит «Шаг 11: фото-сетка и анкета — фронт», push.

---

### Task 7: Спека-хвосты и выкатка (контроллер)

- [ ] Пройтись по CLAUDE.md/README на предмет хвостов (промо-блок, лента, «Тебя лайкнули») — поправить.
- [ ] Выкатка: разовый SQL закрытия flat-точек, `scripts/deploy.sh`, приёмка на проде двумя аккаунтами (подбор с двух сторон, лайк/пропуск/возврат, отклонение лайка, взаимный, комната с описанием, фото-сетка, фильтры), контроль «parser не поднялся».
