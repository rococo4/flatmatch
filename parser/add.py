"""CLI ручной досыпки квартир (§8): пост канала или свой текст → source='manual'.

    python add.py --url https://t.me/arendamsc/26240
    python add.py --text "Сдаётся двушка у Сокола, 85 000" --photo room.jpg

Правила те же, что у ленты (pipeline.verdict): руками нельзя завести точку,
которую парсер бы отверг. Отказ печатается причиной, форса нет.
"""

import argparse
import re
import sys
import uuid
from dataclasses import dataclass, field
from pathlib import Path

import anthropic
import httpx
import psycopg

import db
import extract
import fetch
import photos
import pipeline
import zones
from config import Config

SOURCE = "manual"

NO_LINK_WARNING = (
    "внимание: без ссылки в тексте точка не дедупится и не голосуется — "
    "если ссылка на оригинал есть, добавь её прямо в --text"
)

_POST_URL_RE = re.compile(r"^https?://t\.me/([A-Za-z0-9_]{3,})/(\d+)/?$")

_REASONS = {
    "wanted": "это «сниму», а не сдача",
    "roommate": "это поиск соседа, а не сдача квартиры",
    "room": "сдаётся комната или койко-место, а не квартира",
    "other": "это не объявление об аренде квартиры",
    "no_zone": "станция не распозналась — нужна одна станция московского метро из справочника",
    "no_price": "цена не найдена или вне диапазона 20 000–300 000 ₽ за квартиру",
    "bad_fields": "невозможные поля: комнаты, залог или ограничения",
}


class Refusal(Exception):
    """Причина отказа оператору: печатается и даёт код возврата 1."""


@dataclass(frozen=True)
class Draft:
    """Что досыпаем: текст плюс фото — ссылками поста или локальными файлами."""

    text: str
    url: str | None = None
    photo_urls: list[str] = field(default_factory=list)
    photo_paths: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class Added:
    point_id: uuid.UUID
    zone_id: str
    station: str | None
    photos: int
    url_norm: str | None


def main(argv=None) -> int:
    args = parse_args(argv)
    try:
        cfg = Config.from_env()
    except (RuntimeError, ValueError) as err:
        print(f"конфигурация: {err}", file=sys.stderr)
        return 1
    if not cfg.anthropic_api_key:
        print("ANTHROPIC_API_KEY не задан — разбирать текст нечем", file=sys.stderr)
        return 1

    llm = anthropic.Anthropic(api_key=cfg.anthropic_api_key)
    try:
        with fetch.client() as client:
            draft = draft_from_args(client, args)
    except Refusal as err:
        print(f"не добавлено: {err}", file=sys.stderr)
        return 1

    with db.connect(cfg.database_url) as conn:
        return run(conn, llm, cfg, draft)


def parse_args(argv=None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="add.py",
        description="Досыпка квартиры руками: точка source='manual' из поста канала или текста.",
    )
    where = parser.add_mutually_exclusive_group(required=True)
    where.add_argument("--url", help="ссылка на пост канала: https://t.me/<канал>/<id>")
    where.add_argument("--text", help="текст объявления")
    parser.add_argument(
        "--photo",
        action="append",
        metavar="ФАЙЛ",
        help=f"локальный файл фото, до {photos.LIMIT} (флаг можно повторять); "
        "с --url заменяет фото поста",
    )
    return parser.parse_args(argv)


def run(conn, llm, cfg, draft: Draft) -> int:
    try:
        added = add(conn, llm, cfg, draft)
    except Refusal as err:
        print(f"не добавлено: {err}", file=sys.stderr)
        return 1
    print(f"точка {added.point_id} · зона {added.station} ({added.zone_id}) · фото {added.photos}")
    if added.url_norm is None:
        print(NO_LINK_WARNING, file=sys.stderr)
    return 0


def add(conn, llm, cfg, draft: Draft) -> Added:
    text = draft.text.strip()
    if not text:
        raise Refusal("пустой текст")

    try:
        got, _ = extract.extract(llm, zones.station_names(cfg.zones_path), text)
    except extract.ExtractError as err:
        raise Refusal(f"разбор не удался: {err}") from err

    ready = pipeline.verdict(conn, cfg, got, draft.url, text)
    if ready.reason is not None:
        raise Refusal(_reason(ready))

    photo_keys = _photo_keys(cfg, draft)
    try:
        with conn.transaction(), conn.cursor() as cur:
            point_id = pipeline.insert_point(
                cur, got, ready.zone_id, ready.url_norm, photo_keys, SOURCE
            )
    except psycopg.errors.UniqueViolation as err:
        # Ту же ссылку успел вставить тик парсера, пока мы разбирали текст.
        raise Refusal(_dup(pipeline.live_point(conn, ready.url_norm))) from err

    return Added(
        point_id=point_id,
        zone_id=ready.zone_id,
        station=got.station,
        photos=len(photo_keys),
        url_norm=ready.url_norm,
    )


def draft_from_args(client: httpx.Client, args) -> Draft:
    paths = list(args.photo or [])
    if args.url:
        post = draft_from_url(client, args.url)
        return Draft(text=post.text, url=post.url, photo_urls=post.photo_urls, photo_paths=paths)
    return Draft(text=args.text, photo_paths=paths)


def draft_from_url(client: httpx.Client, url: str) -> Draft:
    found = _POST_URL_RE.match(url.strip())
    if found is None:
        raise Refusal(f"не ссылка на пост канала: {url}")
    channel, message_id = found.group(1), int(found.group(2))

    try:
        posts = fetch.feed(client, channel)
    except httpx.HTTPError as err:
        raise Refusal(f"канал {channel} не отдал ленту: {err}") from err

    post = next((p for p in posts if p.message_id == message_id), None)
    if post is None:
        # Превью-лента отдаёт только хвост канала: старый пост оттуда не достать.
        raise Refusal(
            f"поста {channel}/{message_id} нет в превью-ленте канала (t.me/s отдаёт "
            "последние ~10–20, зависит от канала) — досыпь текстом: "
            '--text "..." --photo файл'
        )
    return Draft(
        text=post.text,
        url=pipeline.POST_URL.format(channel=channel, message_id=message_id),
        photo_urls=list(post.photo_urls),
    )


def _reason(ready: pipeline.Verdict) -> str:
    if ready.reason == pipeline.SKIP_DUP:
        return _dup(ready.point_id)
    return _REASONS.get(ready.reason, ready.reason)


def _dup(point_id) -> str:
    return f"такая квартира уже есть: {point_id}" if point_id else "такая квартира уже есть"


def _photo_keys(cfg, draft: Draft) -> list[str]:
    if draft.photo_paths:
        return _copied(cfg, draft.photo_paths)
    if draft.photo_urls:
        with fetch.client() as client:
            return photos.download(client, draft.photo_urls, cfg.photo_dir)
    return []


def _copied(cfg, paths: list[str]) -> list[str]:
    if len(paths) > photos.LIMIT:
        raise Refusal(f"фото больше {photos.LIMIT}: {len(paths)}")

    # Читаем и проверяем всё до первой записи: половина фото в хранилище при
    # отказе — мусор, на который никто не сошлётся.
    blobs = []
    for path in paths:
        try:
            content = Path(path).read_bytes()
        except OSError as err:
            raise Refusal(f"фото {path}: {err.strerror or err}") from err
        if photos.ext(content) is None:
            raise Refusal(f"фото {path}: не jpeg, png или webp")
        blobs.append(content)
    return [photos.save(content, cfg.photo_dir) for content in blobs]


if __name__ == "__main__":
    sys.exit(main())
