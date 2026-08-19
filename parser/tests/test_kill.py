"""Гашение на тестовой БД: без TEST_DATABASE_URL тесты пропускаются.

Свои строки — точки с zone_id 's-test-kill' и channel_posts в канале
test-kill; post_alive мокается, живых запросов нет.
"""

import os
import random
from datetime import datetime, timezone

import pytest

import kill

psycopg = pytest.importorskip("psycopg")

CHANNEL = "test-kill"
ZONE = "s-test-kill"
NOW = datetime(2026, 8, 20, 12, 0, 0, tzinfo=timezone.utc)


@pytest.fixture
def points():
    return []


@pytest.fixture
def users():
    return []


@pytest.fixture
def conn(points, users):
    dsn = os.environ.get("TEST_DATABASE_URL")
    if not dsn:
        pytest.skip("TEST_DATABASE_URL не задан")
    with psycopg.connect(dsn, autocommit=True) as connection:
        _cleanup(connection, points, users)
        yield connection
        _cleanup(connection, points, users)


@pytest.fixture(autouse=True)
def sleep_calls(monkeypatch):
    calls = []
    monkeypatch.setattr(kill.time, "sleep", lambda seconds: calls.append(seconds))
    return calls


def fake_post_alive(results: dict, calls: list):
    def fn(client, channel, message_id):
        calls.append((channel, message_id))
        return results[(channel, message_id)]

    return fn


def refusing_post_alive(calls: list):
    def fn(client, channel, message_id):
        calls.append((channel, message_id))
        raise AssertionError("post_alive не должен был вызываться")

    return fn


@pytest.mark.parametrize("status", ["active", "paused"])
def test_deleted_post_closes_the_point(conn, points, monkeypatch, status):
    point_id = _point(conn, points, status=status, url_norm=f"https://t.me/{CHANNEL}/101")
    _channel_post(conn, 101, point_id)
    calls = []
    monkeypatch.setattr(kill, "fetch", _FakeFetch(fake_post_alive({(CHANNEL, 101): False}, calls)))

    closed = kill.sweep(conn, object(), NOW)

    assert closed == 1
    assert calls == [(CHANNEL, 101)]
    point = _point_row(conn, point_id)
    assert point["status"] == "closed"
    assert point["close_reason"] == "gone"
    assert point["edited_at"] == NOW
    post = _post_row(conn, 101)
    assert post["deleted_at"] == NOW


def test_indeterminate_leaves_the_point_untouched(conn, points, monkeypatch):
    point_id = _point(conn, points, url_norm=f"https://t.me/{CHANNEL}/102")
    _channel_post(conn, 102, point_id)
    monkeypatch.setattr(kill, "fetch", _FakeFetch(fake_post_alive({(CHANNEL, 102): None}, [])))

    closed = kill.sweep(conn, object(), NOW)

    assert closed == 0
    point = _point_row(conn, point_id)
    assert point["status"] == "active"
    assert point["close_reason"] is None
    post = _post_row(conn, 102)
    assert post["deleted_at"] is None
    assert post["last_checked_at"] == NOW


def test_alive_post_touches_last_checked_at(conn, points, monkeypatch):
    point_id = _point(conn, points, url_norm=f"https://t.me/{CHANNEL}/103")
    _channel_post(conn, 103, point_id)
    monkeypatch.setattr(kill, "fetch", _FakeFetch(fake_post_alive({(CHANNEL, 103): True}, [])))

    closed = kill.sweep(conn, object(), NOW)

    assert closed == 0
    assert _post_row(conn, 103)["last_checked_at"] == NOW


def test_user_link_with_avito_url_is_not_selected(conn, points, users, monkeypatch):
    user_id = _user(conn, users)
    point_id = _point(
        conn,
        points,
        source="user_link",
        created_by=user_id,
        url_norm="https://avito.ru/moskva/kvartiry/kvartira_1",
    )
    monkeypatch.setattr(kill, "fetch", _FakeFetch(refusing_post_alive([])))

    closed = kill.sweep(conn, object(), NOW)

    assert closed == 0
    assert _point_row(conn, point_id)["status"] == "active"


def test_manual_point_with_tme_url_is_selected(conn, points, monkeypatch):
    # У manual-точки может не быть строки channel_posts вовсе — UPDATE
    # channel_posts просто не находит строк, гашение точки это не блокирует.
    point_id = _point(
        conn, points, source="manual", url_norm=f"https://t.me/{CHANNEL}/104"
    )
    calls = []
    monkeypatch.setattr(kill, "fetch", _FakeFetch(fake_post_alive({(CHANNEL, 104): False}, calls)))

    closed = kill.sweep(conn, object(), NOW)

    assert closed == 1
    assert calls == [(CHANNEL, 104)]
    point = _point_row(conn, point_id)
    assert point["status"] == "closed"
    assert point["close_reason"] == "gone"


def test_pauses_between_requests_but_not_before_the_first(conn, points, monkeypatch, sleep_calls):
    first = _point(conn, points, url_norm=f"https://t.me/{CHANNEL}/105")
    second = _point(conn, points, url_norm=f"https://t.me/{CHANNEL}/106")
    _channel_post(conn, 105, first)
    _channel_post(conn, 106, second)
    results = {(CHANNEL, 105): True, (CHANNEL, 106): True}
    monkeypatch.setattr(kill, "fetch", _FakeFetch(fake_post_alive(results, [])))

    kill.sweep(conn, object(), NOW)

    assert len(sleep_calls) == 1
    assert kill.PAUSE_RANGE[0] <= sleep_calls[0] <= kill.PAUSE_RANGE[1]


class _FakeFetch:
    def __init__(self, post_alive):
        self.post_alive = post_alive


def _point(
    connection,
    points: list,
    *,
    status: str = "active",
    source: str = "tg_channel",
    created_by=None,
    url_norm: str,
):
    with connection.cursor() as cur:
        cur.execute(
            """
            INSERT INTO points (type, status, source, created_by, zone_id, url_norm)
            VALUES ('flat', %s, %s, %s, %s, %s)
            RETURNING id
            """,
            (status, source, created_by, ZONE, url_norm),
        )
        point_id = cur.fetchone()[0]
    points.append(point_id)
    return point_id


def _channel_post(connection, message_id: int, point_id) -> None:
    with connection.cursor() as cur:
        cur.execute(
            """
            INSERT INTO channel_posts (channel, message_id, text, text_hash, point_id, status)
            VALUES (%s, %s, 'текст', %s, %s, 'point_created')
            """,
            (CHANNEL, message_id, f"hash-{message_id}", point_id),
        )


def _user(connection, users: list):
    tg_id = random.randint(9 * 10**11, 10**12)
    with connection.cursor() as cur:
        cur.execute(
            "INSERT INTO users (tg_id, name) VALUES (%s, 'kill-test') RETURNING id",
            (tg_id,),
        )
        user_id = cur.fetchone()[0]
    users.append(user_id)
    return user_id


def _point_row(connection, point_id) -> dict:
    with connection.cursor() as cur:
        cur.execute(
            "SELECT status, close_reason, edited_at FROM points WHERE id = %s", (point_id,)
        )
        row = cur.fetchone()
    assert row is not None
    return dict(zip(("status", "close_reason", "edited_at"), row))


def _post_row(connection, message_id: int) -> dict:
    with connection.cursor() as cur:
        cur.execute(
            "SELECT deleted_at, last_checked_at FROM channel_posts "
            "WHERE channel = %s AND message_id = %s",
            (CHANNEL, message_id),
        )
        row = cur.fetchone()
    assert row is not None
    return dict(zip(("deleted_at", "last_checked_at"), row))


def _cleanup(connection, points: list, users: list) -> None:
    with connection.cursor() as cur:
        cur.execute("DELETE FROM channel_posts WHERE channel = %s", (CHANNEL,))
        if points:
            cur.execute("DELETE FROM points WHERE id = ANY(%s)", (points,))
        if users:
            cur.execute("DELETE FROM users WHERE id = ANY(%s)", (users,))
    points.clear()
    users.clear()
