# FlatMatch

Telegram Mini App для поиска со-съёмщика квартиры в Москве. Не агрегатор
квартир, а поиск людей: «квартиру найдёшь где угодно — мы найдём того, с кем
её снимать».

Спека продукта — [`CLAUDE.md`](./CLAUDE.md), единственный источник правды.
Если код расходится со спекой — прав документ.

Продукт собран по §12 целиком: онбординг и модерация, поиски и зонные пулы,
дорогой лайк и мэтч, точки с квартирами, пуши, свежесть, парсер ТГ-каналов с
досыпкой, блок с жалобой, src-трекинг и пустые состояния. Живёт на
[flatmatch.ru](https://flatmatch.ru) (см. «Деплой»). Дальше — §13: посев.

## Структура

```
api/       Go, структурированный монолит: SQL руками через pgx, без ORM
web/       React + TypeScript + Vite, @telegram-apps/sdk-react + telegram-ui
parser/    Python: парсер ТГ-каналов аренды + LLM, досыпка add.py (§12 шаг 7b)
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

Заготовок в `internal/service/` не осталось: `notifications` — живой пакет
(§7): доменные сервисы пишут события в лог `notificationsrepo`, фоновый
воркер в том же процессе группирует их и шлёт через бота с диплинками, а
ответы кнопками («ещё ищешь?», опрос) возвращаются колбэками через `tgbot`.

## Что уже работает

- `/start` у бота отдаёт кнопку, открывающую Mini App; фронт шлёт initData,
  api проверяет HMAC, находит-или-создаёт пользователя по `tg_id`;
- онбординг 12 шагов → модерация анкеты в чате основателя → `active`;
- поиски, зонные пулы с секциями и вердикт-чипами, лента search-first;
- профиль человека → дорогой лайк (5 слотов / 24 ч) → мэтч с саммари;
- точки: квартиры с рынка и «сосед к себе», квартирные пулы, свежесть-голоса;
- пуши через бота: воркер, агрегация, кнопки-ответы, диплинки;
- парсер ТГ-каналов с LLM-структуризацией и досыпкой (`parser/`);
- блок и жалоба в модер-чат, `startapp=src_*` в `users.source`;
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

### Сидер фейковых людей и точек

Пустая база — пустые экраны: промо-блок ленты считает ноль человек, у зонного
пула нет ни секций, ни вердикт-чипов, лента без квартир, и смотреть на них не
на чем.

```sh
cd api && go run ./cmd/devseed
```

Двадцать анкет в статусе `active`, с одобренной фото-заглушкой и одним
активным поиском у каждой. Зоны подобраны вокруг Хамовников: против поиска
«Хамовники, 35 000–45 000, сентябрь 2026» выходит десять полных совпадений,
четыре частичных и двое не совпавших — плюс люди в зонах, которые с
Хамовниками не пересекаются, чтобы граница пула тоже была видна. Отдельно там
есть «только свой пол» в обе стороны (§4) и поиск с одной лишь верхней
границей цены — карточка с чипами «не указана».

Плюс четыре квартиры и одно «к себе» (§2.2). Три рыночные — активные, без
автора, у метро Фрунзенская (в пуле пятеро), Спортивная (свежая, в пуле двое)
и Сокольники (три недели, в пуле трое): у первых двух зона попадает в
Хамовники, у третьей — только в красную ветку, так что видно и ранжирование
ленты, и границу зоны. «К себе» — комната Кати у Спортивной, 45 000 за долю,
с одним откликом от Олега: у Кати это «N откликов», у неё же во входящих —
«на твою комнату».

Заодно, если в базе уже есть dev-аккаунт `777000001` (он появляется от первого
входа через `devinitdata`), Катя и Ира лайкают его — чтобы «Тебя лайкнули» было
не пустым без второго телеграма, — и ему заводится своя квартира в статусе
«на проверке»: она и лежит в «Присмотренных», и ждёт вердикта в чате модерации.
Мэтч сид не делает специально: ответить на входящий и увидеть мэтч — ручной
сценарий. Сам dev-аккаунт сидер не создаёт; нет его в базе — блок молча
пропускается, и в выводе об этом строчка.

Сидер идемпотентен: `tg_id` фиксированы начиная с `900000001`, анкеты пишутся
через `ON CONFLICT (tg_id) DO UPDATE`, поиски каждый раз перезаписываются,
лайки и входы в пул — `ON CONFLICT DO NOTHING`, у точек фиксированные `id` с
префиксом `5eed`. Второй прогон приводит базу в то же состояние, а не плодит
копии — правь таблицы в `cmd/devseed/main.go` и запускай снова.

Снести — по тому же диапазону `tg_id` и префиксу точек, в порядке ссылок:
мэтчи, лайки, пулы, точки, поиски, люди. Мэтч и лайк называют сид-человека
любой из двух сторон, поэтому чистятся по обеим.

```sh
docker compose exec postgres psql -U flatmatch -d flatmatch \
  -c 'DELETE FROM matches USING users u WHERE u.tg_id >= 900000001 AND (a_id = u.id OR b_id = u.id)' \
  -c 'DELETE FROM likes_users USING users u WHERE u.tg_id >= 900000001 AND (from_id = u.id OR to_id = u.id)' \
  -c "DELETE FROM likes_points WHERE point_id IN (SELECT id FROM points WHERE id::text LIKE '5eed%') OR user_id IN (SELECT id FROM users WHERE tg_id >= 900000001)" \
  -c "DELETE FROM points WHERE id::text LIKE '5eed%'" \
  -c 'DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id >= 900000001)' \
  -c 'DELETE FROM users WHERE tg_id >= 900000001'
```

Против чужой базы сидер не запустится: хост в `DATABASE_URL` должен быть
локальным (`localhost`, `127.0.0.1`, `postgres`), иначе процесс падает с
отказом. Двадцать выдуманных анкет перед живыми людьми — не та ошибка,
которую замечают на следующий день.

### Справочник зон

Районы, станции и ветки метро лежат одним файлом
`api/internal/service/zones/moscow.json`, вшитым в бинарь (§8). Он уже в
репозитории — обычно трогать его не нужно. Перегенерировать стоит, когда
открылась станция или ветка:

```sh
curl -sSLo /tmp/mo.geojson https://gis-lab.info/data/mos-adm/mo.geojson
cd api && go run ./cmd/genzones -out internal/service/zones/moscow.json
```

Станции и ветки генератор берёт из `https://api.hh.ru/metro/1`, границы
районов — из того самого `mo.geojson`, и привязывает станцию к району по
геометрии с запасом в 400 метров: «Парк культуры» — станция Хамовников, с
какой стороны улицы ни стоял бы вестибюль.

Склонения («в Хамовниках», «пул Хамовников») морфологией не считаются — это
ручной файл `api/cmd/genzones/declensions.json`. На район, которого в нём нет,
генерация падает с требованием дописать его руками; молча ронять зону она не
станет.

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

## Деплой

Прод уже поднят: **[flatmatch.ru](https://flatmatch.ru)**, одна машина
(Timeweb Cloud, Амстердам, Ubuntu 24.04), docker compose, Caddy впереди с
сертификатом от Let's Encrypt (§10). Kubernetes, Redis и очередей нет и не
планируется.

Доступ — по алиасу из `~/.ssh/config` (ssh слушает нестандартный порт 2222,
22-й оставлен открытым на случай, если конфиг поедет):

```
Host flatmatch
    HostName 72.56.92.50
    Port 2222
    User root
    IdentityFile ~/.ssh/flatmatch
```

Репозиторий на сервере — `/opt/flatmatch`, там же поднят стек: `postgres`,
`api`, `parser`, `caddy` (Caddy отдаёт собранный фронт и проксирует `/api`).
В BotFather у бота прописан `https://flatmatch.ru`.

### Выкатка

```sh
scripts/deploy.sh          # хост flatmatch по умолчанию
scripts/deploy.sh <host>   # другой алиас, если машин станет две
```

Скрипт делает `rsync -az --delete` репозитория в `/opt/flatmatch` (мимо
`.git`, `node_modules`, `.venv`, `web/dist`, `.env` и прочего локального),
затем по ssh — `docker compose up -d --build`, `docker image prune -f` и
печатает `docker compose ps`. Миграции применит сам api при старте, отдельного
шага нет. Фронт собирается в образе Caddy, локальный `web/dist` не едет.

**`.env` живёт только на сервере** — `/opt/flatmatch/.env`, `chmod 600`, в git
его нет и rsync его не трогает (иначе локальный файл с туннельным `APP_URL`
затирал бы прод). Меняются переменные руками на месте:

```sh
ssh flatmatch
nano /opt/flatmatch/.env
cd /opt/flatmatch && docker compose up -d
```

На проде от локального `.env` отличаются `BOT_TOKEN` (боевой бот), `DOMAIN` и
`APP_URL` (`flatmatch.ru`), `POSTGRES_PASSWORD` (не дефолтный),
`MODERATION_CHAT_ID` и `ANTHROPIC_API_KEY` для парсера.

### Логи и здоровье

```sh
curl https://flatmatch.ru/healthz
ssh flatmatch 'cd /opt/flatmatch && docker compose logs -f api'
ssh flatmatch 'cd /opt/flatmatch && docker compose logs -f parser'
ssh flatmatch 'cd /opt/flatmatch && docker compose ps'
```

### Бэкапы

`scripts/backup.sh` уезжает на сервер тем же rsync и запускается там: читает
`/opt/flatmatch/.env`, снимает `pg_dump` через `docker compose exec -T
postgres` и кладёт gzip в `/opt/flatmatch/backups/flatmatch-<дата>.sql.gz`,
удаляя всё старше семи дней. В кроне (`crontab -e` на сервере):

```
0 4 * * * /opt/flatmatch/scripts/backup.sh >> /var/log/flatmatch-backup.log 2>&1
```

Восстановление — из того же дампа:

```sh
gunzip -c backups/flatmatch-2026-08-21.sql.gz \
  | docker compose exec -T postgres psql -U flatmatch -d flatmatch
```

### Что где хранится

- данные Postgres — том `postgres_data`;
- фото анкет и точек — том `photos`, общий у `api` и `parser`;
- сертификаты Caddy — том `caddy_data`; не удаляй его без нужды, иначе на
  каждый передеплой будет запрашиваться новый сертификат и упрёшься в лимиты
  Let's Encrypt.

### Если машину поднимают заново

1. Домен A-записью на IP: Caddy выпишет сертификат сам, но только если домен
   уже резолвится.
2. `curl -fsSL https://get.docker.com | sh`.
3. `ufw allow 22,2222,80,443`, ssh на 2222, ключ `~/.ssh/flatmatch`.
4. `mkdir -p /opt/flatmatch`, `scripts/deploy.sh` с локальной машины, затем
   `.env` руками на сервере (шаблон — `.env.example`) и ещё раз
   `docker compose up -d --build`.
5. Адрес Mini App в BotFather — `https://<домен>`.

## Переменные окружения

| Переменная            | Обязательна | По умолчанию | Зачем                                                        |
| --------------------- | ----------- | ------------ | ------------------------------------------------------------ |
| `BOT_TOKEN`           | да          | —            | Bot API и секрет для проверки HMAC у initData                 |
| `APP_URL`             | да\*        | —            | HTTPS-адрес Mini App, на него ведёт кнопка `/start`           |
| `DATABASE_URL`        | да          | —            | Подключение к Postgres                                        |
| `BOT_POLLING_ENABLED` | нет         | `true`       | Выключи, если бот уже запущен в другом процессе               |
| `PUSH_WORKER_ENABLED` | нет         | `true`       | Фоновый воркер пушей (§7); без поллинга бота вопросы с кнопками не задаются |
| `PUSH_TICK`           | нет         | `30s`        | Как часто воркер разбирает очередь                            |
| `TMA_INITDATA_TTL`    | нет         | `24h`        | Максимальный возраст initData                                 |
| `API_ADDR`            | нет         | `:8080`      | Адрес прослушивания                                           |
| `LOG_LEVEL`           | нет         | `info`       | `debug` / `info` / `warn` / `error`                           |
| `MODERATION_CHAT_ID`  | нет         | —            | Чат, куда бот шлёт анкеты и точки на вердикт (§5)             |
| `DOMAIN`              | нет         | `localhost`  | Домен для Caddy, только при деплое                            |
| `POSTGRES_*`          | нет         | `flatmatch`  | Учётные данные Postgres в compose                             |

\* Обязательна при `BOT_POLLING_ENABLED=true` или `PUSH_WORKER_ENABLED=true`
(кнопки пушей — тоже Mini App), и должна начинаться с `https://`.

`MODERATION_CHAT_ID` на локальном стенде — твой личный чат с ботом (напиши ему
что-нибудь и возьми `chat.id` из
`curl "https://api.telegram.org/bot$BOT_TOKEN/getUpdates"`) или тестовая
группа, куда бот добавлен. Пусто — не ошибка: вердикты слать некуда, анкеты и
точки просто копятся в `pending`. Когда чат появится, накопленную очередь
поднимает команда `/queue`, отправленная в этот же чат.

Фронтовые переменные — в [`web/.env.example`](./web/.env.example),
переменные парсера (`ANTHROPIC_API_KEY`, `PARSER_CHANNELS`, `POLL_MINUTES`,
`ALIVE_HOURS`) — в [`parser/README.md`](./parser/README.md).

## Известные ограничения

- Один процесс api может опрашивать бота через `getUpdates`. Если запустишь
  второй с тем же токеном, Telegram ответит 409 и второй остановится. Для
  локальной разработки параллельно с прод-ботом ставь
  `BOT_POLLING_ENABLED=false`.
- `@telegram-apps/telegram-ui` поддерживает React 18, поэтому React закреплён
  на 18.3. Обновление до 19 ждёт версию UI-кита с его поддержкой.
- `cmd/devinitdata` подписывает данные настоящим токеном и выдаёт любую
  личность. Это инструмент разработки — на сервере он не нужен и не
  запускается.
