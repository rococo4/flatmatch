"""Парсер ТГ-каналов аренды (§8): цикл поллинга лент.

Тик раз в POLL_MINUTES с джиттером ±20%: инжест лент всех PARSER_CHANNELS в
channel_posts. Разбор новых постов LLM и гашение удалённых подключаются
следующими шагами — швы process_new() и check_alive() уже вызываются.
"""

import random
import sys
import time

import httpx

import db
import fetch
import ingest
from config import Config

CHANNEL_PAUSE = 3.0
JITTER = 0.2


def main() -> int:
    try:
        cfg = Config.from_env()
    except (RuntimeError, ValueError) as err:
        log(f"конфигурация: {err}", stream=sys.stderr)
        return 1

    log(f"старт, база {db.redacted(cfg.database_url)}")
    log(f"каналы: {', '.join(cfg.channels)}; тик {cfg.poll_minutes} мин")

    with db.connect(cfg.database_url) as conn, fetch.client() as client:
        while True:
            tick(conn, client, cfg)
            time.sleep(next_delay(cfg.poll_minutes))


def tick(conn, client: httpx.Client, cfg: Config) -> None:
    for i, channel in enumerate(cfg.channels):
        if i:
            # Не долбим t.me пачкой запросов подряд.
            time.sleep(CHANNEL_PAUSE)
        try:
            stored = ingest.ingest_channel(conn, client, channel)
        except httpx.HTTPError as err:
            log(f"канал {channel} пропущен до следующего тика: {err}", stream=sys.stderr)
            continue
        log(f"канал {channel}: новых постов {stored}")
    process_new(conn, cfg)
    check_alive(conn, client, cfg)


def process_new(conn, cfg: Config) -> None:
    # Шов: LLM-разбор новых постов и создание точек — следующий шаг.
    return


def check_alive(conn, client: httpx.Client, cfg: Config) -> None:
    # Шов: обход живости t.me-точек раз в ALIVE_HOURS — следующий шаг.
    return


def next_delay(poll_minutes: int) -> float:
    base = poll_minutes * 60
    return base * random.uniform(1 - JITTER, 1 + JITTER)


def log(message: str, stream=sys.stdout) -> None:
    print(f"parser: {message}", file=stream, flush=True)


if __name__ == "__main__":
    sys.exit(main())
