# parser — квартиры из ТГ-каналов

Отдельный сервис, который наполняет `points` квартирами с рынка (§8, §12 шаг 7b).

## Что уже есть

1. `fetch.py` — HTTP-поллинг ленты `https://t.me/s/<канал>` (без MTProto:
   превью-страницы канала хватает на ~20 последних постов) и проверка жизни
   поста через `https://t.me/<канал>/<id>?embed=1`.
2. `ingest.py` — сырьё поста в таблицу `channel_posts`, идемпотентно по
   `(channel, message_id)`; форварды сразу `skipped/forward`.
3. `main.py` — цикл: раз в `POLL_MINUTES` (джиттер ±20%) инжест всех
   `PARSER_CHANNELS`.

Дальше по плану: LLM-структуризация поста, автовалидация, фото и создание
точек `source='tg_channel'`; гашение точек по удалённым постам; CLI досыпки
`source='manual'`.

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
