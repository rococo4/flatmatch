"""Postgres — единственный интерфейс парсера с бэком (§10)."""

import psycopg


def connect(dsn: str) -> psycopg.Connection:
    # autocommit: каждый пост — самостоятельная запись, длинных транзакций тут нет.
    return psycopg.connect(dsn, autocommit=True)


def redacted(dsn: str) -> str:
    # Пароль в логи не пишем.
    return dsn.split("@")[-1] if "@" in dsn else dsn
