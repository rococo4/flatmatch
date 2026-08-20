"""Конфигурация парсера: всё из окружения, ничего из файлов (§5 дизайна)."""

import os
from dataclasses import dataclass

DEFAULT_CHANNELS = (
    "arendamsc,ugnest_msk,norealtor,flats_4_friends,nebabushkin_msk,"
    "keystomoscow,lvngrm_msk"
)
DEFAULT_PHOTO_DIR = "../api/.data/photos"
DEFAULT_ZONES_PATH = "../api/internal/service/zones/moscow.json"
DEFAULT_POLL_MINUTES = 20
DEFAULT_ALIVE_HOURS = 4


@dataclass(frozen=True)
class Config:
    database_url: str
    anthropic_api_key: str | None
    channels: list[str]
    photo_dir: str
    zones_path: str
    poll_minutes: int = DEFAULT_POLL_MINUTES
    alive_hours: int = DEFAULT_ALIVE_HOURS

    @classmethod
    def from_env(cls) -> "Config":
        dsn = _env("DATABASE_URL")
        if not dsn:
            raise RuntimeError("DATABASE_URL не задан")
        return cls(
            database_url=dsn,
            anthropic_api_key=_env("ANTHROPIC_API_KEY") or None,
            channels=parse_channels(_env("PARSER_CHANNELS") or DEFAULT_CHANNELS),
            photo_dir=_env("PHOTO_DIR") or DEFAULT_PHOTO_DIR,
            zones_path=_env("ZONES_PATH") or DEFAULT_ZONES_PATH,
            poll_minutes=_positive_int("POLL_MINUTES", DEFAULT_POLL_MINUTES),
            alive_hours=_positive_int("ALIVE_HOURS", DEFAULT_ALIVE_HOURS),
        )


def parse_channels(raw: str) -> list[str]:
    names = []
    for part in raw.split(","):
        name = part.strip().lstrip("@")
        if name and name not in names:
            names.append(name)
    return names


def _env(name: str) -> str:
    return (os.environ.get(name) or "").strip()


def _positive_int(name: str, default: int) -> int:
    raw = _env(name)
    if not raw:
        return default
    value = int(raw)
    if value <= 0:
        raise RuntimeError(f"{name} должен быть положительным, а не {value}")
    return value
