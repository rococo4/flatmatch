"""Скачивание фото поста в общее фото-хранилище points/<uuid>.<ext> (§8 шаг 8).

Тип определяет не Content-Type (CDN t.me отдаёт его не всегда честно), а
magic bytes на первых байтах ответа. Не-картинка или сбой скачивания — фото
просто выпадает из списка, пост это не останавливает.
"""

import os
import uuid

import httpx

LIMIT = 5
MAGIC = {b"\xff\xd8\xff": ".jpg", b"\x89PNG": ".png", b"RIFF": ".webp"}
TIMEOUT = 20.0


def download(
    client: httpx.Client, urls: list[str], photo_dir: str, limit: int = LIMIT
) -> list[str]:
    keys = []
    for url in urls[:limit]:
        try:
            response = client.get(url, timeout=TIMEOUT)
            response.raise_for_status()
        except httpx.HTTPError:
            continue

        key = save(response.content, photo_dir)
        if key is not None:
            keys.append(key)
    return keys


def save(content: bytes, photo_dir: str) -> str | None:
    """Байты картинки под новым ключом; None — если это не jpeg/png/webp."""
    got = ext(content)
    if got is None:
        return None

    key = f"points/{uuid.uuid4()}{got}"
    path = os.path.join(photo_dir, key)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as file:
        file.write(content)
    return key


def ext(content: bytes) -> str | None:
    """Расширение по magic bytes; None — байты не картинка."""
    for magic, suffix in MAGIC.items():
        if content.startswith(magic):
            return suffix
    return None
