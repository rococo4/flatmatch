"""Инжест на тестовой БД: без TEST_DATABASE_URL тесты пропускаются.

Свои строки — только каналы с префиксом test-, они же и чистятся.
"""

import os
from pathlib import Path

import httpx
import pytest

import ingest

psycopg = pytest.importorskip("psycopg")

FIXTURES = Path(__file__).parent / "fixtures"
CHANNEL = "test-ingest"
TEST_PREFIX = "test-%"


@pytest.fixture
def conn():
    dsn = os.environ.get("TEST_DATABASE_URL")
    if not dsn:
        pytest.skip("TEST_DATABASE_URL не задан")
    with psycopg.connect(dsn, autocommit=True) as connection:
        _cleanup(connection)
        yield connection
        _cleanup(connection)


@pytest.fixture
def client():
    html = (FIXTURES / "feed_arendamsc.html").read_text(encoding="utf-8")
    transport = httpx.MockTransport(lambda request: httpx.Response(200, html=html))
    with httpx.Client(transport=transport) as c:
        yield c


def test_ingest_is_idempotent(conn, client):
    assert ingest.ingest_channel(conn, client, CHANNEL) == 4
    assert ingest.ingest_channel(conn, client, CHANNEL) == 0
    assert _count(conn) == 4


def test_ingest_stores_raw_fields(conn, client):
    ingest.ingest_channel(conn, client, CHANNEL)
    row = _row(conn, 26240)

    assert row["status"] == "new"
    assert row["skip_reason"] is None
    assert row["text"].startswith("📌Доброго времени суток!\n")
    assert row["text_hash"] == ingest.text_hash(row["text"])
    assert len(row["photo_urls"]) == 2
    assert row["posted_at"].isoformat() == "2026-08-17T14:48:16+00:00"
    assert row["point_id"] is None
    assert row["possible_dup"] is False


def test_ingest_skips_forward(conn, client):
    ingest.ingest_channel(conn, client, CHANNEL)
    row = _row(conn, 26245)

    assert row["status"] == "skipped"
    assert row["skip_reason"] == "forward"


def _cleanup(connection) -> None:
    with connection.cursor() as cur:
        cur.execute("DELETE FROM channel_posts WHERE channel LIKE %s", (TEST_PREFIX,))


def _count(connection) -> int:
    with connection.cursor() as cur:
        cur.execute("SELECT count(*) FROM channel_posts WHERE channel = %s", (CHANNEL,))
        return cur.fetchone()[0]


def _row(connection, message_id: int) -> dict:
    columns = (
        "status, skip_reason, text, text_hash, photo_urls, posted_at, point_id, possible_dup"
    )
    with connection.cursor() as cur:
        cur.execute(
            f"SELECT {columns} FROM channel_posts WHERE channel = %s AND message_id = %s",
            (CHANNEL, message_id),
        )
        row = cur.fetchone()
    assert row is not None
    return dict(zip([c.strip() for c in columns.split(",")], row))
