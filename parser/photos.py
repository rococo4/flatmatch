"""Скачивание фото поста в общее фото-хранилище points/<uuid>.<ext> (§8 шаг 8).

Тип определяет не Content-Type (CDN t.me отдаёт его не всегда честно), а
magic bytes на первых байтах ответа. Не-картинка или сбой скачивания — фото
просто выпадает из списка, пост это не останавливает.
"""

import os
import uuid

import httpx

MAGIC = {b"\xff\xd8\xff": ".jpg", b"\x89PNG": ".png", b"RIFF": ".webp"}
TIMEOUT = 20.0


def download(client: httpx.Client, urls: list[str], photo_dir: str, limit: int = 5) -> list[str]:
    keys = []
    for url in urls[:limit]:
        try:
            response = client.get(url, timeout=TIMEOUT)
            response.raise_for_status()
        except httpx.HTTPError:
            continue

        ext = _ext(response.content)
        if ext is None:
            continue

        key = f"points/{uuid.uuid4()}{ext}"
        path = os.path.join(photo_dir, key)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as file:
            file.write(response.content)
        keys.append(key)
    return keys


def _ext(content: bytes) -> str | None:
    for magic, ext in MAGIC.items():
        if content.startswith(magic):
            return ext
    return None
