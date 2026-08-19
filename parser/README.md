# parser — квартиры из ТГ-каналов

Отдельный сервис, который наполняет `points` квартирами с рынка (§8, §12 шаг 7b).

## Что уже есть

1. `fetch.py` — HTTP-поллинг ленты `https://t.me/s/<канал>` (без MTProto:
   превью-страницы канала хватает на ~20 последних постов) и проверка жизни
   поста через `https://t.me/<канал>/<id>?embed=1`.
2. `ingest.py` — сырьё поста в таблицу `channel_posts`, идемпотентно по
   `(channel, message_id)`; форварды сразу `skipped/forward`.
3. `extract.py` — один вызов `claude-sonnet-5` на пост: классификация
   (`rent_flat` / `rent_room` / `wanted` / `roommate` / `other`) и поля
   объявления по json-схеме. Список станций едет стабильным префиксом
   системного промпта под `cache_control`.
4. `zones.py` — станции из `moscow.json` (`ZONES_PATH`): имя из ответа LLM →
   `zone_id`, регистр и «ё» не важны.
5. `norm.py` — нормализация ссылки avito/cian, порт `normalizeURL` из
   `api/internal/service/points/validate.go`: одна ссылка обязана давать одну
   строку `url_norm` в обоих сервисах, иначе дедуп молча ломается.
6. `pipeline.py` — пост → точка: автовалидация (зона, цена 20–300к, комнаты,
   залог, ограничения), `url_norm` (ссылка из поста, иначе `t.me/<канал>/<id>`),
   дедуп по живой точке с тем же `url_norm`, пометка `possible_dup` по
   совпадению зоны, цены и комнат за неделю, INSERT `points`
   `source='tg_channel'` сразу `active`.
7. `kill.py` — раз в `ALIVE_HOURS` обход живых `tg_channel`/`manual`-точек с
   t.me-ссылкой: `fetch.post_alive` однозначно говорит «поста нет» →
   `points` тихо в `closed/gone` + `channel_posts.deleted_at`; неопределённый
   ответ (сеть, не-200) точку не трогает, только отметка `last_checked_at`.
8. `main.py` — цикл: раз в `POLL_MINUTES` (джиттер ±20%) инжест всех
   `PARSER_CHANNELS`, разбор новых постов, затем — раз в `ALIVE_HOURS` —
   гашение (`kill.sweep`).

Каждый пост заканчивается ровно одной записью в `channel_posts`: точкой
(`point_created`), скипом с причиной (`forward`, `no_text`, `wanted`,
`roommate`, `room`, `other`, `no_zone`, `no_price`, `bad_fields`, `dup`) или
`error`/`llm_error` — когда модель дважды подряд ответила не по схеме. Сеть и
лимиты постов не жгут: такой пост остаётся `new` и уедет в следующий тик.

Дальше по плану: CLI досыпки `source='manual'`.

Авито и Циан не парсим — оттуда приходят только пользовательские ссылки
(`source='user_link'`), которые люди добавляют под себя.

## Границы

С бэкендом общается **только через базу** (§10). Общего кода, общих очередей
и HTTP-вызовов между `api` и `parser` нет и не планируется — это единственный
интерфейс между ними.

## Запуск

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt -r requirements-dev.txt
DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' .venv/bin/python main.py
```

В докере сервис пока под профилем: `docker compose --profile parser up -d`.

Переменные окружения (`config.py`): `DATABASE_URL` (обязательна),
`ANTHROPIC_API_KEY`, `PARSER_CHANNELS` (csv, дефолт — пять каналов старта),
`PHOTO_DIR`, `ZONES_PATH`, `POLL_MINUTES` (20), `ALIVE_HOURS` (4).

## Тесты

```sh
.venv/bin/python -m pytest -q
```

Сетевых тестов нет: разбор проверяется на сохранённых фикстурах
`tests/fixtures/`. Тесты инжеста ходят в базу из `TEST_DATABASE_URL` (без неё
пропускаются) и трогают только каналы с префиксом `test-`.
