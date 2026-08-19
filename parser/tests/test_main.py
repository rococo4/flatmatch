"""check_alive: гашение раз в ALIVE_HOURS, первый тик после старта — всегда."""

import pytest

import main
from config import Config


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
