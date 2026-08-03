# FlatMatch

Telegram Mini App для поиска со-съёмщика квартиры в Москве. Не агрегатор
квартир, а поиск людей: «квартиру найдёшь где угодно — мы найдём того, с кем
её снимать».

Спека продукта — [`CLAUDE.md`](./CLAUDE.md), единственный источник правды.
Если код расходится со спекой — прав документ.

Сейчас в репозитории **скелет** (§13): вход по Telegram работает end-to-end,
продуктовой механики ещё нет. Дальше — §12, шаг 2.

## Структура

```
api/       Go, структурированный монолит: SQL руками через pgx, без ORM
web/       React + TypeScript + Vite, @telegram-apps/sdk-react + telegram-ui
parser/    заглушка парсера ТГ-каналов (§12 шаг 7)
migrations в api/migrations, goose, вшиты в бинарь и применяются на старте
```

Бэкенд разложен по слоям, а не по одному пакету на домен:

```
api/internal/
├── entity/                 доменные типы: User, Status, … — ноль зависимостей
├── repo/usersrepo/         Postgres: repo.go, postgres.go, model.go
├── service/users/          бизнес-правила + interface Repository
├── api/http/authmw/        проверка initData → пользователь в контексте
├── api/http/usersapi/      HTTP-хендлеры и DTO
├── tma/ tgbot/             Telegram: проверка initData, Bot API
└── httpx/ config/ postgres/  общая обвязка
```

Правило одно: **импорты текут только внутрь.**

```
api/http/*  →  service/*  →  entity
repo/*      →              →  entity
```

`entity` не знает ни про Postgres, ни про HTTP, ни про Telegram — там нет ни
`db:`, ни `json:`-тегов. Форму доменного типа определяет продукт, а не колонка
и не поле в ответе. У репозитория своя row-структура с `db:`-тегами и явным
`toEntity()`, у API — свои DTO. Это дублирование намеренное: схема БД и формат
ответа меняются, не таща за собой домен.

Сервис объявляет интерфейс своего хранилища у себя (§10, «интерфейсы на
стороне потребителя»), поэтому `service` не импортирует `repo`, а `repo` не
импортирует `service`. Связывает их единственное место — `cmd/api/main.go`.

В `internal/service/` рядом с `users` лежат заготовки — `searches`, `points`,
`pools`, `likes`, `matches`, `notifications`, `moderation`, `zones`. Кода в них
нет, только `doc.go` с зафиксированной зоной ответственности и ссылкой на
параграф спеки; хранилище каждый получит в `internal/repo/<домен>repo/`, когда
дойдёт очередь.

## Что уже работает

- `/start` у бота отдаёт кнопку, открывающую Mini App;
- фронт шлёт initData, api проверяет HMAC, находит-или-создаёт пользователя
  по `tg_id` и отвечает профилем;
- экран «Привет, {имя}» в теме Telegram, светлой и тёмной;
- `GET /healthz` — состояние процесса и базы.

## Перед первым запуском

Нужен токен бота — без него api не стартует, потому что тем же токеном
проверяется подпись initData.

1. Открой [@BotFather](https://t.me/BotFather), отправь `/newbot`, выбери имя
   и username. В ответ придёт токен вида `7000000000:AAH...`.
2. `cp .env.example .env` и вставь токен в `BOT_TOKEN`.

`APP_URL` заполнишь на следующем шаге — это адрес, по которому открывается
Mini App, и Telegram принимает только HTTPS.

## Локальный запуск

Нужны Go 1.26+, Node 22+, Docker, cloudflared.

Одной командой — поднимет Postgres, фронт, туннель, впишет свежий `APP_URL`
в `.env` и запустит api:

```sh
scripts/dev-up.sh     # напечатает адрес Mini App
scripts/dev-down.sh   # остановить
```

Дальше — что за чем происходит и как это сделать руками.

### 1. База

```sh
docker compose up -d postgres
```

Postgres поднимется на `127.0.0.1:5432`. Миграции применит сам api при старте —
отдельного шага нет.

### 2. Туннель для HTTPS

Telegram открывает Mini App только по HTTPS, поэтому локальный `localhost:5173`
нужно выставить наружу.

```sh
brew install cloudflared
cloudflared tunnel --url http://localhost:5173
```

Команда напечатает адрес вида `https://something-random.trycloudflare.com`.
Положи его в `APP_URL` в `.env`. Адрес меняется при каждом перезапуске
туннеля — не забывай обновлять.

Альтернатива: `ngrok http 5173`. Оба домена уже разрешены в
`web/vite.config.ts` (`allowedHosts`).

### 3. Mini App в BotFather

@BotFather → `/mybots` → выбери бота → **Bot Settings** → **Configure Mini App**
→ **Enable Mini App** → вставь адрес туннеля.

### 4. api и фронт

В двух терминалах:

```sh
cd api && go run ./cmd/api
```

```sh
cd web && npm install && npm run dev
```

Теперь напиши боту `/start` — придёт кнопка, она откроет Mini App с экраном
«Привет, {имя}».

### Разработка в обычном браузере

Открывать Telegram ради каждого изменения необязательно: фронт умеет
притворяться клиентом Telegram (`src/telegram/mockEnv.ts`). Но api всё равно
проверяет подпись, поэтому мок нужно снабдить настоящей подписанной initData:

```sh
cd api && go run ./cmd/devinitdata
```

Скопируй вывод в `web/.env.local`:

```
VITE_MOCK_INIT_DATA=user=%7B%22id%22%3A777000001...&hash=...
```

Без этой переменной страница откроется, но покажет 401 — это тоже рабочий
сценарий, если хочешь посмотреть, как выглядит ошибка входа.

Тот же приём работает из консоли:

```sh
cd api
curl -H "Authorization: tma $(go run ./cmd/devinitdata)" localhost:8080/api/v1/me
```

## Тесты и проверки

```sh
cd api && go build ./... && go vet ./... && go test ./...
cd web && npm run lint && npm run build
```

Тесты `internal/tma` покрывают проверку initData: подделанные данные, чужой
токен, протухший и «будущий» `auth_date`, отсутствующие поля, кириллица в
имени. Тесты `internal/service/users` гоняют правила сервиса против фейкового
хранилища. Ни базы, ни сети им не нужно.

### Тесты, которым нужна база

Тесты `internal/repo/...` и `internal/postgres` работают с настоящим Postgres
и по умолчанию пропускаются. Чтобы прогнать их:

```sh
docker compose up -d postgres
cd api
export TEST_DATABASE_URL='postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable'
go test ./internal/repo/... ./internal/postgres/...
```

Они сами применяют миграции и убирают за собой. Отдельную базу заводить не
нужно: тестовый пользователь живёт на отрицательном `tg_id`, которого у
реальных аккаунтов не бывает.

### Миграции вручную

Обычно не требуется — api применяет их при старте. Если нужно откатить:

```sh
go install github.com/pressly/goose/v3/cmd/goose@latest
cd api/migrations
goose postgres 'postgres://flatmatch:flatmatch@localhost:5432/flatmatch?sslmode=disable' status
goose postgres '...' down
```

## Деплой на VPS

Одна машина, docker compose, Caddy впереди (§10). Kubernetes, Redis и очередей
нет и не планируется.

1. Заведи домен и направь его A-записью на IP машины. Telegram требует HTTPS,
   а Caddy выпишет сертификат сам — но только если домен уже резолвится.
2. Поставь Docker:
   ```sh
   curl -fsSL https://get.docker.com | sh
   ```
3. Склонируй репозиторий и заполни `.env`:
   ```sh
   git clone <repo> flatmatch && cd flatmatch
   cp .env.example .env
   ```
   Обязательно поменяй:
   - `BOT_TOKEN` — токен бота;
   - `DOMAIN` — твой домен, под него Caddy получит сертификат;
   - `APP_URL` — `https://<домен>`;
   - `POSTGRES_PASSWORD` — не оставляй значение по умолчанию.
4. Запусти:
   ```sh
   docker compose up -d --build
   ```
   Соберётся три сервиса: `postgres`, `api`, `caddy` (Caddy отдаёт собранный
   фронт и проксирует `/api`). Парсер под профилем и не поднимается.
5. Проверь:
   ```sh
   curl https://<домен>/healthz
   docker compose logs -f api
   ```
6. В BotFather замени адрес Mini App с туннеля на `https://<домен>`.

Обновление — `git pull && docker compose up -d --build`. Миграции применятся
сами при старте api.

### Что где хранится

- данные Postgres — том `postgres_data`;
- сертификаты Caddy — том `caddy_data`; не удаляй его без нужды, иначе на
  каждый передеплой будет запрашиваться новый сертификат и упрёшься в лимиты
  Let's Encrypt.

## Переменные окружения

| Переменная            | Обязательна | По умолчанию | Зачем                                                        |
| --------------------- | ----------- | ------------ | ------------------------------------------------------------ |
| `BOT_TOKEN`           | да          | —            | Bot API и секрет для проверки HMAC у initData                 |
| `APP_URL`             | да\*        | —            | HTTPS-адрес Mini App, на него ведёт кнопка `/start`           |
| `DATABASE_URL`        | да          | —            | Подключение к Postgres                                        |
| `BOT_POLLING_ENABLED` | нет         | `true`       | Выключи, если бот уже запущен в другом процессе               |
| `TMA_INITDATA_TTL`    | нет         | `24h`        | Максимальный возраст initData                                 |
| `API_ADDR`            | нет         | `:8080`      | Адрес прослушивания                                           |
| `LOG_LEVEL`           | нет         | `info`       | `debug` / `info` / `warn` / `error`                           |
| `MODERATION_CHAT_ID`  | нет         | —            | Чат модерации (§5), заготовка                                 |
| `DOMAIN`              | нет         | `localhost`  | Домен для Caddy, только при деплое                            |
| `POSTGRES_*`          | нет         | `flatmatch`  | Учётные данные Postgres в compose                             |

\* Обязательна при `BOT_POLLING_ENABLED=true`, и должна начинаться с `https://`.

Фронтовые переменные — в [`web/.env.example`](./web/.env.example).

## Известные ограничения скелета

- Один процесс api может опрашивать бота через `getUpdates`. Если запустишь
  второй с тем же токеном, Telegram ответит 409 и второй остановится. Для
  локальной разработки параллельно с прод-ботом ставь
  `BOT_POLLING_ENABLED=false`.
- `@telegram-apps/telegram-ui` поддерживает React 18, поэтому React закреплён
  на 18.3. Обновление до 19 ждёт версию UI-кита с его поддержкой.
- `cmd/devinitdata` подписывает данные настоящим токеном и выдаёт любую
  личность. Это инструмент разработки — на сервере он не нужен и не
  запускается.
