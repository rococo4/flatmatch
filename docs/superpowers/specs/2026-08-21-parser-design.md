# Парсер ТГ-каналов + LLM + досыпка — дизайн (§12 шаг 7b)

Дата: 2026-08-21. Статус: развилки A–J согласованы (досье
docs/research/2026-08-21-parser-brainstorm.md + решения Арсения в чате);
реализация конвейером, приёмка на стенде.

Охват: сервис parser (наполнение points source='tg_channel'), таблица сырья
channel_posts, LLM-структуризация, скачивание фото, гашение по удалённому
посту, досыпка source='manual' через CLI. Фронт — одна мелочь (лейбл ссылки
на t.me-оригинал).

## Решения

| Развилка | Решение |
|---|---|
| Транспорт | HTTP-поллинг `t.me/s/<канал>` (лента) + проверка удаления через `t.me/<канал>/<id>?embed=1`; MTProto НЕ делаем, модуль fetch — узкий шов под него |
| Рантайм | Python 3.13 (заглушка parser/ дорастает); httpx, beautifulsoup4, psycopg[binary], anthropic. С бэком — только через БД (§10) |
| Сырьё | Таблица `channel_posts`, миграция 0009 в api/migrations (общая история схемы; api её не трогает) |
| LLM | **`claude-sonnet-5`** (решение Арсения: «можем подороже», ~$10–15/мес на нашем объёме; интро-цена до конца августа), structured outputs, список станций в кэшируемом системном промпте; `ANTHROPIC_API_KEY` — ключ с console.anthropic.com в .env (подписка Claude не конвертируется в API) |
| Дедуп | `url_norm` заполнен ВСЕГДА: avito/cian-ссылка из поста (нормализация байт-в-байт как validate.go), иначе `https://t.me/<канал>/<id>`; уникальный индекс = дедуп; форварды скипаются; эвристика полей — только пометка possible_dup |
| Метро→зона | Список станционных зон (`kind=station`, 313 имён) в системном промпте, LLM выбирает одну или null; парсер валидирует exact-match (lower, ё→е, трим) → id; не распозналось → skip `no_zone` |
| Фото | До 5 фото поста → photostore теми же ключами `points/<uuid>.<ext>`, magic-bytes-проверка (jpeg/png/webp); общий named volume в compose для api и parser (заодно фото переживают пересборку); без фото — `photo_keys = '{}'` |
| Жизненный цикл | Автовалидация → сразу `active`; пост удалён → `closed`/`gone` тихо (пуша нет); `last_seen_alive` парсер НЕ продлевает — TTL 14 дней потолок (решение Арсения: «полюбасу быстрее сдаются»); голоса «сдана» работают как на любых точках (url_norm есть → голосуемы; скрытие сразу финальное — создателя нет); edits постов игнорируем, text_hash пишем |
| Досыпка | CLI в парсере: `add.py --url https://t.me/<канал>/<id>` / `add.py --text "..." [--photo f.jpg]…` — весь тот же пайплайн, `source='manual'`; отказ автовалидации печатается с причиной, форса нет; оператор — основатель |
| Посев | Досыпку концентрировать в зонах ВШЭ: Курская, Чистые пруды, Китай-город, Бауманская, Таганская и бюджетный пояс восточнее (память seed-plan-hse) |

## 1. Схема — миграция 0009_channel_posts.sql

```sql
CREATE TABLE channel_posts (
    channel    text NOT NULL,
    message_id bigint NOT NULL,
    posted_at  timestamptz NULL,
    text       text NOT NULL,
    text_hash  text NOT NULL,
    photo_urls text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'new'
           CHECK (status IN ('new', 'skipped', 'point_created', 'error')),
    skip_reason  text NULL,
    llm          jsonb NULL,
    point_id     uuid NULL REFERENCES points (id),
    possible_dup boolean NOT NULL DEFAULT false,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_checked_at timestamptz NULL,
    deleted_at      timestamptz NULL,
    PRIMARY KEY (channel, message_id)
);
CREATE INDEX channel_posts_new_idx ON channel_posts (first_seen_at) WHERE status = 'new';
CREATE INDEX channel_posts_point_idx ON channel_posts (point_id) WHERE point_id IS NOT NULL;
```

skip_reason — словарь уровня парсера (не CHECK): `wanted`, `roommate`, `room`,
`other`, `no_zone`, `no_price`, `bad_fields`, `dup`, `forward`, `no_text`,
`llm_error`. §11 спеки продукта пополняется одной строкой про channel_posts
(парсерская таблица, api не читает) и правкой семантики url_norm («носитель
ссылки на оригинал: avito/cian или t.me-пост»).

## 2. Сервис parser — структура

```
parser/
  main.py          — цикл: тик поллинга лент → обработка new → (раз в N тиков) проверка живости
  fetch.py         — feed(channel) -> [RawPost], post_alive(channel, id) -> bool | None
                     (None = не смогли определить: сеть/не-200 — НЕ считать удалением)
  extract.py       — LLM-вызов: classify+extract, схема ответа, промпт со станциями
  pipeline.py      — автовалидация, мапинг зоны, дедуп, фото, INSERT points + channel_posts
  kill.py          — обход активных tg_channel/manual-точек с t.me-url_norm, гашение удалённых
  zones.py         — загрузка станций из api/internal/service/zones/moscow.json (путь в env/volume)
  db.py            — psycopg, DSN из env
  add.py           — CLI досыпки
  config.py        — env: DATABASE_URL, ANTHROPIC_API_KEY, PARSER_CHANNELS (csv),
                     PHOTO_DIR, POLL_MINUTES (дефолт 20), ALIVE_HOURS (дефолт 4)
```

RawPost: channel, message_id, posted_at, text, photo_urls[], is_forward.
Поллинг: каждый канал раз в POLL_MINUTES с джиттером ±20%, честный User-Agent,
backoff при не-200 (канал пропускается до следующего тика, лог warn). Лента
t.me/s отдаёт ~20 последних постов — этого достаточно, глубокая история не нужна.

Каналы старта (PARSER_CHANNELS): arendamsc, ugnest_msk, norealtor,
flats_4_friends, nebabushkin_msk.

## 3. Пайплайн поста

1. Пост уже в channel_posts (PK channel+message_id) → пропуск (идемпотентность).
2. `is_forward` → status='skipped', skip_reason='forward'; пустой текст
   (видео и медиа без подписи — на живой ленте ~40% постов) → status='skipped',
   skip_reason='no_text', ДО всякого вызова LLM.
3. LLM (`claude-sonnet-5`, structured outputs, temperature дефолт):
   системный промпт = роль + правила + список станций (стабильный префикс,
   `cache_control`); ответ:

```json
{
  "kind": "rent_flat | rent_room | wanted | roommate | other",
  "station": "Сокол | null",
  "price_month_total": 85000,
  "rooms": 2,
  "isolated": true,
  "bathrooms": "combined | separate | two | null",
  "utilities": "included | meters | null",
  "deposit": 45000,
  "restrictions": ["no_pets", "no_couples", "no_smoking"],
  "move_in": "2026-09-01 | null",
  "listing_url": "https://avito.ru/... | null"
}
```

   Промпт-правила: цена приводится к total за объект («по 40к на двоих» → 80000);
   диапазон → нижняя граница; станция — одна, ближайшая к жилью, строго из списка.
   Сырой ответ целиком → channel_posts.llm. Сетевые/5xx — ретраи SDK; невалидная
   схема — один повтор; после — status='error' (пост не блокирует остальные).
4. kind ≠ rent_flat → skipped (`wanted`/`roommate`/`room`/`other`).
5. Автовалидация (зеркало user_link минус фото): станция распозналась в id;
   20000 ≤ price_month_total ≤ 300000; rooms null|1..20; deposit null|0..1000000;
   restrictions ⊂ словаря; иначе skipped (`no_zone`/`no_price`/`bad_fields`).
6. url_norm: listing_url → нормализация как validate.go; иначе t.me-ссылка.
   SELECT живой точки с тем же url_norm → skipped 'dup' (+point_id найденной);
   гонку закрывает уникальный индекс (IntegrityError → тот же 'dup').
7. Эвристика possible_dup: живая tg_channel-точка ≤7 дней с тем же (zone_id,
   price_total, rooms) → пометка, НЕ блокировка.
8. Фото: до 5, скачивание с CDN превью, magic bytes, ключи `points/<uuid>.<ext>`
   в PHOTO_DIR; ошибка скачивания одного фото — пропуск фото, не поста.
9. INSERT points: type='flat', status='active', source='tg_channel',
   created_by NULL, zone_id, price_total, rooms, isolated, bathrooms, utilities,
   deposit, restrictions, move_in, photo_keys, url_norm. status='point_created',
   point_id.

## 4. Гашение (kill.py)

Раз в ALIVE_HOURS: точки `status IN ('active','paused') AND source IN
('tg_channel','manual') AND url_norm LIKE 'https://t.me/%'` → embed-проверка.
Однозначный маркер «поста нет» (`tgme_widget_message_error` / «Post not found»)
→ UPDATE points closed/gone + edited_at, channel_posts.deleted_at. Сеть/не-200/
таймаут → last_checked_at, точку не трогать. Пуша нет (как TTL-архив §8).
Темп — 1 запрос в 2–3 сек.

Поведение embed для удалённых постов верифицируется руками в задаче 1 (риск
транспорта); если маркер ненадёжен — гашение остаётся на голосах и TTL 14,
MTProto — отложенный апгрейд за швом fetch.

## 5. Компоуз и окружение

- named volume `photos`, смонтирован в api (PHOTO_DIR) и parser — фото общие
  и переживают пересборку.
- parser выходит из-под profiles → обычный сервис (стартует после postgres;
  локально можно и `python main.py` напрямую с DSN на localhost).
- .env: `ANTHROPIC_API_KEY` (новый), `PARSER_CHANNELS`, `POLL_MINUTES`,
  `ALIVE_HOURS`; .env.example пополняется.
- moscow.json парсер читает из репо (bind-mount или COPY в образ) — источник
  один, генератор зон уже версионирует его.

## 6. Фронт (мелочь)

`openOriginalLabel`: url на t.me → «Открыть пост в Telegram» (сейчас упал бы в
общее «Открыть оригинал» — годится, но t.me-лейбл точнее и работает на этап 0
посева: тап ведёт в канал-источник). Больше фронт не трогаем: карточки flat
уже рендерятся, голоса уже работают.

## 7. Тесты

- Python: pytest; fetch — парсинг сохранённых HTML-фикстур ленты и embed
  (живой/удалённый/пустой); extract — мок anthropic-клиента (схема, правила цены
  — юнитом промпт не проверить, фиксируем контракт разбора); pipeline —
  автовалидация таблицей, url_norm-нормализация байт-в-байт против кейсов из
  Go-теста validate.go (портировать кейсы), дедуп/скипы на тестовой БД
  (TEST_DATABASE_URL, отдельный канал-неймспейс в channel_posts, чистка своих
  строк); kill — маркеры embed из фикстур.
- Go: миграция 0009 применяется/откатывается; больше в api изменений нет
  (кроме .env.example/compose — не тестируются).
- Приёмка на стенде: (1) тик по @arendamsc → строки в channel_posts ровно по
  одному разу, повторный тик дублей не даёт; (2) точки видны в ленте с зоной,
  ценой, фото и строкой «добавлена сегодня»; (3) голос «сдана» ×2 закрывает
  парсерную точку финально; (4) руками удалённый/несуществующий пост →
  closed/gone за форс-тик; (5) avito-ссылка из поста дедупится с пользовательской
  подачей той же ссылки («уже есть, в пуле N»); (6) `add.py --url` создаёт
  manual-точку в ленте; (7) skip-статистика читается одним SELECT.

## Не входит (§J досье)

MTProto/Telethon; очереди/redis; свой мигратор парсера; пуш «появилась квартира
в зоне»; реакция на edits; продление last_seen_alive живым постом; фото-альбомы
>5, видео, перцептивные хэши; комментарии/реакции/просмотры; «ищу соседа» как
контент; realtor; дашборды.

## Правки CLAUDE.md по итогам

- §8: источники — парсер работает (5 каналов, t.me/s-поллинг, Sonnet 5),
  «пост удалён → точка гаснет» реализовано; url_norm-семантика.
- §10: парсер — Python, HTTP-поллинг превью (без MTProto на MVP), LLM Sonnet 5.
- §11: channel_posts одной строкой; url_norm — «avito/cian или t.me-пост».
- §12: 7b сделано; §13 — текущая задача: шаг 8 (блок, пустые состояния,
  src-трекинг, VPS-деплой, полировка → посев).
