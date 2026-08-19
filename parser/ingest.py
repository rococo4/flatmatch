"""Инжест ленты канала в channel_posts — сырьё до всякой обработки.

Идемпотентность держит PK (channel, message_id): повторный тик по той же
ленте не создаёт ни строки. Транзакциями управляет вызывающий (db.connect
отдаёт autocommit-соединение).
"""

import hashlib

import fetch

STATUS_NEW = "new"
STATUS_SKIPPED = "skipped"
SKIP_FORWARD = "forward"

_INSERT = """
INSERT INTO channel_posts (
    channel, message_id, posted_at, text, text_hash, photo_urls, status, skip_reason)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (channel, message_id) DO NOTHING
"""


def ingest_channel(conn, client, channel: str) -> int:
    return store_posts(conn, fetch.feed(client, channel))


def store_posts(conn, posts: list[fetch.RawPost]) -> int:
    stored = 0
    with conn.cursor() as cur:
        for post in posts:
            # Форвард чужого поста — не наш контент: пишем сразу скипнутым,
            # чтобы он не попал в LLM и не задвоил объявление из канала-донора.
            status = STATUS_SKIPPED if post.is_forward else STATUS_NEW
            reason = SKIP_FORWARD if post.is_forward else None
            cur.execute(
                _INSERT,
                (
                    post.channel,
                    post.message_id,
                    post.posted_at,
                    post.text,
                    text_hash(post.text),
                    post.photo_urls,
                    status,
                    reason,
                ),
            )
            stored += cur.rowcount
    return stored


def text_hash(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()
