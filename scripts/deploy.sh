#!/usr/bin/env bash
#
# Выкатывает прод. Сервер — клон GitHub-репозитория, поэтому выкатка = подтянуть
# origin/main и пересобрать стек. Локальное дерево на сервер НЕ едет: что не
# закоммичено и не запушено, то не выкатится — скрипт это проверяет до ssh.
#
#   scripts/deploy.sh            # на flatmatch (алиас из ~/.ssh/config)
#   scripts/deploy.sh other-host # другой хост с тем же раскладом
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOST="${1:-flatmatch}"
REMOTE_DIR="/opt/flatmatch"
BRANCH="main"

cd "$ROOT"

say() { printf '\033[1;34m▸\033[0m %s\n' "$*"; }
ok()  { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

for cmd in git ssh; do
  command -v "$cmd" >/dev/null || die "$cmd не установлен"
done

current="$(git branch --show-current)"
[ "$current" = "$BRANCH" ] || die "выкатываем только $BRANCH, а сейчас $current"
[ -z "$(git status --porcelain --untracked-files=no)" ] \
  || die "есть незакоммиченные изменения — закоммить или отложи (git stash)"

git fetch -q origin "$BRANCH"
local_sha="$(git rev-parse HEAD)"
remote_sha="$(git rev-parse "origin/$BRANCH")"
[ "$local_sha" = "$remote_sha" ] \
  || die "локальный $BRANCH (${local_sha:0:7}) не совпадает с origin/$BRANCH (${remote_sha:0:7}) — git push или git pull"

ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" true 2>/dev/null \
  || die "нет доступа по ssh к $HOST. Проверь ~/.ssh/config и ключ"

say "выкатываю $(git log -1 --format='%h %s') на $HOST"
ssh "$HOST" "set -e
  cd $REMOTE_DIR
  git fetch -q origin $BRANCH
  git reset -q --hard origin/$BRANCH
  echo \"на сервере: \$(git log -1 --format='%h %s')\"
  docker compose up -d --build
  docker image prune -f
  docker compose ps"
ok "выкачено"
