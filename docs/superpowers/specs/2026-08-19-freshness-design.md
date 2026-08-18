# Свежесть квартир — дизайн (§12 шаг 7a)

Дата: 2026-08-19. Статус: дефолты согласованы одним ответом («давай»);
реализация ночью автономно, приёмка утром.

Охват: краудсорс-свежесть §8 для квартир с рынка (шторка после перехода по
ссылке, голоса, порог скрытия, «Проверил — она живая» у создателя),
еженедельный пинг создателю (user_link) и автору «к себе», TTL-архив,
пуш пулу «похоже, сдана», строка свежести на карточке, отражение в «Моих
поисках». Парсер каналов и «пост удалён» — отдельная спека 7b.

## Решения

| Вопрос | Решение |
|---|---|
| Порядок в шаге 7 | Сначала свежесть (7a), парсер (7b) — после отдельного брейншторма |
| Кто голосует | Любой авторизованный, кто видит карточку и нажал «Открыть оригинал»; не только член пула. Один голос на (человек, квартира), передумать можно (upsert). Создатель голосует на общих основаниях |
| Порог скрытия | «сдана» ≥ max(2, ceil(0.3 × размер пула)); пул = likes_points точки (создатель в нём) |
| Шторка | Не ловим возврат из браузера — после тапа по ссылке под ней появляется блок-вопрос и висит до ответа или ухода с карточки |
| Скрытие | `paused` («похоже, сдана»): из ленты пропадает, у вошедших остаётся строкой в «Моих поисках»; tg_channel/manual без создателя → сразу `closed`/`gone` |
| Восстановление | «Проверил — она живая» у создателя user_link: paused → active, голоса стёрты, last_seen_alive = now. Второе скрытие финальное (`closed`/`gone`); ссылку можно подать заново — через модерацию |
| Пинг | Раз в 7 дней с last_seen_alive: user_link — «Квартира у м. Сокол ещё актуальна?» `[Да, живая] [Уже сдана]`; intent_home — «Комната у м. Сокол ещё свободна?» `[Да] [Нашёл соседа]` |
| TTL-архив | 14 дней без подтверждений ИЛИ 3 дня молчания после пинга → `closed`/`expired`; реактивации точек нет |
| Строка свежести | «подтверждена сегодня/вчера/N дней назад» / «добавлена N дней назад, не проверялась»; один голос «сдана» — серая пометка |
| «Точка изменилась», URL-чекер, «пост удалён» | Не в 7a (§9 / 7b) |

## 1. Схема — миграция 0008_freshness.sql

```sql
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
```

- `last_seen_alive` — последнее подтверждение жизни: создание, голос «да»,
  «Проверил — она живая», ответ «да» на пинг, для intent_home — ещё и
  рождение мэтча с этой точкой в контексте. TTL и пинг считаются от него.
- `gone_count` — сколько раз точку скрывали голосами; у user_link 0 → первое
  скрытие даёт paused, 1 → второе финальное.
- `ping_asked_at` — когда задан пинг; сброс при любом подтверждении.
- `points.status = 'paused'` до сих пор не использовался — теперь paused
  означает ровно «похоже, сдана» (отдельного pause_reason нет).
- `close_reason` пополняется значениями `gone` (скрыта голосами финально /
  «Уже сдана» на пинг), `expired` (TTL), `found` (intent_home: «Нашёл
  соседа»). Существующие `closed`, `rejected` не трогаем. Словарь — в
  entity, CHECK в БД не добавляем (колонка исторически свободная).

## 2. Голоса — сервис points

`Vote(ctx, userID, pointID, alive bool)`:

1. Точка читается как карточка для смотрящего (`CardByID`, те же правила
   видимости, что у экрана): не видна → 404. Не `flat` или без `url_norm`
   → 422 `not_votable`. Статус не `active` → 409 `point_not_active`.
2. Upsert голоса `(user_id, point_id) → alive`.
3. `alive = true` → `points.last_seen_alive = now(), ping_asked_at = NULL`.
4. `alive = false` → в одной транзакции с локом строки точки:
   `gone = count(votes WHERE NOT alive)`, `pool = count(likes_points)`;
   если `gone >= max(2, ceil(0.3 * pool))`:
   - `source = 'user_link' AND gone_count = 0` → `status = 'paused',
     gone_count = 1, edited_at = now()`;
   - иначе → `status = 'closed', close_reason = 'gone', gone_count + 1`.
   После коммита — пуш `point_gone` (§5). Голоса при скрытии НЕ стираются
   (стираются только восстановлением).
5. Ответ — обновлённая карточка (DTO §6).

`Restore(ctx, userID, pointID)`: только создатель, только `paused` → иначе
404 (чужая) / 409 `point_not_paused`. `status = 'active', last_seen_alive =
now(), ping_asked_at = NULL, edited_at = now()`; `DELETE FROM point_votes
WHERE point_id`. `gone_count` не сбрасывается. Пуша нет.

Порог — чистая функция `entity.GoneThreshold(pool int) int` = `max(2,
ceil(0.3*pool))`, покрыта таблицей (пул 0..10).

## 3. Пинг и TTL — воркер notifications (часовой скан)

К `askTTL / pauseSilent / askPostMatch` добавляются:

- `pingPoints`: `points.PointsForPing(7d)` — `status = 'active' AND
  (source = 'user_link' OR type = 'intent_home') AND created_by IS NOT NULL
  AND last_seen_alive < now() - 7d AND (ping_asked_at IS NULL OR ping_asked_at
  < now() - 7d)`. На каждую — строка `point_ping` создателю (§5) +
  `MarkPingAsked`. Actor = `"<point>:<last_seen_alive unix>"` — раунд
  молчания, как у still_searching.
- `expirePoints`: `points.Expire(14d, 3d)` — `UPDATE points SET status =
  'closed', close_reason = 'expired', edited_at = now() WHERE status IN
  ('active','paused') AND type IN ('flat','intent_home') AND (last_seen_alive
  < now() - 14d OR (ping_asked_at IS NOT NULL AND ping_asked_at < now() - 3d
  AND last_seen_alive < ping_asked_at AND EXISTS (sent point_ping по этой
  точке с sent_at >= ping_asked_at - 1 min)))`. `pending` не трогаем
  (очередь модерации не про свежесть). Пуша об архиве нет. Возвращает ids
  для лога.

Колбэки бота, префикс `pnt:` — `pnt:alive:<point>` / `pnt:gone:<point>`,
владелец = created_by по tg_id тапнувшего, чужой → «не твоё», повтор →
«уже ответил» (точка уже не active) :
- alive → `points.TouchAlive(id)` (`last_seen_alive = now(), ping_asked_at
  = NULL`), метка «✓ Отмечено: актуальна» / для intent_home «✓ Отмечено:
  свободна».
- gone → flat: `closed`/`gone`; intent_home: `closed`/`found`. Метка
  «✓ Объявление закрыто». Пуша пулу от пинга нет: пул создателя user_link
  узнаёт при следующем заходе (карточка закрыта), это не голосование.

Мэтч с `context_point_id` (likes → ensureMatch) → после коммита
`points.TouchAlive(context_point_id)` best-effort — активность автора «к
себе» продлевает точку.

## 4. Тексты (дословно) и типы пушей

| type | Кому | Режим | Текст | Кнопка |
|---|---|---|---|---|
| `point_gone` | участники пула минус проголосовавший последним; создатель user_link — отдельным текстом | личный; actor = `"<point>:<gone_count>"`, context = point | членам: «Квартира у м. Фрунзенская, похоже, сдана»; создателю: «Твою квартиру у м. Фрунзенская отметили как сданную — проверь» | членам «Открыть» → `point_<id>`; создателю «Проверить» → `mine` |
| `point_ping` | создатель user_link / автор intent_home | вопрос; actor = `"<point>:<last_seen_alive unix>"`, context = point | flat: «Квартира у м. Сокол ещё актуальна?»; intent_home: «Комната у м. Сокол ещё свободна?» | flat: `[Да, живая]` `pnt:alive:<id>` · `[Уже сдана]` `pnt:gone:<id>`; intent_home: `[Да]` · `[Нашёл соседа]` (те же колбэки) |

Место — `name_at` зоны точки из справочника («у м. Сокол», «в Хамовниках»),
кладётся в payload при вставке (`place`), как у остальных пушей; `kind`
(`flat`/`intent_home`) и `own` (bool) — тоже в payload. `point_gone`
входит в `personalPushTypes` (считается предохранителем), `point_ping` —
нет (вопрос). Оба не агрегируются → страница PendingPersonal (после фикса
8354253 берёт всё не-агрегируемое). Пуши идут через `notifications.Emit`
с `WHERE push_blocked_at IS NULL`; получатели `point_gone` — `PoolMembers`
точки без фильтра visible_people (объект общий, не человек), минус
последний проголосовавший.

## 5. HTTP

- `POST /api/v1/points/{id}/vote` `{"alive": true|false}` → 200 карточка;
  404 / 409 `point_not_active` / 422 `not_votable`.
- `POST /api/v1/points/{id}/restore` → 200 карточка; 404 / 409
  `point_not_paused`.
- DTO точки (`PointDTO`, карточка / лента / mine) дополняется:
  `last_seen_alive` (RFC3339), `gone_votes` (int, число «сдана»),
  `my_vote` (`true|false|null`), `restorable` (bool: моя, user_link,
  paused). `close_reason` уже отдаётся — фронт учит новые значения.

## 6. Фронт

- **Карточка квартиры** (FlatPoolScreen): строка `addedAt` заменяется
  строкой свежести: `last_seen_alive` позже `created_at` (хоть на минуту) →
  «подтверждена сегодня» / «вчера» / «N дней назад»; иначе «добавлена N
  дней назад, не проверялась». При `gone_votes ≥ 1` у активной —
  приписка серым « · кто-то отметил как сданную». У `paused` вместо строки
  — жёлтая плашка «похоже, сдана — отметили N человек»; у создателя под ней
  кнопка «Проверил — она живая» (жёлтая, простое подтверждение не нужно —
  действие обратимо голосами). У `closed`/`gone` — серая «сдана».
- **Шторка**: тап по «Открыть на Авито/Циан» открывает ссылку (как сейчас)
  и показывает под кнопкой блок: заголовок «Объявление ещё живое?», кнопки
  «Да» / «Уже сдана», подпись «Помоги остальным в пуле — сданные мы
  скрываем». После ответа блок сменяется на «Спасибо, отметили · изменить»
  (тап — снова кнопки). Если `my_vote` уже есть при открытии карточки —
  сразу состояние «Ты отметил: живая/сдана · изменить», без ожидания
  тапа по ссылке. Блок только у `flat` с `url` и статусом `active`. Если
  голос скрыл точку — карточка перерисовывается из ответа (paused-плашка).
- **Мои поиски**: строка статуса в «Присмотренных» учит `paused` → «похоже,
  сдана» + действие «Проверил — она живая» вместо «Скрыть» (закрыть тоже
  можно); `closed` с причинами: `gone` → «сдана», `expired` → «истёк срок»,
  `found` → «нашёл соседа», `rejected` → «отклонена» (как было), `closed` →
  «закрыта». У вошедших (блок «в пулах») paused-точка остаётся строкой с
  пометкой «похоже, сдана», тап → карточка. «Зову к себе»: `closed`/`found`
  и `expired` — те же лейблы.
- `startapp` новых нет: `point_<id>`, `mine` уже разбираются.

## 7. Тесты

- entity: таблица порога.
- pointsrepo: upsert голоса; скрытие user_link → paused + gone_count 1;
  повторное → closed/gone; tg_channel → сразу closed/gone; Restore стирает
  голоса и не трогает gone_count; PointsForPing (свежая не попадает,
  спрошенная <7д не попадает); Expire по 14 дням и по молчанию 3 дня только
  при отправленном пинге; pending не трогается; TouchAlive сбрасывает
  ping_asked_at.
- service points: 422 у intent_home, 409 у paused, порог не достигнут → без
  пуша, достигнут → Emit point_gone членам минус голосующий + создателю с
  own=true.
- notifications: тексты point_gone/point_ping (flat/intent_home, own),
  колбэки `pnt:` (чужой, повтор, alive → touch, gone → close с нужной
  причиной), воркер вставляет пинг и архивирует по молчанию.
- httptest: vote/restore коды.
- Фронт: tsc + oxlint; ручная приёмка утром на стенде — Дев голосует
  «сдана» дважды на квартиру пользователя → paused + пуш; «Проверил — она
  живая»; подкрутить last_seen_alive −8д → пинг в личку → «Да, живая».

## Не входит

Парсер и «пост удалён → точка гаснет» (7b); автоматический URL-чекер (§9);
пуш об архиве по TTL; реактивация точек; голоса на intent_home.

## Правки CLAUDE.md по итогам

- §8: механика свежести — как реализовано (кто голосует, порог, второе
  скрытие финальное, пинг и для «к себе», TTL 14д/3д молчания, лейблы).
- §7: типы `point_gone`, `point_ping` в таблицу.
- §11: points.last_seen_alive/gone_count/ping_asked_at, point_votes,
  close_reason словарь.
- §12/§13: 7a сделано, 7b (парсер) — следующий брейншторм.
