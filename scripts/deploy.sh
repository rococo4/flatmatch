#!/usr/bin/env bash
#
# Выкатывает репозиторий на прод: rsync в /opt/flatmatch и пересборка стека.
#
# Хост — алиас из ~/.ssh/config (по умолчанию flatmatch: IP, порт 2222, ключ).
# Секреты не едут: .env живёт только на сервере, скрипт его не трогает.
#
#   scripts/deploy.sh            # на flatmatch
#   scripts/deploy.sh other-host # на другой хост с тем же раскладом
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOST="${1:-flatmatch}"
REMOTE_DIR="/opt/flatmatch"

cd "$ROOT"

say() { printf '\033[1;34m▸\033[0m %s\n' "$*"; }
ok()  { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

for cmd in rsync ssh; do
  command -v "$cmd" >/dev/null || die "$cmd не установлен"
done

ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" true 2>/dev/null \
  || die "нет доступа по ssh к $HOST. Проверь ~/.ssh/config и ключ"

# Исключения: чужие сборки, локальные стенды и всё секретное. .env отдельной
# строкой — на сервере он свой, перезаписать его отсюда значит уронить прод.
# Исключённое --delete не трогает, поэтому здесь же серверное: .env и backups
# локально не существуют, и без этой строки выкатка стирала бы дампы.
EXCLUDES=(
  .git node_modules .venv .dev api/.data web/dist .superpowers .env
  '*.png' .claude web/.env.local __pycache__ .pytest_cache backups
)

args=()
for pattern in "${EXCLUDES[@]}"; do
  args+=(--exclude "$pattern")
done

say "синхронизирую репозиторий на $HOST:$REMOTE_DIR"
rsync -az --delete "${args[@]}" -e "ssh" ./ "$HOST:$REMOTE_DIR/"
ok "файлы на месте"

say "пересобираю стек"
ssh "$HOST" "cd $REMOTE_DIR && docker compose up -d --build && docker image prune -f && docker compose ps"
ok "выкачено"
