"""Гашение точек, чей ТГ-пост исчез из канала (§8, §4 дизайна парсера).

Раз в ALIVE_HOURS обходим живые tg_channel/manual-точки с t.me-ссылкой и
проверяем пост через embed (fetch.post_alive). Однозначное «поста нет» гасит
точку тихо, без пуша — как TTL-архив (§8). Неопределённый ответ (сеть,
не-200) точку не трогает вовсе: только отметка last_checked_at на сырьё,
если оно есть — у manual-точек channel_posts может не быть строки.
"""

import random
import re
import time

import fetch

PAUSE_RANGE = (2.0, 3.0)

_URL_RE = re.compile(r"^https://t\.me/([^/]+)/(\d+)$")

_SELECT = """
SELECT id, url_norm FROM points
WHERE status IN ('active', 'paused')
  AND source IN ('tg_channel', 'manual')
  AND url_norm LIKE 'https://t.me/%'
"""

_CLOSE = """
UPDATE points SET status = 'closed', close_reason = 'gone', edited_at = %s
WHERE id = %s AND status IN ('active', 'paused')
"""

_MARK_DELETED = "UPDATE channel_posts SET deleted_at = %s WHERE point_id = %s"

_TOUCH_CHECKED = "UPDATE channel_posts SET last_checked_at = %s WHERE point_id = %s"


def sweep(conn, client, now) -> int:
    with conn.cursor() as cur:
        cur.execute(_SELECT)
        rows = cur.fetchall()

    closed = 0
    for i, (point_id, url_norm) in enumerate(rows):
        if i:
            time.sleep(random.uniform(*PAUSE_RANGE))
        match = _URL_RE.match(url_norm)
        if match is None:
            continue
        channel, message_id = match.group(1), int(match.group(2))
        alive = fetch.post_alive(client, channel, message_id)
        if alive is False:
            closed += _close(conn, point_id, now)
        else:
            _touch(conn, point_id, now)
    return closed


def _close(conn, point_id, now) -> int:
    # Точку могли закрыть голосами или TTL между SELECT и этим UPDATE: чужую
    # причину закрытия не перебиваем, а пост всё равно помечаем удалённым.
    with conn.transaction(), conn.cursor() as cur:
        cur.execute(_CLOSE, (now, point_id))
        closed = cur.rowcount
        cur.execute(_MARK_DELETED, (now, point_id))
    return closed


def _touch(conn, point_id, now) -> None:
    with conn.cursor() as cur:
        cur.execute(_TOUCH_CHECKED, (now, point_id))
