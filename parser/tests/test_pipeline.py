"""Пайплайн на тестовой БД: без TEST_DATABASE_URL тесты пропускаются.

Свои строки — только каналы с префиксом test- и точки, созданные внутри теста;
чистится ровно это. Зоны берутся из настоящего moscow.json.
"""

import json
import os
from datetime import date
from pathlib import Path

import pytest

import pipeline
from config import Config
from conftest import FakeClient

psycopg = pytest.importorskip("psycopg")

MOSCOW = Path(__file__).parents[2] / "api" / "internal" / "service" / "zones" / "moscow.json"
CHANNEL = "test-pipeline"
TEST_PREFIX = "test-%"

FLAT = {
    "kind": "rent_flat",
    "station": "Сокол",
    "price_month_total": 85000,
    "rooms": 2,
    "isolated": True,
    "bathrooms": "separate",
    "utilities": "meters",
    "deposit": 45000,
    "restrictions": ["no_pets"],
    "move_in": "2026-09-01",
    "listing_url": None,
}


@pytest.fixture
def cfg():
    return Config(
        database_url="",
        anthropic_api_key=None,
        channels=[CHANNEL],
        photo_dir="",
        zones_path=str(MOSCOW),
    )


@pytest.fixture
def extra_points():
    return []


@pytest.fixture
def conn(extra_points):
    dsn = os.environ.get("TEST_DATABASE_URL")
    if not dsn:
        pytest.skip("TEST_DATABASE_URL не задан")
    with psycopg.connect(dsn, autocommit=True) as connection:
        _cleanup(connection, extra_points)
        yield connection
        _cleanup(connection, extra_points)


def test_flat_post_becomes_a_point(conn, cfg):
    _post(conn, 1, "Сдаётся двушка у метро Сокол, 85000 в месяц")

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 1)
    assert row["status"] == "point_created"
    assert row["skip_reason"] is None
    assert row["llm"] == FLAT
    assert row["possible_dup"] is False

    point = _point(conn, row["point_id"])
    assert point["type"] == "flat"
    assert point["status"] == "active"
    assert point["source"] == "tg_channel"
    assert point["created_by"] is None
    assert point["zone_id"] == "st-sokol"
    assert point["price_total"] == 85000
    assert point["price_share"] is None
    assert point["rooms"] == 2
    assert point["isolated"] is True
    assert point["bathrooms"] == "separate"
    assert point["utilities"] == "meters"
    assert point["deposit"] == 45000
    assert point["restrictions"] == ["no_pets"]
    assert point["move_in"] == date(2026, 9, 1)
    assert point["photo_keys"] == []
    assert point["url_norm"] == f"https://t.me/{CHANNEL}/1"


def test_listing_url_wins_over_the_post_link(conn, cfg):
    _post(conn, 2, "Квартира у Сокола, объявление на авито")
    answer = dict(FLAT, listing_url="https://www.avito.ru/moskva/kvartiry/2-k_kv%61rtira_7?x=1")

    pipeline.process_new(conn, FakeClient(json.dumps(answer)), cfg)

    point = _point(conn, _post_row(conn, 2)["point_id"])
    assert point["url_norm"] == "https://avito.ru/moskva/kvartiry/2-k_kvartira_7"


def test_repeated_processing_is_a_dup(conn, cfg):
    _post(conn, 3, "Сдаётся двушка у метро Сокол")
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)
    created = _post_row(conn, 3)["point_id"]

    _reopen(conn, 3)
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 3)
    assert row["status"] == "skipped"
    assert row["skip_reason"] == "dup"
    assert row["point_id"] == created
    assert _points_with(conn, f"https://t.me/{CHANNEL}/3") == 1


def test_existing_live_point_blocks_the_insert(conn, cfg, extra_points):
    url = f"https://t.me/{CHANNEL}/4"
    existing = _seed_point(conn, extra_points, url)
    _post(conn, 4, "Сдаётся двушка у метро Сокол")

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 4)
    assert row["status"] == "skipped"
    assert row["skip_reason"] == "dup"
    assert row["point_id"] == existing
    assert _points_with(conn, url) == 1


@pytest.mark.parametrize(
    ("kind", "reason"),
    [("wanted", "wanted"), ("roommate", "roommate"), ("rent_room", "room"), ("other", "other")],
)
def test_foreign_kinds_are_skipped(conn, cfg, kind, reason):
    _post(conn, 5, "Ищу квартиру у Сокола")
    answer = dict(FLAT, kind=kind)

    pipeline.process_new(conn, FakeClient(json.dumps(answer)), cfg)

    row = _post_row(conn, 5)
    assert row["status"] == "skipped"
    assert row["skip_reason"] == reason
    assert row["point_id"] is None
    assert row["llm"] == answer


def test_one_room_flat_still_makes_a_point(conn, cfg):
    _post(conn, 55, "Сдаётся однушка у Сокола, 65 000")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, rooms=1))), cfg)

    assert _post_row(conn, 55)["status"] == "point_created"


def test_unknown_rooms_still_makes_a_point(conn, cfg):
    _post(conn, 56, "Сдаётся квартира у Сокола, 65 000")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, rooms=None))), cfg)

    assert _post_row(conn, 56)["status"] == "point_created"


def test_station_outside_moscow_is_skipped(conn, cfg):
    _post(conn, 6, "Сдаётся квартира в Химках")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, station="Химки"))), cfg)

    assert _post_row(conn, 6)["skip_reason"] == "no_zone"


def test_no_station_is_skipped(conn, cfg):
    _post(conn, 7, "Сдаётся квартира где-то")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, station=None))), cfg)

    assert _post_row(conn, 7)["skip_reason"] == "no_zone"


@pytest.mark.parametrize("price", [5000, 400000, None])
def test_price_outside_the_range_is_skipped(conn, cfg, price):
    _post(conn, 8, "Сдаётся квартира у Сокола")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, price_month_total=price))), cfg)

    assert _post_row(conn, 8)["skip_reason"] == "no_price"


@pytest.mark.parametrize(
    "answer",
    [
        {"rooms": 0},
        {"rooms": 40},
        {"deposit": -1},
        {"restrictions": ["no_kids"]},
    ],
)
def test_impossible_fields_are_skipped(conn, cfg, answer):
    _post(conn, 9, "Сдаётся квартира у Сокола")

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, **answer))), cfg)

    assert _post_row(conn, 9)["skip_reason"] == "bad_fields"


def test_post_without_text_never_reaches_the_llm(conn, cfg):
    _post(conn, 10, "   ")
    llm = FakeClient()

    pipeline.process_new(conn, llm, cfg)

    row = _post_row(conn, 10)
    assert row["status"] == "skipped"
    assert row["skip_reason"] == "no_text"
    assert row["llm"] is None
    assert llm.calls == []


def test_broken_llm_answer_marks_error(conn, cfg):
    _post(conn, 11, "Сдаётся квартира у Сокола")

    pipeline.process_new(conn, FakeClient("не json", "тоже не json"), cfg)

    row = _post_row(conn, 11)
    assert row["status"] == "error"
    assert row["skip_reason"] == "llm_error"
    assert row["llm"] is None
    assert row["point_id"] is None


def test_twin_point_marks_possible_dup(conn, cfg):
    _post(conn, 12, "Сдаётся двушка у метро Сокол")
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)
    _post(conn, 13, "Та же двушка, другой пост")

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 13)
    assert row["status"] == "point_created"
    assert row["possible_dup"] is True
    assert _post_row(conn, 12)["possible_dup"] is False


LONG = (
    "Сдаётся просторная светлая двушка у метро Сокол, 54 метра, потолки 2,8, "
    "качественный ремонт, вся техника: стиральная машина, холодильник, "
    "посудомоечная машина, кондиционер. Адрес: Ленинградский проспект 70. "
    "Цена 85 000 плюс счётчики, залог месяц. Звонить +7-900-000-00-00"
)
REPOST = (
    "2-к квартира🏙️ м.Сокол\n📌Сдаётся просторная светлая двушка у метро Сокол, "
    "54 метра, потолки 2,8, качественный ремонт, вся техника: 🔹стиральная машина, "
    "🔹холодильник, 🔹посудомоечная машина, 🔹кондиционер. 📍Адрес: Ленинградский "
    "проспект 70. 💳Цена 85 000 плюс счётчики, залог месяц. 💌+7-900-000-00-00 @agent"
)


def test_repost_with_similar_text_is_a_dup(conn, cfg):
    _post(conn, 20, LONG)
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)
    original = _post_row(conn, 20)["point_id"]
    _post(conn, 21, REPOST)

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 21)
    assert row["status"] == "skipped"
    assert row["skip_reason"] == "dup"
    assert row["point_id"] == original
    assert _points_with(conn, f"https://t.me/{CHANNEL}/21") == 0


def test_repost_after_the_original_closed_is_a_new_point(conn, cfg):
    _post(conn, 22, LONG)
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)
    original = _post_row(conn, 22)["point_id"]
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE points SET status = 'closed', close_reason = 'gone' WHERE id = %s",
            (original,),
        )
    _post(conn, 23, REPOST)

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 23)
    assert row["status"] == "point_created"
    assert row["point_id"] != original


def test_twin_with_other_text_and_price_is_not_a_dup(conn, cfg):
    _post(conn, 24, LONG)
    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)
    _post(conn, 25, REPOST)

    pipeline.process_new(conn, FakeClient(json.dumps(dict(FLAT, price_month_total=90000))), cfg)

    assert _post_row(conn, 25)["status"] == "point_created"


def test_all_new_posts_are_taken_in_one_pass(conn, cfg):
    _post(conn, 14, "Сдаётся двушка у метро Сокол")
    _post(conn, 15, "Ищу квартиру у Сокола")
    llm = FakeClient(json.dumps(FLAT), json.dumps(dict(FLAT, kind="wanted")))

    pipeline.process_new(conn, llm, cfg)

    assert _post_row(conn, 14)["status"] == "point_created"
    assert _post_row(conn, 15)["status"] == "skipped"
    assert len(llm.calls) == 2


def test_transient_llm_failure_leaves_the_post_new(conn, cfg):
    _post(conn, 16, "Сдаётся квартира у Сокола")

    class Boom:
        class messages:
            @staticmethod
            def create(**kwargs):
                raise RuntimeError("сеть отвалилась")

    pipeline.process_new(conn, Boom(), cfg)

    assert _post_row(conn, 16)["status"] == "new"


def test_photos_are_downloaded_before_the_point_is_created(conn, cfg, monkeypatch):
    urls = ["https://cdn/a.jpg", "https://cdn/b.jpg"]
    _post(conn, 17, "Сдаётся квартира у Сокола", photo_urls=urls)
    calls = []

    def fake_download(client, got_urls, photo_dir, limit=5):
        calls.append((got_urls, photo_dir, limit))
        return ["points/fake-a.jpg", "points/fake-b.jpg"]

    monkeypatch.setattr(pipeline.photos, "download", fake_download)

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    point = _point(conn, _post_row(conn, 17)["point_id"])
    assert point["photo_keys"] == ["points/fake-a.jpg", "points/fake-b.jpg"]
    assert calls == [(urls, cfg.photo_dir, 5)]


def test_broken_photo_storage_does_not_send_the_post_back_to_new(conn, cfg, monkeypatch):
    _post(conn, 18, "Сдаётся квартира у Сокола", photo_urls=["https://cdn/a.jpg"])

    def boom(client, urls, photo_dir, limit=5):
        raise PermissionError(13, "Permission denied", photo_dir)

    monkeypatch.setattr(pipeline.photos, "download", boom)

    pipeline.process_new(conn, FakeClient(json.dumps(FLAT)), cfg)

    row = _post_row(conn, 18)
    assert row["status"] == "point_created"
    assert _point(conn, row["point_id"])["photo_keys"] == []


def _post(connection, message_id: int, text: str, photo_urls: list | None = None) -> None:
    with connection.cursor() as cur:
        cur.execute(
            """
            INSERT INTO channel_posts (channel, message_id, text, text_hash, photo_urls)
            VALUES (%s, %s, %s, %s, %s)
            """,
            (CHANNEL, message_id, text, f"hash-{message_id}", photo_urls or []),
        )


def _reopen(connection, message_id: int) -> None:
    with connection.cursor() as cur:
        cur.execute(
            """
            UPDATE channel_posts SET status = 'new', skip_reason = NULL, point_id = NULL
            WHERE channel = %s AND message_id = %s
            """,
            (CHANNEL, message_id),
        )


def _post_row(connection, message_id: int) -> dict:
    columns = "status, skip_reason, llm, point_id, possible_dup"
    with connection.cursor() as cur:
        cur.execute(
            f"SELECT {columns} FROM channel_posts WHERE channel = %s AND message_id = %s",
            (CHANNEL, message_id),
        )
        row = cur.fetchone()
    assert row is not None
    return dict(zip([c.strip() for c in columns.split(",")], row))


def _point(connection, point_id) -> dict:
    columns = (
        "type, status, source, created_by, zone_id, price_total, price_share, rooms, "
        "isolated, bathrooms, utilities, deposit, restrictions, move_in, photo_keys, url_norm"
    )
    assert point_id is not None
    with connection.cursor() as cur:
        cur.execute(f"SELECT {columns} FROM points WHERE id = %s", (point_id,))
        row = cur.fetchone()
    assert row is not None
    return dict(zip([c.strip() for c in columns.split(",")], row))


def _points_with(connection, url_norm: str) -> int:
    with connection.cursor() as cur:
        cur.execute("SELECT count(*) FROM points WHERE url_norm = %s", (url_norm,))
        return cur.fetchone()[0]


def _seed_point(connection, extra_points: list, url_norm: str):
    with connection.cursor() as cur:
        cur.execute(
            """
            INSERT INTO points (type, status, source, zone_id, price_total, url_norm)
            VALUES ('flat', 'active', 'tg_channel', 'st-sokol', 90000, %s)
            RETURNING id
            """,
            (url_norm,),
        )
        point_id = cur.fetchone()[0]
    extra_points.append(point_id)
    return point_id


def _cleanup(connection, extra_points: list) -> None:
    with connection.cursor() as cur:
        cur.execute(
            "SELECT point_id FROM channel_posts WHERE channel LIKE %s AND point_id IS NOT NULL",
            (TEST_PREFIX,),
        )
        ids = [row[0] for row in cur.fetchall()] + list(extra_points)
        cur.execute("DELETE FROM channel_posts WHERE channel LIKE %s", (TEST_PREFIX,))
        if ids:
            cur.execute("DELETE FROM points WHERE id = ANY(%s)", (ids,))
    extra_points.clear()
