---
name: running-locally
description: Use when the FlatMatch Mini App has to run on this machine — opening it from Telegram, checking /start, reproducing something a user saw, walking through the onboarding again on an account that has already submitted it, or after the cloudflared address changed and login started failing with 401.
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

**Запускать в отдельном окне терминала.** Туннель, dev-сервер фронта и api
остаются детьми той оболочки, из которой стартовал скрипт: закроешь окно —
стенд умрёт целиком, и в Телеграме кнопка поведёт в никуда. Отсюда же правило
для агента: не запускать `dev-up.sh` обычным синхронным вызовом — когда вызов
завершится или будет прерван, вся группа процессов получит сигнал и стенд
ляжет прямо посреди теста. Либо просить человека запустить у себя, либо
запускать фоном (`run_in_background`).

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
| стенд лёг разом, в `api.log` — `shutdown signal received` | скрипт запускали из недолговечной оболочки (вызов агента, закрытое окно) → см. «Быстрый путь» |
| в приложении «анкета на проверке» вместо онбординга | аккаунт уже `pending` → сбросить в draft, ниже |

**Первым делом смотри `.dev/api.log`.** Каждый запрос там одной строкой с
`request_id`, а причина 401 пишется в лог и намеренно не отдаётся клиенту —
в приложении будет просто «Не получилось войти».

## Пройти онбординг заново

Анкета проходится один раз: после отправки аккаунт уезжает в `pending`, шаг 11,
и приложение показывает «анкета на проверке». Второй прогон — только сбросом
статуса, регистрации в продукте нет и завести «ещё один аккаунт» нельзя: вход
по `tg_id`.

```sh
# кто вообще есть в базе
docker compose exec postgres psql -U flatmatch -d flatmatch \
  -c "SELECT tg_id, name, tg_username, status, onboarding_step FROM users"

# вернуть на первый экран, ответы и фото остаются в строке
docker compose exec postgres psql -U flatmatch -d flatmatch \
  -c "UPDATE users SET status='draft', onboarding_step=0 WHERE tg_id=<tg_id>"
```

Ответы при этом сохраняются, и экраны откроются уже заполненными — это нужный
режим, чтобы проверить возврат к брошенному шагу. Для честного «первого
запуска» строку проще удалить, она пересоздастся сама на ближайшем открытии
приложения:

```sh
docker compose exec postgres psql -U flatmatch -d flatmatch \
  -c "DELETE FROM users WHERE tg_id=<tg_id>"
```

В базе обычно два аккаунта: настоящий телеграмный и `777000001` — мок из
`cmd/devinitdata`. Сбрасывать надо тот, которым открываешь приложение.
Из неинтерактивной оболочки — `docker compose exec -T`, иначе не будет tty.

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
