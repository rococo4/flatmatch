"""Станции из справочника зон api (§8): имя из поста → zone_id.

Справочник один на продукт и версионируется генератором зон, парсер только
читает его по ZONES_PATH. Наружу торчат словарь для мапинга ответа LLM и
список имён для системного промпта.
"""

import json
from functools import lru_cache

KIND_STATION = "station"


def stations(path) -> dict[str, str]:
    return _load(str(path))[0]


def station_names(path) -> list[str]:
    return _load(str(path))[1]


def normalized(name: str) -> str:
    return name.strip().lower().replace("ё", "е")


@lru_cache(maxsize=4)
def _load(path: str) -> tuple[dict[str, str], list[str]]:
    with open(path, encoding="utf-8") as file:
        raw = json.load(file)
    table, names = {}, []
    for zone in raw.get("zones", ()):
        if zone.get("kind") != KIND_STATION:
            continue
        table[normalized(zone["name"])] = zone["id"]
        names.append(zone["name"])
    return table, names
