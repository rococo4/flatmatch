"""Цикл: гашение раз в ALIVE_HOURS и страж тика, который держит процесс живым."""

import httpx
import psycopg
import pytest

import main
from config import Config


class _FakeConn:
    """Соединение на тик: контекст закрывает его сам."""

    def __init__(self):
        self.closed = False

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.closed = True


@pytest.fixture(autouse=True)
def reset_last_sweep():
    main._last_sweep = None
    yield
    main._last_sweep = None


@pytest.fixture
def cfg():
    return Config(
        database_url="",
        anthropic_api_key=None,
        channels=[],
        photo_dir="",
        zones_path="",
        alive_hours=4,
    )


def test_first_call_runs_the_sweep(monkeypatch, cfg):
    calls = []
    monkeypatch.setattr(main.kill, "sweep", lambda conn, client, now: calls.append(now) or 0)

    main.check_alive(None, None, cfg)

    assert len(calls) == 1
    assert main._last_sweep == calls[0]


def test_second_call_soon_after_is_skipped(monkeypatch, cfg):
    calls = []
    monkeypatch.setattr(main.kill, "sweep", lambda conn, client, now: calls.append(now) or 0)

    main.check_alive(None, None, cfg)
    main.check_alive(None, None, cfg)

    assert len(calls) == 1


def test_call_after_alive_hours_runs_again(monkeypatch, cfg):
    from datetime import timedelta

    calls = []
    monkeypatch.setattr(main.kill, "sweep", lambda conn, client, now: calls.append(now) or 0)

    main.check_alive(None, None, cfg)
    main._last_sweep -= timedelta(hours=cfg.alive_hours + 1)
    main.check_alive(None, None, cfg)

    assert len(calls) == 2


def test_broken_database_does_not_kill_the_loop(monkeypatch, cfg, capsys):
    def boom(dsn):
        raise psycopg.OperationalError("сервер ушёл")

    monkeypatch.setattr(main.db, "connect", boom)

    main.safe_tick(cfg, None, None)

    assert "тик не удался" in capsys.readouterr().err


def test_tick_gets_a_connection_opened_for_it(monkeypatch, cfg):
    seen = []
    monkeypatch.setattr(main.db, "connect", lambda dsn: _FakeConn())
    monkeypatch.setattr(main, "tick", lambda conn, client, llm, got: seen.append(conn))

    main.safe_tick(cfg, None, None)

    assert len(seen) == 1
    assert seen[0].closed is True


def test_unreachable_channel_does_not_stop_the_others(monkeypatch, capsys):
    cfg = Config(
        database_url="",
        anthropic_api_key=None,
        channels=["dead", "alive"],
        photo_dir="",
        zones_path="",
    )

    def ingest_channel(conn, client, channel):
        if channel == "dead":
            raise httpx.ConnectError("нет сети")
        return 2

    monkeypatch.setattr(main, "CHANNEL_PAUSE", 0)
    monkeypatch.setattr(main.ingest, "ingest_channel", ingest_channel)
    monkeypatch.setattr(main, "check_alive", lambda conn, client, got: None)

    main.tick(None, None, None, cfg)

    assert "ленты: dead —, alive +2" in capsys.readouterr().out


def test_delay_jitters_around_the_period():
    delays = {main.next_delay(20) for _ in range(20)}

    assert len(delays) > 1
    assert all(20 * 60 * 0.8 <= delay <= 20 * 60 * 1.2 for delay in delays)
