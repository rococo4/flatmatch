# Точки — дизайн (§12 шаг 5)

Дата: 2026-08-13. Статус: подход А одобрен; пользователь разрешил реализацию
без ревью секций и спеки (ночной режим, как шаг 4).

Охват: формы (квартира по ссылке, «к себе»), квартирные пулы, дешёвые лайки,
выход из пула, дорогие лайки с point-контекстом, лента с точками, «Мои поиски»
блоки 2–3, модерационный чат на ОБЕ очереди (§2.2, §5, §6, §8).

## Решения пользователя

| Вопрос | Решение |
|---|---|
| Краудсорс-свежесть (§8: шторка, голоса, порог скрытия) | ЦЕЛИКОМ шаг 7. Карточка показывает только «добавлена N дней назад» от created_at |
| Модерационный чат | ОБЕ очереди §5: анкеты (ок / фото не ок / текст не ок) и точки (ок / отклонить) |
| Фильтр точек в выдаче | Режет ТОЛЬКО зона; цена и комнатность не участвуют ни в фильтре, ни в сортировке. Ранжирование — §8: score ~ pool/(age+c)^k |
| Возврат в пул вошедшего | Блок «Присмотренные квартиры» в «Моих поисках» = созданные (лимит 5) + вошедшие (лимита нет), тап → пул |

Наследуется из утренней волны шага 4 (правило «мы увиделись», коммит 8381586):
- дешёвый вход в пул квартиры поиска НЕ требует;
- дорогой лайк из пула квартиры: отношение = co-membership (оба в likes_points
  этой точки), поиск НЕ требуется;
- отклик «Хочу эту комнату»: отношение несёт сама карточка-витрина, поиск НЕ
  требуется; профиль автора intent_home открыт всем видящим карточку;
- смэтченные скрыты из discovery: список участников пула точки — тоже
  discovery, тот же NOT EXISTS-фильтр по matches.

## 1. Схема — миграция 0004_points.sql

```sql
points (
    id uuid PK DEFAULT gen_random_uuid(),
    type   text CHECK (type IN ('flat','intent_home')),
    stage  text NULL CHECK (stage IN ('live','moving_in')),
    status text NOT NULL DEFAULT 'pending'
           CHECK (status IN ('pending','active','paused','closed')),
    source text NOT NULL CHECK (source IN ('tg_channel','user_link','manual','realtor')),
    created_by uuid NULL REFERENCES users(id),
    zone_id text NOT NULL,          -- станция из справочника (kind=station)
    price_total int NULL,            -- flat: общая цена
    price_share int NULL,            -- intent_home: цена доли въезжающего
    rooms int NULL,
    isolated bool NULL,
    bathrooms int NULL,              -- опционально из формы flat
    utilities text NULL,             -- included / meters, опционально
    restrictions text[] NOT NULL DEFAULT '{}',  -- чипы ограничений
    photo_key text NULL,             -- скрин (flat) / фото комнаты (intent_home)
    url_norm text NULL UNIQUE,       -- нормализованная ссылка, дедуп
    move_in date NULL,               -- intent_home moving_in: обязателен (сервис)
    deposit int NULL,
    close_reason text NULL,
    created_at, edited_at timestamptz
);
-- CHECK: (created_by IS NULL) = (source IN ('tg_channel','manual'))
-- CHECK: type='intent_home' → stage NOT NULL AND created_by NOT NULL
-- CHECK: type='flat' → stage IS NULL

likes_points (
    user_id  uuid REFERENCES users(id),
    point_id uuid REFERENCES points(id),
    created_at timestamptz DEFAULT now(),
    PRIMARY KEY (user_id, point_id)
);
```

Только type='flat' имеет пул — likes_points гейтится в транзакции входа
(кросс-табличный CHECK невозможен). point_changes НЕ создаётся: редактирования
точек в §6 нет, дельта-пуши — шаг 6+.

Лимиты (в транзакции создания, лок родительской строки users — урок шага 3):
user_link: ≤5 активных (pending+active) созданных; intent_home: live строго 1,
moving_in ≤2, суммарно ≤2 активных «к себе». Слот поисков не занимают.

## 2. Домен и репо (pointsrepo + расширение likesrepo)

- `pointsrepo.Create` — вставка + лимиты под локом users-строки.
- `Feed(viewer, zoneIDs)` — активные точки, чья станция ∈ зоны выдачи;
  каждой — счётчики: flat «в пуле N» (likes_points × visible_people ×
  not-matched — персональный), intent_home «N откликов» (число дорогих лайков
  с этим point-контекстом — агрегат, имена не светятся). Сортировка
  score = pool/(age_days+2)^1.5 (§8), intent_home считает откликами.
- `Mine(viewer)` — созданные (все статусы кроме closed) + вошедшие
  (likes_points), для блоков «Моих поисков».
- `PoolMembers(viewer, pointID)` — участники пула flat-точки: likes_points ×
  visible_people × not-matched; каждому — свежайший активный поиск (для чипов
  параметров; их может не быть — карточка тогда без чипов), совпадения анкеты.
  Доступ — только участнику пула (проверка в сервисе: сам в likes_points).
- `Enter/Leave(user, pointID)` — вход (только active flat, идемпотентно
  ON CONFLICT) и выход (простое удаление; дорогие лайки НЕ трогаются §2.3).
- `likesrepo.Create` — контекст расширяется point_id (взаимоисключим с
  zone_id). Гейты тратящего лайка при point-контексте: точка active; для
  intent_home — to_id == created_by точки; для flat — от лайкающего есть
  likes_points на эту точку (co-membership). Требования поиска НЕТ. Ответные
  лайки как были — без гейтов. Бюджет 5/24ч общий для всех контекстов.
- `peopleapi.metInPool` — расширение отношения: co-membership активной flat
  ИЛИ target — автор активной intent_home. Профиль принимает ?point={id}
  как контекст (мини-карточка «пересеклись: {точка}»).
- `matches` саммари: контекст-точка → блок ГДЕ «на комнату {имя} · м. {метро}»
  (intent_home) / «квартира у м. {метро}» (flat), исторический.

## 3. Модерация (service/moderation + tgbot)

- Конфиг: `MODERATION_CHAT_ID` (пустой → очереди только копятся в БД, warn).
- Событийная отправка: анкета ушла в pending (онбординг-сабмит или правка
  фото/«о себе» у active) → sendPhoto (фото + caption: имя, возраст, «о себе»)
  с inline-кнопками `[ок] [фото не ок] [текст не ок]`; точка создана →
  sendPhoto (скрин/фото комнаты + поля) с `[ок] [отклонить]`.
- callback_query в polling-цикле tgbot → moderation.Handle(data):
  - `mod:u:ok:<id>` → status=active, photo_status=ok, about_status=ok;
  - `mod:u:photo:<id>` → §5 две ветки: не был active → status=draft +
    photo_status=rejected; был active → только photo_status=rejected;
  - `mod:u:about:<id>` → симметрично про about_status;
  - `mod:p:ok:<id>` → point active; `mod:p:no:<id>` → point closed
    (close_reason='rejected').
  Ответ: editMessageReplyMarkup (кнопки убрать) + answerCallbackQuery «✓».
- `/queue` в модерационном чате — дослать всё pending (догон после рестарта).
- Идемпотентность: повторный тап по устаревшей кнопке — no-op с ответом
  «уже обработано».

## 4. HTTP (pointsapi + расширения)

- `POST /points` — обе формы одним эндпоинтом (type в теле).
  flat: url (нормализация → url_norm; домен только avito.ru/cian.ru),
  станция, price_total 20к–300к, rooms, isolated, photo_key (обязателен),
  опционально bathrooms/utilities/restrictions/deposit. Дубль url_norm →
  409 `{point_id, pool_count}` — «эта квартира уже есть, в пуле N — войти?».
  intent_home: stage (гейт формы), станция, price_share (та же вилка),
  photo_key (фото комнаты), isolated, deposit?, restrictions?, move_in
  (обязателен при moving_in). Источник: user_link (flat) / intent_home без
  source-поля в API — сервер сам ставит user_link.
  Ответ: точка со статусом «на проверке». Создателю видна сразу (§5).
- Фото: реюз photostore-аплоада онбординга (тот же endpoint, ключ в форму).
- `GET /points/{id}` — карточка (flat: «в пуле N», вошедшему — флаг in_pool;
  intent_home: «N откликов», bio-строка автора). pending-точка видна только
  создателю, остальным 404.
- `POST /points/{id}/enter` / `DELETE /points/{id}/enter` — дешёвый вход/выход.
- `GET /points/{id}/pool` — участники, только вошедшим (403 need_enter).
- `GET /points/mine` — блоки «Моих поисков» (created + entered).
- `GET /feed` — points перестаёт быть `[]`: карточки в зоне формы + счётчик
  «M квартир» для searches DTO (points_count по зоне поиска).
- `POST /likes` — принимает `point_id` (взаимоисключим с zone_id).
- `GET /people/{id}?point=` — контекст точки в профиле.

## 5. Фронт

- Оживить готовые DEMO-экраны: PointFormScreens (обе формы → POST /points,
  фото-аплоад, 409-дубль «войти?», финал «ты первый в пуле» + шеринг),
  FlatPoolScreen (участники, «Выйти из пула» с готовой шторкой), FeedScreen
  (реальные карточки, «Интересно — войти в пул» одним тапом, intent_home
  «Хочу эту комнату» = postLike с point_id и подписью остатка), MySearches
  блоки 2–3 (реальные списки, «M квартир» на карточке поиска).
- Профиль из пула точки: openPerson с point-контекстом, лайк с point_id.
- «Тебя лайкнули»: пометка «на твою комнату» при point-контексте входящего
  (контекст-карта точки, тап → карточка точки).
- Мэтч: блок ГДЕ с точкой.
- Шеринг «Позвать в пул»: t.me/share/url?url=https://t.me/{bot}/app?startapp=
  point_{id}; при старте приложения start_param `point_<id>` → открыть
  карточку точки (простая обработка initData.start_param).
- StillLiveSheet остаётся спящей (свежесть — шаг 7).

## 6. devseed

2–3 рыночные квартиры в Хамовниках/на красной ветке (active, с пулами из
сидов), 1 intent_home от Кати (live) с парой откликов. Обновить README.

## 7. Тесты

- pointsrepo: лимиты под гонкой (6 параллельных user_link → 5), intent_home
  каунты по стадиям, дедуп url_norm, счётчики персональны (пол/блок/мэтч),
  pool members только вошедшим.
- likesrepo: point-контекст — co-membership гейт: flat-лайк не-члена пула →
  ErrNotFound (профиль участника ему и так недоступен, лайк отвечает тем же);
  intent_home-лайк не-автору точки → ErrNotFound; отклик без единого поиска
  валиден; бюджет общий с зонными лайками.
- moderation: маппинг колбэков на статусы, ветки §5 (draft vs active),
  идемпотентность повторного тапа.
- httptest: приватность DTO точек (created_by не светится — §2.2 обезличена),
  pending видна только создателю, пул только вошедшим.
- Приёмка curl'ом (Chrome может быть отключён): формы → модерация (ok/no) →
  лента → вход в пул → дорогой лайк из пула → мэтч с point-контекстом →
  отклик на «к себе» → «на твою комнату» → мэтч.

## Не входит

Свежесть/голоса/TTL/пинги (шаг 7); пуши, включая «точка изменилась» (шаг 6);
point_changes и редактирование точек (нет в §6); source='realtor'; парсер;
комментарии при входе в пул (§9); спецэкран разбора откликов (§2.2 — нет).

## Правки спеки по итогам

- §6 «Мои поиски» блок 2: «Присмотренные» = созданные (N из 5) + вошедшие
  (без лимита) — уточнить формулировку.
- §13: DoD шага 5 — по этому доку.
