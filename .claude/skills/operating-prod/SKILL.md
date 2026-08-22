---
name: operating-prod
description: Use when a task touches the live FlatMatch server (flatmatch.ru) — deploying a change, reading production logs, querying or editing the production database, restoring a backup, figuring out why something a real user saw is broken, changing server-side config (.env, Caddy), or when the server looks down.
---

# Прод FlatMatch

Одна машина: Timeweb VPS, `https://flatmatch.ru`. `/opt/flatmatch` — git-клон
`rococo4/flatmatch` (только `origin/main`) по read-only deploy-ключу, стек
`docker compose`: `postgres`, `api`, `parser`, `caddy`. Подробная справка —
README, раздел «Деплой»; здесь — порядок действий и границы.

## Доступ

- `ssh flatmatch` — алиас из `~/.ssh/config` (порт 2222; 22 из сети
  основателя закрыт, это не поломка).
- Всё на сервере — из `/opt/flatmatch`, compose v2 слитно: `docker compose …`.
- База снаружи закрыта, ходить через контейнер:
  `docker compose exec -T postgres psql -U flatmatch -d flatmatch -c "…"`.
  Многострочный SQL — heredoc; `$$` внутри ssh-строки не использовать.
- Образ `api` — distroless, `docker compose exec api ls` не работает. Файлы
  тома смотреть с хоста; тома называются `flatmatch_photos`,
  `flatmatch_postgres_data`, `flatmatch_caddy_data`, `flatmatch_caddy_config`
  (проверить: `docker volume ls`):
  `ls "$(docker volume inspect flatmatch_photos -f '{{.Mountpoint}}')/photos"`.
- `.env` живёт только на сервере (`/opt/flatmatch/.env`, 600): правка на
  месте + `docker compose up -d` (пересборка не нужна). Значения в чат,
  коммиты и память не копировать.
- Всё остальное запечено в образы на `docker build`: `Caddyfile` и фронт — в
  образ `caddy`, код — в `api` и `parser`. Любая правка из репозитория
  доезжает только через `scripts/deploy.sh` (он собирает); голый рестарт
  контейнера её не подхватит.

## Выкатка

Единственный путь: коммит → `git push origin main` → `scripts/deploy.sh`.
Скрипт откажется, если ветка не `main`, есть незакоммиченное или
`HEAD ≠ origin/main` — это не обходить, а починить (`git push`). На сервере
он делает `git reset --hard origin/main` и `docker compose up -d --build`;
контейнеры с неизменившимся образом не перезапускаются — норма.

После выкатки:

```sh
curl -s -o /dev/null -w '%{http_code}\n' https://flatmatch.ru/healthz   # 200
ssh flatmatch 'cd /opt/flatmatch && docker compose ps'                   # все Up
ssh flatmatch 'cd /opt/flatmatch && docker compose logs --since 2m api | grep -iE "error|panic|migrated"'
```

Миграции применяет сам `api` на старте (в логе `schema migrated`).

## База: что можно руками

| Можно | Как |
|---|---|
| Смотреть | любой `SELECT` |
| Точечно поправить поле, которого нет в UI, или когда пользователь просит за него | `UPDATE … WHERE id = '<uuid>' AND <поле> = '<старое>' RETURNING …` — одна строка, по id |
| Метрика посева | `select source, count(*) from users group by 1` |

Нельзя:

- **Строка основателя (tg_id 188993596) — только чтение.** Свой профиль он
  правит в приложении; просьбу «поменяй мне через SQL» переводить в «поменяй
  в приложении».
- Модерация (`status`, `photo_status`, `about_status`, точки `pending`) —
  только кнопки бота в модер-чате; `/queue` досылает накопленное. Массовый
  перевод `pending → active` — отказ со ссылкой на §5 CLAUDE.md.
- `UPDATE`/`DELETE` без `WHERE … id = …` — стоп, вопрос основателю.
- Лайки и мэтчи не удалять (§2.3: необратимы).

## Бэкапы и восстановление

Крон 04:00 → `/opt/flatmatch/backups/flatmatch-<дата>.sql.gz`, хранятся
7 дней; вручную — `./scripts/backup.sh` на сервере. Дамп не покрывает том
`photos`.

Восстановление теряет всё, что было после дампа → явное «да» от основателя
и свежий `backup.sh` перед сносом. Только в пустую базу, остановив клиентов:

```sh
docker compose stop api parser
docker compose exec -T postgres dropdb --force -U flatmatch flatmatch
docker compose exec -T postgres createdb -U flatmatch flatmatch
gunzip -c backups/flatmatch-YYYY-MM-DD.sql.gz \
  | docker compose exec -T postgres psql -U flatmatch -d flatmatch -v ON_ERROR_STOP=1
docker compose start api parser
```

## Симптом → первая проверка

| Симптом | Куда смотреть |
|---|---|
| Картинки не грузятся | `curl -sI https://flatmatch.ru/media/<photo_key>`: `text/html` вместо `image/jpeg` — Caddy не проксирует `/media`; 404 — файла нет в томе; дальше лог `caddy` |
| У всех 401 на входе | `BOT_TOKEN` и `APP_URL` в `.env`; лог `api` по `initdata` |
| Пуши не приходят | `notifications` (`sent_at`, `failed_at`, `attempts`), `users.push_blocked_at`, лог `api` по `push` |
| Парсер не даёт квартир | `docker compose logs --since 1h parser`; `select status, skip_reason, count(*) from channel_posts group by 1,2` |
| HTTPS/сертификат | лог `caddy`; том `caddy_data` не удалять |
| Контейнер в `Restarting` | `docker compose logs --tail 100 <svc>`; `df -h` |

## Красные флаги

rsync на сервер · правка файлов в `/opt/flatmatch` руками (следующая
выкатка сотрёт) · `docker compose down -v` (снесёт базу и фото) · `git push`
с сервера (ключ read-only, и незачем) · выкатка незапушенного · `UPDATE
users` по строке основателя · SQL вместо кнопок модерации · локальный `api`
с `BOT_POLLING_ENABLED=true` (отберёт апдейты у прода).
