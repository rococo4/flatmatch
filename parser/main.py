"""Парсер ТГ-каналов аренды (§8): цикл поллинга лент.

Тик раз в POLL_MINUTES с джиттером ±20%: инжест лент всех PARSER_CHANNELS в
channel_posts, затем разбор новых постов LLM в точки, затем — раз в
ALIVE_HOURS — гашение точек с удалённым постом (kill.sweep).
"""

import random
import sys
import time
from datetime import datetime, timedelta, timezone

import anthropic
import httpx

import db
import fetch
import ingest
import kill
import pipeline
from config import Config

CHANNEL_PAUSE = 3.0
JITTER = 0.2

_last_sweep: datetime | None = None


def main() -> int:
    try:
        cfg = Config.from_env()
    except (RuntimeError, ValueError) as err:
        log(f"конфигурация: {err}", stream=sys.stderr)
        return 1

    log(f"старт, база {db.redacted(cfg.database_url)}")
    log(f"каналы: {', '.join(cfg.channels)}; тик {cfg.poll_minutes} мин")

    llm = llm_client(cfg)
    if llm is None:
        log("ANTHROPIC_API_KEY не задан — посты будут копиться неразобранными")

    with fetch.client() as client:
        while True:
            safe_tick(cfg, client, llm)
            time.sleep(next_delay(cfg.poll_minutes))


def safe_tick(cfg: Config, client: httpx.Client, llm) -> None:
    """Тик под общим стражем: упавший тик не роняет процесс.

    Соединение открывается на тик: между тиками десятки минут простоя, за
    которые базу успевают закрыть таймауты и NAT, а переподключение здесь
    дешевле, чем чинить протухшее соединение по ходу.
    """
    try:
        with db.connect(cfg.database_url) as conn:
            tick(conn, client, llm, cfg)
    except Exception as err:
        log(f"тик не удался, ждём следующего: {err}", stream=sys.stderr)


def tick(conn, client: httpx.Client, llm, cfg: Config) -> None:
    counts = []
    for i, channel in enumerate(cfg.channels):
        if i:
            # Не долбим t.me пачкой запросов подряд.
            time.sleep(CHANNEL_PAUSE)
        try:
            counts.append(f"{channel} +{ingest.ingest_channel(conn, client, channel)}")
        except httpx.HTTPError as err:
            counts.append(f"{channel} —")
            log(f"канал {channel} пропущен до следующего тика: {err}", stream=sys.stderr)
    log(f"ленты: {', '.join(counts)}")

    if llm is not None:
        pipeline.process_new(conn, llm, cfg)
    check_alive(conn, client, cfg)


def llm_client(cfg: Config):
    if not cfg.anthropic_api_key:
        return None
    return anthropic.Anthropic(api_key=cfg.anthropic_api_key)


def check_alive(conn, client: httpx.Client, cfg: Config) -> None:
    global _last_sweep
    now = datetime.now(timezone.utc)
    if _last_sweep is not None and now - _last_sweep < timedelta(hours=cfg.alive_hours):
        return
    closed = kill.sweep(conn, client, now)
    _last_sweep = now
    log(f"гашение: закрыто точек {closed}")


def next_delay(poll_minutes: int) -> float:
    base = poll_minutes * 60
    return base * random.uniform(1 - JITTER, 1 + JITTER)


def log(message: str, stream=None) -> None:
    print(f"parser: {message}", file=stream or sys.stdout, flush=True)


if __name__ == "__main__":
    sys.exit(main())
