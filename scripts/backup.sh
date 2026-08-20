#!/usr/bin/env bash
#
# Дамп базы. Запускается НА СЕРВЕРЕ (кроном раз в сутки, 0 4 * * *):
#
#   0 4 * * * /opt/flatmatch/scripts/backup.sh >> /var/log/flatmatch-backup.log 2>&1
#
# Кладёт gzip в /opt/flatmatch/backups и стирает всё старше семи дней.
set -euo pipefail

ROOT="/opt/flatmatch"
BACKUP_DIR="$ROOT/backups"
KEEP_DAYS=7

cd "$ROOT"

die() { printf '✗ %s\n' "$*" >&2; exit 1; }

[ -f "$ROOT/.env" ] || die ".env не найден в $ROOT"

set -a
# shellcheck disable=SC1091
. "$ROOT/.env"
set +a

USER_NAME="${POSTGRES_USER:-flatmatch}"
DB_NAME="${POSTGRES_DB:-flatmatch}"
FILE="$BACKUP_DIR/flatmatch-$(date +%F).sql.gz"

mkdir -p "$BACKUP_DIR"

docker compose exec -T postgres pg_dump -U "$USER_NAME" "$DB_NAME" | gzip > "$FILE"
[ -s "$FILE" ] || die "дамп пустой: $FILE"

find "$BACKUP_DIR" -name 'flatmatch-*.sql.gz' -mtime +"$KEEP_DAYS" -delete

printf '✓ %s · %s\n' "$FILE" "$(du -h "$FILE" | cut -f1)"
