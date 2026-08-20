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

# Из .env нужны два имени; целиком его не source-им: пароль со спецсимволами
# превратился бы в команду.
env_value() { sed -n "s/^$1=//p" "$ROOT/.env" | tail -n 1; }

USER_NAME="$(env_value POSTGRES_USER)"; USER_NAME="${USER_NAME:-flatmatch}"
DB_NAME="$(env_value POSTGRES_DB)";     DB_NAME="${DB_NAME:-flatmatch}"
FILE="$BACKUP_DIR/flatmatch-$(date +%F).sql.gz"

mkdir -p "$BACKUP_DIR"

trap 'rm -f "$FILE"; die "дамп не снялся: $FILE удалён"' ERR
docker compose exec -T postgres pg_dump -U "$USER_NAME" "$DB_NAME" | gzip > "$FILE"
trap - ERR

find "$BACKUP_DIR" -name 'flatmatch-*.sql.gz' -mtime +"$KEEP_DAYS" -delete

printf '✓ %s · %s\n' "$FILE" "$(du -h "$FILE" | cut -f1)"
