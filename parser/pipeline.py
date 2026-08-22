"""Пост канала → точка (§3 дизайна, шаги 3–9).

Каждый пост проходит путь целиком и заканчивается ровно одной записью в
channel_posts: точкой, скипом с причиной или ошибкой. Соседние посты падение
одного не задевает.
"""

import sys
from dataclasses import dataclass

import psycopg
from psycopg.types.json import Jsonb

import dedup
import extract
import fetch
import photos
import zones
from norm import normalize_listing_url

SOURCE = "tg_channel"
POST_URL = "https://t.me/{channel}/{message_id}"

MIN_PRICE = 20_000
MAX_PRICE = 300_000
ROOMS_CAP = 20
DEPOSIT_CAP = 1_000_000
RESTRICTIONS = ("no_pets", "no_couples", "no_smoking")
TWIN_DAYS = 7

STATUS_SKIPPED = "skipped"
STATUS_POINT_CREATED = "point_created"
STATUS_ERROR = "error"

SKIP_BY_KIND = {
    "rent_room": "room",
    "wanted": "wanted",
    "roommate": "roommate",
    "other": "other",
}
SKIP_NO_TEXT = "no_text"
SKIP_NO_ZONE = "no_zone"
SKIP_NO_PRICE = "no_price"
SKIP_BAD_FIELDS = "bad_fields"
SKIP_DUP = "dup"
SKIP_LLM_ERROR = "llm_error"

_SELECT_NEW = """
SELECT channel, message_id, text, photo_urls
FROM channel_posts
WHERE status = 'new'
ORDER BY first_seen_at
"""

_FINISH = """
UPDATE channel_posts
SET status = %s, skip_reason = %s, llm = %s, point_id = %s, possible_dup = %s
WHERE channel = %s AND message_id = %s
"""

_LIVE_BY_URL = "SELECT id FROM points WHERE url_norm = %s AND status <> 'closed' LIMIT 1"

_SIMILAR = f"""
SELECT p.id, cp.text
FROM points p
JOIN channel_posts cp ON cp.point_id = p.id
WHERE p.status <> 'closed'
  AND p.zone_id = %s
  AND p.price_total = %s
  AND p.created_at >= now() - interval '{TWIN_DAYS} days'
ORDER BY p.created_at
"""

_TWIN = f"""
SELECT EXISTS (
    SELECT 1 FROM points
    WHERE source = '{SOURCE}' AND status <> 'closed'
      AND zone_id = %s
      AND price_total IS NOT DISTINCT FROM %s
      AND rooms IS NOT DISTINCT FROM %s
      AND created_at >= now() - interval '{TWIN_DAYS} days')
"""

_INSERT_POINT = """
INSERT INTO points (
    type, status, source, created_by, zone_id,
    price_total, rooms, isolated, bathrooms, utilities,
    deposit, restrictions, move_in, photo_keys, url_norm)
VALUES ('flat', 'active', %s, NULL, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
RETURNING id
"""


@dataclass(frozen=True)
class Verdict:
    """Приговор разобранному посту: причина отказа либо поля будущей точки."""

    reason: str | None = None
    point_id: str | None = None
    zone_id: str | None = None
    url_norm: str | None = None


def process_new(conn, llm, cfg) -> None:
    with conn.cursor() as cur:
        cur.execute(_SELECT_NEW)
        posts = [
            dict(zip(("channel", "message_id", "text", "photo_urls"), row))
            for row in cur.fetchall()
        ]

    with fetch.client() as photo_client:
        for post in posts:
            try:
                status = process_post(conn, llm, cfg, post, photo_client)
            except Exception as err:
                # Сеть, лимиты, недоступный ключ: пост остаётся new и уедет в
                # следующий тик, соседние посты падение одного не задевает.
                log(f"пост {_where(post)} отложен: {err}", stream=sys.stderr)
                continue
            log(f"пост {_where(post)}: {status}")


def process_post(conn, llm, cfg, post, photo_client) -> str:
    if not post["text"].strip():
        # Видео и медиа без подписи — почти половина ленты, LLM на них не тратим.
        return _finish(conn, post, STATUS_SKIPPED, SKIP_NO_TEXT, None)

    try:
        got, raw = extract.extract(llm, zones.station_names(cfg.zones_path), post["text"])
    except extract.ExtractError as err:
        log(f"пост {_where(post)}: {err}", stream=sys.stderr)
        return _finish(conn, post, STATUS_ERROR, SKIP_LLM_ERROR, None)

    ready = verdict(
        conn,
        cfg,
        got,
        POST_URL.format(channel=post["channel"], message_id=post["message_id"]),
        post["text"],
    )
    if ready.reason is not None:
        return _finish(conn, post, STATUS_SKIPPED, ready.reason, raw, point_id=ready.point_id)

    photo_keys = _photos(photo_client, post, cfg)

    try:
        return _create(conn, post, got, raw, ready, photo_keys)
    except psycopg.errors.UniqueViolation:
        # Гонку с параллельной подачей той же ссылки закрывает уникальный индекс.
        live = live_point(conn, ready.url_norm)
        return _finish(conn, post, STATUS_SKIPPED, SKIP_DUP, raw, point_id=live)


def verdict(conn, cfg, got, fallback_url: str | None, text: str | None = None) -> Verdict:
    """Правила §3 дизайна, шаги 4–6: годится ли разбор в точку.

    Общие для ленты и досыпки (add.py) — расхождение здесь означало бы, что
    руками можно завести точку, которую парсер бы не завёл. Текст нужен для
    ловли репостов: та же станция и цена за неделю плюс похожие слова.
    """
    if got.kind in SKIP_BY_KIND:
        return Verdict(reason=SKIP_BY_KIND[got.kind])

    zone_id = _zone(cfg, got.station)
    if zone_id is None:
        return Verdict(reason=SKIP_NO_ZONE)
    if got.price_month_total is None or not MIN_PRICE <= got.price_month_total <= MAX_PRICE:
        return Verdict(reason=SKIP_NO_PRICE)
    if not _fields_ok(got):
        return Verdict(reason=SKIP_BAD_FIELDS)

    url_norm = normalize_listing_url(got.listing_url or "") or fallback_url
    if url_norm is not None:
        live = live_point(conn, url_norm)
        if live is not None:
            return Verdict(reason=SKIP_DUP, point_id=live, url_norm=url_norm)
    if text:
        repost = _reposted(conn, zone_id, got.price_month_total, text)
        if repost is not None:
            return Verdict(reason=SKIP_DUP, point_id=repost, url_norm=url_norm)
    return Verdict(zone_id=zone_id, url_norm=url_norm)


def insert_point(cur, got, zone_id: str, url_norm: str | None, photo_keys: list[str], source: str):
    cur.execute(
        _INSERT_POINT,
        (
            source,
            zone_id,
            got.price_month_total,
            got.rooms,
            got.isolated,
            got.bathrooms,
            got.utilities,
            got.deposit,
            got.restrictions,
            got.move_in,
            photo_keys,
            url_norm,
        ),
    )
    return cur.fetchone()[0]


def live_point(conn, url_norm: str | None):
    with conn.cursor() as cur:
        cur.execute(_LIVE_BY_URL, (url_norm,))
        row = cur.fetchone()
    return row[0] if row else None


def log(message: str, stream=None) -> None:
    print(f"parser: {message}", file=stream or sys.stdout, flush=True)


def _where(post) -> str:
    return f"{post['channel']}/{post['message_id']}"


def _photos(photo_client, post, cfg) -> list[str]:
    try:
        return photos.download(photo_client, post["photo_urls"], cfg.photo_dir)
    except OSError as err:
        # Диск, права на том, битый путь: фото теряем, но пост доводим до конца.
        # За LLM по нему уже заплачено, а брошенный в new он купит новый вызов
        # на каждом следующем тике — и так вечно.
        log(f"пост {_where(post)}: фото не сохранились: {err}", stream=sys.stderr)
        return []


def _create(conn, post, got, raw, ready: Verdict, photo_keys: list[str]) -> str:
    twin = _twin(conn, ready.zone_id, got.price_month_total, got.rooms)
    with conn.transaction(), conn.cursor() as cur:
        point_id = insert_point(cur, got, ready.zone_id, ready.url_norm, photo_keys, SOURCE)
        cur.execute(
            _FINISH,
            (
                STATUS_POINT_CREATED,
                None,
                Jsonb(raw),
                point_id,
                twin,
                post["channel"],
                post["message_id"],
            ),
        )
    return STATUS_POINT_CREATED


def _finish(conn, post, status: str, reason: str | None, raw, point_id=None) -> str:
    with conn.cursor() as cur:
        cur.execute(
            _FINISH,
            (
                status,
                reason,
                Jsonb(raw) if raw is not None else None,
                point_id,
                False,
                post["channel"],
                post["message_id"],
            ),
        )
    return status


def _zone(cfg, station: str | None) -> str | None:
    if not station:
        return None
    return zones.stations(cfg.zones_path).get(zones.normalized(station))


def _fields_ok(got) -> bool:
    if got.rooms is not None and not 1 <= got.rooms <= ROOMS_CAP:
        return False
    if got.deposit is not None and not 0 <= got.deposit <= DEPOSIT_CAP:
        return False
    return all(code in RESTRICTIONS for code in got.restrictions)


def _reposted(conn, zone_id: str, price: int, text: str):
    words = dedup.tokens(text)
    with conn.cursor() as cur:
        cur.execute(_SIMILAR, (zone_id, price))
        for point_id, other in cur.fetchall():
            if dedup.same_listing(words, dedup.tokens(other)):
                return point_id
    return None


def _twin(conn, zone_id: str, price: int, rooms: int | None) -> bool:
    with conn.cursor() as cur:
        cur.execute(_TWIN, (zone_id, price, rooms))
        return cur.fetchone()[0]
