"""Досыпка: тот же пайплайн руками. Без TEST_DATABASE_URL часть тестов пропускается.

Свои строки ищутся снимком points до и после теста — id досыпанной точки
заранее неизвестен, а чужие строки в тестовой базе трогать нельзя.
"""

import json
import os
from pathlib import Path

import httpx
import pytest

import add
from config import Config
from conftest import FakeClient

psycopg = pytest.importorskip("psycopg")

FIXTURES = Path(__file__).parent / "fixtures"
MOSCOW = Path(__file__).parents[2] / "api" / "internal" / "service" / "zones" / "moscow.json"
JPEG = b"\xff\xd8\xff" + b"\x00" * 20

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
def cfg(tmp_path):
    return Config(
        database_url="",
        anthropic_api_key=None,
        channels=[],
        photo_dir=str(tmp_path / "photos"),
        zones_path=str(MOSCOW),
    )


@pytest.fixture
def conn():
    dsn = os.environ.get("TEST_DATABASE_URL")
    if not dsn:
        pytest.skip("TEST_DATABASE_URL не задан")
    with psycopg.connect(dsn, autocommit=True) as connection:
        yield connection


@pytest.fixture
def added(conn):
    before = _ids(conn)
    yield lambda: sorted(_ids(conn) - before)
    extra = list(_ids(conn) - before)
    if extra:
        with conn.cursor() as cur:
            cur.execute("DELETE FROM points WHERE id = ANY(%s)", (extra,))


def test_text_becomes_a_manual_point(conn, cfg, added, capsys):
    draft = add.Draft(text="Сдаётся двушка у метро Сокол, 85000 в месяц")

    code = add.run(conn, FakeClient(json.dumps(FLAT)), cfg, draft)

    out = capsys.readouterr().out
    assert code == 0
    point = _only_point(conn, added())
    assert str(point["id"]) in out
    assert "Сокол" in out
    assert point["type"] == "flat"
    assert point["status"] == "active"
    assert point["source"] == "manual"
    assert point["created_by"] is None
    assert point["zone_id"] == "st-sokol"
    assert point["price_total"] == 85000
    assert point["rooms"] == 2
    assert point["url_norm"] is None
    assert point["photo_keys"] == []


def test_listing_url_from_the_text_is_stored(conn, cfg, added):
    answer = dict(FLAT, listing_url="https://www.avito.ru/moskva/kvartiry/2-k_kvartira_77?x=1")

    add.add(conn, FakeClient(json.dumps(answer)), cfg, add.Draft(text="Сокол, авито"))

    point = _only_point(conn, added())
    assert point["url_norm"] == "https://avito.ru/moskva/kvartiry/2-k_kvartira_77"


def test_price_out_of_range_prints_the_reason(conn, cfg, added, capsys):
    answer = dict(FLAT, price_month_total=5000)

    code = add.run(conn, FakeClient(json.dumps(answer)), cfg, add.Draft(text="Сокол за копейки"))

    assert code == 1
    assert "цена" in capsys.readouterr().err
    assert added() == []


def test_foreign_kind_prints_the_reason(conn, cfg, added, capsys):
    answer = dict(FLAT, kind="wanted")

    code = add.run(conn, FakeClient(json.dumps(answer)), cfg, add.Draft(text="Ищу квартиру"))

    assert code == 1
    assert "не добавлено" in capsys.readouterr().err
    assert added() == []


def test_unknown_station_prints_the_reason(conn, cfg, added, capsys):
    answer = dict(FLAT, station="Химки")

    code = add.run(conn, FakeClient(json.dumps(answer)), cfg, add.Draft(text="Квартира в Химках"))

    assert code == 1
    assert "станция" in capsys.readouterr().err
    assert added() == []


def test_live_point_with_the_same_link_is_refused(conn, cfg, added, capsys):
    url = "https://avito.ru/moskva/kvartiry/manual_dup_test"
    existing = _seed_point(conn, url)
    answer = dict(FLAT, listing_url=url)

    code = add.run(conn, FakeClient(json.dumps(answer)), cfg, add.Draft(text="Сокол, авито"))

    assert code == 1
    assert str(existing) in capsys.readouterr().err
    assert added() == [existing]


def test_local_photos_land_in_the_store(conn, cfg, added, tmp_path):
    photo = tmp_path / "room.jpg"
    photo.write_bytes(JPEG)

    add.add(
        conn,
        FakeClient(json.dumps(FLAT)),
        cfg,
        add.Draft(text="Сдаётся двушка у Сокола", photo_paths=[str(photo)]),
    )

    point = _only_point(conn, added())
    assert len(point["photo_keys"]) == 1
    key = point["photo_keys"][0]
    assert key.startswith("points/") and key.endswith(".jpg")
    assert (Path(cfg.photo_dir) / key).read_bytes() == JPEG


def test_photo_that_is_not_an_image_refuses_the_whole_add(conn, cfg, added, tmp_path, capsys):
    junk = tmp_path / "scan.pdf"
    junk.write_bytes(b"%PDF-1.7 not a photo")

    code = add.run(
        conn,
        FakeClient(json.dumps(FLAT)),
        cfg,
        add.Draft(text="Сдаётся двушка у Сокола", photo_paths=[str(junk)]),
    )

    assert code == 1
    assert "scan.pdf" in capsys.readouterr().err
    assert added() == []


def test_missing_photo_file_refuses_the_whole_add(conn, cfg, added, tmp_path, capsys):
    code = add.run(
        conn,
        FakeClient(json.dumps(FLAT)),
        cfg,
        add.Draft(text="Сдаётся двушка у Сокола", photo_paths=[str(tmp_path / "нет.jpg")]),
    )

    assert code == 1
    assert "нет.jpg" in capsys.readouterr().err
    assert added() == []


def test_empty_text_never_reaches_the_llm(conn, cfg, added):
    llm = FakeClient()

    with pytest.raises(add.Refusal):
        add.add(conn, llm, cfg, add.Draft(text="   "))

    assert llm.calls == []
    assert added() == []


def test_draft_from_a_post_url_takes_text_and_photos():
    with _client(_feed()) as client:
        draft = add.draft_from_url(client, "https://t.me/arendamsc/26240")

    assert draft.url == "https://t.me/arendamsc/26240"
    assert draft.text.startswith("📌Доброго времени суток!")
    assert len(draft.photo_urls) == 2


def test_post_outside_the_feed_window_suggests_text():
    with _client(_feed()) as client:
        with pytest.raises(add.Refusal) as err:
            add.draft_from_url(client, "https://t.me/arendamsc/1")

    assert "--text" in str(err.value)


@pytest.mark.parametrize(
    "url",
    ["https://avito.ru/moskva/kvartiry/1", "https://t.me/arendamsc", "просто текст"],
)
def test_url_that_is_not_a_post_link_is_refused(url):
    with _client(_feed()) as client:
        with pytest.raises(add.Refusal):
            add.draft_from_url(client, url)


def test_args_require_url_or_text():
    with pytest.raises(SystemExit):
        add.parse_args([])


def _feed() -> str:
    return (FIXTURES / "feed_arendamsc.html").read_text(encoding="utf-8")


def _client(html: str) -> httpx.Client:
    def handler(request):
        return httpx.Response(200, html=html)

    return httpx.Client(transport=httpx.MockTransport(handler))


def _ids(connection) -> set:
    with connection.cursor() as cur:
        cur.execute("SELECT id FROM points")
        return {row[0] for row in cur.fetchall()}


def _only_point(connection, ids: list) -> dict:
    assert len(ids) == 1
    columns = (
        "id, type, status, source, created_by, zone_id, price_total, rooms, photo_keys, url_norm"
    )
    with connection.cursor() as cur:
        cur.execute(f"SELECT {columns} FROM points WHERE id = %s", (ids[0],))
        row = cur.fetchone()
    assert row is not None
    return dict(zip([c.strip() for c in columns.split(",")], row))


def _seed_point(connection, url_norm: str):
    with connection.cursor() as cur:
        cur.execute(
            """
            INSERT INTO points (type, status, source, zone_id, price_total, url_norm)
            VALUES ('flat', 'active', 'manual', 'st-sokol', 90000, %s)
            RETURNING id
            """,
            (url_norm,),
        )
        return cur.fetchone()[0]
