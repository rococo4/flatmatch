#!/usr/bin/env bash
#
# Останавливает всё, что поднял scripts/dev-up.sh.
#
# Том с данными Postgres не трогается — `docker compose down` без -v.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$ROOT/.dev"

cd "$ROOT"

say() { printf '\033[1;34m▸\033[0m %s\n' "$*"; }
ok()  { printf '\033[1;32m✓\033[0m %s\n' "$*"; }

for name in api web tunnel; do
  pidfile="$RUN_DIR/$name.pid"
  [ -f "$pidfile" ] || continue

  pid="$(cat "$pidfile")"
  if kill -0 "$pid" 2>/dev/null; then
    # Группа целиком: `go run` и `npm run dev` держат дочерний процесс, который
    # переживёт смерть родителя и оставит порт занятым.
    kill -- "-$(ps -o pgid= "$pid" | tr -d ' ')" 2>/dev/null || kill "$pid" 2>/dev/null
    ok "$name остановлен"
  fi
  rm -f "$pidfile"
done

# Подчищаем то, что осталось от запусков мимо скрипта.
pkill -f "cloudflared tunnel --url http://localhost:5173" 2>/dev/null && ok "лишний cloudflared убран"
for port in 8080 5173; do
  pids="$(lsof -ti ":$port" 2>/dev/null)"
  [ -n "$pids" ] && kill -9 $pids 2>/dev/null && ok "порт $port освобождён"
done

say "останавливаю Postgres"
docker compose down >/dev/null 2>&1 && ok "контейнеры сняты, том с данными на месте"

echo
echo "  Docker Desktop продолжает работать. Выключить: osascript -e 'quit app \"Docker\"'"
echo
