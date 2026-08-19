"""HTTP-транспорт: лента t.me/s/<канал> и проверка жизни поста через embed.

Узкий шов под возможный переезд на MTProto: наружу торчат только RawPost,
feed() и post_alive().
"""

import re
from dataclasses import dataclass, field
from datetime import datetime

import httpx
from bs4 import BeautifulSoup, NavigableString

FEED_URL = "https://t.me/s/{channel}"
EMBED_URL = "https://t.me/{channel}/{message_id}?embed=1"
USER_AGENT = "Mozilla/5.0 (compatible; FlatMatchBot/0.1; +https://t.me/flatmatch_bot)"
TIMEOUT = 20.0
DELETED_MARKERS = ("tgme_widget_message_error", "Post not found")

_MESSAGE_SEL = ".tgme_widget_message"
_TEXT_SEL = ".tgme_widget_message_text"
_PHOTO_SEL = ".tgme_widget_message_photo_wrap"
_FORWARD_SEL = ".tgme_widget_message_forwarded_from"
_DATE_SEL = ".tgme_widget_message_date time"
_ID_RE = re.compile(r"/(\d+)$")
_PHOTO_RE = re.compile(r"background-image:\s*url\('([^']+)'\)")


@dataclass(frozen=True)
class RawPost:
    channel: str
    message_id: int
    posted_at: datetime | None
    text: str
    photo_urls: list[str] = field(default_factory=list)
    is_forward: bool = False


def client() -> httpx.Client:
    return httpx.Client(
        headers={"User-Agent": USER_AGENT, "Accept-Language": "ru,en;q=0.8"},
        timeout=TIMEOUT,
        follow_redirects=True,
    )


def feed(client: httpx.Client, channel: str) -> list[RawPost]:
    response = client.get(FEED_URL.format(channel=channel))
    response.raise_for_status()
    return parse_feed(response.text, channel)


def post_alive(client: httpx.Client, channel: str, message_id: int) -> bool | None:
    try:
        response = client.get(EMBED_URL.format(channel=channel, message_id=message_id))
    except httpx.HTTPError:
        return None
    if response.status_code != 200:
        return None
    return parse_alive(response.text)


def parse_feed(html: str, channel: str) -> list[RawPost]:
    soup = BeautifulSoup(html, "html.parser")
    posts = []
    for message in soup.select(_MESSAGE_SEL):
        message_id = _message_id(message.get("data-post"))
        if message_id is None:
            continue
        posts.append(
            RawPost(
                channel=channel,
                message_id=message_id,
                posted_at=_posted_at(message),
                text=_text(message),
                photo_urls=_photo_urls(message),
                is_forward=message.select_one(_FORWARD_SEL) is not None,
            )
        )
    return posts


def parse_alive(html: str) -> bool | None:
    # Однозначно мёртвым считаем только явный маркер ошибки: пустая или
    # неузнанная страница — это «не смогли определить», а не «поста нет».
    if any(marker in html for marker in DELETED_MARKERS):
        return False
    soup = BeautifulSoup(html, "html.parser")
    if soup.select_one(_MESSAGE_SEL) is None:
        return None
    return True


def _message_id(data_post: str | None) -> int | None:
    if not data_post:
        return None
    found = _ID_RE.search(data_post)
    return int(found.group(1)) if found else None


def _posted_at(message) -> datetime | None:
    tag = message.select_one(_DATE_SEL)
    raw = tag.get("datetime") if tag else None
    if not raw:
        return None
    try:
        return datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError:
        return None


def _text(message) -> str:
    tag = message.select_one(_TEXT_SEL)
    if tag is None:
        return ""
    # <br> — единственный носитель переносов строки в разметке t.me.
    for br in tag.find_all("br"):
        br.replace_with(NavigableString("\n"))
    return tag.get_text().strip()


def _photo_urls(message) -> list[str]:
    urls = []
    for wrap in message.select(_PHOTO_SEL):
        found = _PHOTO_RE.search(wrap.get("style") or "")
        if found and found.group(1) not in urls:
            urls.append(found.group(1))
    return urls
