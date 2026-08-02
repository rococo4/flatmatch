#!/usr/bin/env bash
#
# Поднимает локальный стенд FlatMatch целиком:
# Postgres → фронт → HTTPS-туннель → api, с новым APP_URL в .env.
#
# Останавливать — scripts/dev-down.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$ROOT/.dev"
mkdir -p "$RUN_DIR"

cd "$ROOT"

say()  { printf '\033[1;34m▸\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

# --- предполётные проверки -------------------------------------------------
# Лучше упасть здесь с внятным текстом, чем на середине запуска.

[ -f "$ROOT/.env" ] || die ".env не найден. Создай: cp .env.example .env и впиши BOT_TOKEN"

grep -q '^BOT_TOKEN=.\+' "$ROOT/.env" \
  || die "BOT_TOKEN в .env пустой. Возьми токен у @BotFather (/newbot)"

for cmd in docker go npm cloudflared; do
  command -v "$cmd" >/dev/null || die "$cmd не установлен"
done

# Занятый порт означает, что стенд уже поднят. Молча убивать чужой процесс
# нельзя: на 8080 может висеть что-то постороннее.
for port in 8080 5173; do
  if lsof -ti ":$port" >/dev/null 2>&1; then
    die "порт $port занят. Если это прошлый запуск — scripts/dev-down.sh"
  fi
done

# --- Docker ----------------------------------------------------------------
if ! docker info >/dev/null 2>&1; then
  say "поднимаю Docker Desktop"
  open -a Docker 2>/dev/null || die "не смог запустить Docker Desktop, запусти вручную"
  for _ in $(seq 1 60); do
    docker info >/dev/null 2>&1 && break
    sleep 2
  done
  docker info >/dev/null 2>&1 || die "Docker так и не поднялся"
fi
ok "Docker готов"

say "поднимаю Postgres"
docker compose up -d postgres >/dev/null
for _ in $(seq 1 30); do
  docker compose ps postgres 2>/dev/null | grep -q healthy && break
  sleep 2
done
docker compose ps postgres | grep -q healthy || die "Postgres не стал healthy"
ok "Postgres healthy"

# --- фронт -----------------------------------------------------------------
# Первым, потому что туннель смотрит именно на него: наружу отдаётся фронт,
# а /api он сам проксирует на бэкенд.
[ -d "$ROOT/web/node_modules" ] || { say "npm install"; (cd "$ROOT/web" && npm install >/dev/null); }

say "запускаю фронт"
(cd "$ROOT/web" && nohup npm run dev >"$RUN_DIR/web.log" 2>&1 & echo $! >"$RUN_DIR/web.pid")
for _ in $(seq 1 30); do
  lsof -ti :5173 >/dev/null 2>&1 && break
  sleep 1
done
lsof -ti :5173 >/dev/null 2>&1 || die "фронт не поднялся, смотри .dev/web.log"
ok "фронт на :5173"

# --- туннель ---------------------------------------------------------------
# Бесплатный quick tunnel регулярно отваливается по таймауту на запросе к
# api.trycloudflare.com — со второй попытки обычно проходит, поэтому три круга.
tunnel_url=""
for attempt in 1 2 3; do
  say "поднимаю туннель (попытка $attempt)"
  nohup cloudflared tunnel --url http://localhost:5173 >"$RUN_DIR/tunnel.log" 2>&1 &
  echo $! >"$RUN_DIR/tunnel.pid"

  for _ in $(seq 1 20); do
    # Отсекаем api.trycloudflare.com — это служебный адрес самого cloudflared,
    # а не выданный нам. У выданного в имени всегда несколько слов через дефис.
    tunnel_url="$(grep -oE 'https://[a-z0-9]+(-[a-z0-9]+)+\.trycloudflare\.com' "$RUN_DIR/tunnel.log" 2>/dev/null | head -1 || true)"
    [ -n "$tunnel_url" ] && break
    sleep 2
  done

  [ -n "$tunnel_url" ] && break
  kill "$(cat "$RUN_DIR/tunnel.pid")" 2>/dev/null || true
done
[ -n "$tunnel_url" ] || die "туннель не поднялся за три попытки, смотри .dev/tunnel.log"
ok "туннель: $tunnel_url"

# --- APP_URL ---------------------------------------------------------------
# Адрес выдаётся заново при каждом запуске, поэтому .env правится каждый раз.
# api читает APP_URL на старте, так что порядок важен: сначала запись, потом api.
if sed --version >/dev/null 2>&1; then
  sed -i "s|^APP_URL=.*|APP_URL=$tunnel_url|" "$ROOT/.env"       # GNU
else
  sed -i '' "s|^APP_URL=.*|APP_URL=$tunnel_url|" "$ROOT/.env"    # BSD/macOS
fi
ok "APP_URL записан в .env"

# --- api -------------------------------------------------------------------
# Миграции применяет сам на старте, отдельного шага нет.
say "запускаю api"
(cd "$ROOT/api" && nohup go run ./cmd/api >"$RUN_DIR/api.log" 2>&1 & echo $! >"$RUN_DIR/api.pid")
for _ in $(seq 1 40); do
  grep -q '"msg":"bot polling started"' "$RUN_DIR/api.log" 2>/dev/null && break
  grep -q 'fatal:' "$RUN_DIR/api.log" 2>/dev/null && die "api упал: $(grep 'fatal:' "$RUN_DIR/api.log" | head -1)"
  sleep 1
done
grep -q '"msg":"bot polling started"' "$RUN_DIR/api.log" 2>/dev/null \
  || die "api не начал опрашивать бота, смотри .dev/api.log"

bot="$(grep -o '"username":"[^"]*"' "$RUN_DIR/api.log" | head -1 | cut -d'"' -f4)"
ok "api слушает :8080, бот $bot"

# --- итог ------------------------------------------------------------------
cat <<EOF

  Mini App:  $tunnel_url
  Бот:       $bot  →  напиши ему /start

  Логи:      .dev/{api,web,tunnel}.log
  Остановить: scripts/dev-down.sh

EOF
