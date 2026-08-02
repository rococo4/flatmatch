"""Заглушка парсера ТГ-каналов аренды (§8).

Реализация — шаг 7 в §12. Сейчас сервис только подтверждает, что доезжает до
базы: если он падает здесь, значит проблема в инфраструктуре, а не в парсинге.
"""

import os
import sys
import time


def main() -> int:
    dsn = os.environ.get("DATABASE_URL")
    if not dsn:
        print("DATABASE_URL не задан", file=sys.stderr)
        return 1

    # Пароль в логи не пишем.
    where = dsn.split("@")[-1] if "@" in dsn else dsn
    print(f"parser: заглушка, база {where}", flush=True)
    print("parser: реализация — §12 шаг 7, см. parser/README.md", flush=True)

    # Не выходим сразу: restart: unless-stopped иначе крутил бы контейнер
    # в бесконечном цикле перезапусков.
    while True:
        time.sleep(3600)


if __name__ == "__main__":
    sys.exit(main())
