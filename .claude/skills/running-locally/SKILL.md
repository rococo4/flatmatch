---
name: running-locally
description: Use when the FlatMatch Mini App has to run on this machine — opening it from Telegram, checking /start, reproducing something a user saw, or after the cloudflared address changed and login started failing with 401.
---

# Запуск FlatMatch локально

Стенд — четыре части: Postgres, api, dev-сервер фронта и HTTPS-туннель.
Туннель обязателен: Telegram открывает Mini App только по HTTPS, а адрес
выдаётся заново при каждом запуске.

## Быстрый путь

```sh
scripts/dev-up.sh     # поднять всё, напечатает адрес Mini App
scripts/dev-down.sh   # остановить
```

Скрипт сам поднимет Docker Desktop, дождётся healthy у Postgres, запустит
фронт, поднимет туннель, **впишет новый адрес в `.env`** и только потом
запустит api. Логи — в `.dev/{api,web,tunnel}.log`.

После этого напиши боту `/start` — придёт кнопка.

## Порядок важен

Он не случайный, и при ручном запуске его надо повторить:

1. **Postgres** — api применяет миграции на старте и без базы не поднимется.
2. **Фронт на :5173** — туннель смотрит на него, а не на api: наружу отдаётся
   фронт, а `/api` он проксирует на `:8080` сам (`web/vite.config.ts`).
3. **Туннель** — только теперь известен адрес.
4. **`APP_URL` в `.env`** — до запуска api. Он читает переменную один раз, на
   старте, и подставляет её в кнопку `/start`.
5. **api** — последним.

## Что нужно один раз

```sh
cp .env.example .env      # и вписать BOT_TOKEN от @BotFather (/newbot)
brew install cloudflared
```

Без `BOT_TOKEN` api не стартует: тем же токеном проверяется подпись initData,
так что это не опциональная настройка.

## Диагностика

| Симптом | Причина |
|---|---|
| `порт 8080 занят` | предыдущий запуск жив → `scripts/dev-down.sh` |
| `unauthorized · HTTP 401` в приложении | `APP_URL` от старого туннеля, либо api поднят раньше записи адреса → перезапустить |
| `failed to request quick Tunnel: context deadline exceeded` | обычная неудача quick tunnel, скрипт сам делает три попытки |
| api молчит на `/start` | смотреть `.dev/api.log`: нужна строка `bot polling started` |
| `409 Conflict` в логе api | тем же токеном опрашивает второй процесс, живой где-то ещё |

**Первым делом смотри `.dev/api.log`.** Каждый запрос там одной строкой с
`request_id`, а причина 401 пишется в лог и намеренно не отдаётся клиенту —
в приложении будет просто «Не получилось войти».

## Разработка без Telegram

Фронт умеет притворяться клиентом Telegram (`web/src/telegram/mockEnv.ts`), но
api всё равно проверяет подпись. Нужна подписанная initData:

```sh
cd api && go run ./cmd/devinitdata
```

Вывод — в `web/.env.local` как `VITE_MOCK_INIT_DATA`. Тем же способом можно
дёргать api из консоли:

```sh
curl -H "Authorization: tma $(go run ./cmd/devinitdata)" localhost:8080/api/v1/me
```

## Тесты, которым нужна база

По умолчанию пропускаются. Чтобы прогнать:

```sh
docker compose up -d postgres
cd api
TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' \
  go test ./internal/repo/... ./internal/postgres/...
```

## Чего скрипт не делает

- Не выключает Docker Desktop — только контейнеры.
- Не трогает том `postgres_data`: `docker compose down` идёт без `-v`.
  Чтобы схема пересоздалась с нуля, том надо снести руками.
- Не настраивает Mini App в @BotFather. Для кнопки из `/start` это не нужно —
  инлайн-кнопка `web_app` работает с любым HTTPS. Регистрация нужна только для
  кнопки меню и ссылок `t.me/<bot>/app`.
