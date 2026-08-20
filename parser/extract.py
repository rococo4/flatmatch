"""LLM-структуризация поста канала в JSON (§3 дизайна).

Один вызов на пост: классификация плюс поля объявления. Список станций едет
в системном промпте стабильным префиксом под cache_control — он же единственный
источник имён, из которых модели разрешено выбирать.
"""

import json
from dataclasses import dataclass, field
from datetime import date

MODEL = "claude-sonnet-5"
MAX_TOKENS = 1024

KINDS = ("rent_flat", "rent_room", "wanted", "roommate", "other")
BATHROOMS = ("combined", "separate", "two")
UTILITIES = ("included", "meters")
RESTRICTIONS = ("no_pets", "no_couples", "no_smoking")

SYSTEM_PROMPT = """Ты разбираешь посты из телеграм-каналов аренды жилья в Москве
и возвращаешь строго JSON по заданной схеме. Ничего не выдумывай: чего нет в
тексте — то null (или пустой список для restrictions).

Классификация (поле kind):
- rent_flat — сдаётся квартира целиком;
- rent_room — сдаётся комната или койко-место в квартире;
- wanted — человек ищет жильё («сниму», «ищу квартиру»);
- roommate — ищут соседа в уже снятую квартиру;
- other — всё остальное: реклама, услуги, новости, посуточная аренда, продажа.

Цена (price_month_total) — рубли в месяц ЗА ВЕСЬ ОБЪЕКТ, без коммуналки и
депозита. Если цена дана на человека, умножь: «по 40к на двоих» → 80000.
Если дан диапазон, бери НИЖНЮЮ границу: «70–80к» → 70000. «45к» → 45000.

Станция (station) — РОВНО ОДНО имя из списка ниже, дословно, ближайшая к жилью.
Если в посте несколько станций — выбери ближайшую к жилью. Если станции нет,
она не из Москвы или её нет в списке — null. Ничего не додумывай по району.

Остальные поля:
- rooms — число комнат в квартире; студия и апартаменты без спален — 1;
- isolated — комнаты изолированные (true) или смежные (false);
- bathrooms — combined (совмещённый), separate (раздельный), two (два);
- utilities — included (коммуналка включена) или meters (по счётчикам);
- deposit — залог в рублях (0, если явно сказано «без залога»);
- restrictions — ограничения собственника: no_pets, no_couples, no_smoking;
- move_in — дата заезда в формате YYYY-MM-DD, если названа явно;
- listing_url — ссылка на avito.ru или cian.ru, ТОЛЬКО если она есть в тексте.

Станции:
"""

EXTRACT_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": [
        "kind",
        "station",
        "price_month_total",
        "rooms",
        "isolated",
        "bathrooms",
        "utilities",
        "deposit",
        "restrictions",
        "move_in",
        "listing_url",
    ],
    "properties": {
        "kind": {"type": "string", "enum": list(KINDS)},
        "station": {"type": ["string", "null"]},
        "price_month_total": {"type": ["integer", "null"]},
        "rooms": {"type": ["integer", "null"]},
        "isolated": {"type": ["boolean", "null"]},
        # Nullable-перечисления только через anyOf: enum рядом с union-типом
        # API отвергает четырёхсотым.
        "bathrooms": {
            "anyOf": [{"type": "string", "enum": list(BATHROOMS)}, {"type": "null"}]
        },
        "utilities": {
            "anyOf": [{"type": "string", "enum": list(UTILITIES)}, {"type": "null"}]
        },
        "deposit": {"type": ["integer", "null"]},
        "restrictions": {
            "type": "array",
            "items": {"type": "string", "enum": list(RESTRICTIONS)},
        },
        "move_in": {"type": ["string", "null"]},
        "listing_url": {"type": ["string", "null"]},
    },
}


class ExtractError(Exception):
    """Модель дважды подряд ответила не по схеме — пост уходит в error."""


@dataclass(frozen=True)
class Extract:
    kind: str
    station: str | None = None
    price_month_total: int | None = None
    rooms: int | None = None
    isolated: bool | None = None
    bathrooms: str | None = None
    utilities: str | None = None
    deposit: int | None = None
    restrictions: list[str] = field(default_factory=list)
    move_in: date | None = None
    listing_url: str | None = None


def extract(client, station_names: list[str], text: str) -> tuple[Extract, dict]:
    system = SYSTEM_PROMPT + "\n".join(station_names)
    last = None
    for _ in range(2):
        response = client.messages.create(
            model=MODEL,
            max_tokens=MAX_TOKENS,
            system=[
                {
                    "type": "text",
                    "text": system,
                    "cache_control": {"type": "ephemeral"},
                }
            ],
            messages=[{"role": "user", "content": text}],
            output_config={"format": {"type": "json_schema", "schema": EXTRACT_SCHEMA}},
        )
        try:
            raw = json.loads(_text(response))
            return parse(raw), raw
        except (ValueError, TypeError, AttributeError) as err:
            last = err
    raise ExtractError(f"ответ не по схеме: {last}")


def parse(raw: dict) -> Extract:
    if not isinstance(raw, dict):
        raise ValueError("ответ не объект")
    kind = raw.get("kind")
    if kind not in KINDS:
        raise ValueError(f"неизвестный kind: {kind!r}")
    missing = [name for name in EXTRACT_SCHEMA["required"] if name not in raw]
    if missing:
        raise ValueError(f"нет полей: {', '.join(missing)}")

    return Extract(
        kind=kind,
        station=_str(raw["station"]),
        price_month_total=_int(raw["price_month_total"]),
        rooms=_int(raw["rooms"]),
        isolated=_bool(raw["isolated"]),
        bathrooms=_one_of(raw["bathrooms"], BATHROOMS),
        utilities=_one_of(raw["utilities"], UTILITIES),
        deposit=_int(raw["deposit"]),
        restrictions=_codes(raw["restrictions"]),
        move_in=_date(raw["move_in"]),
        listing_url=_str(raw["listing_url"]),
    )


def _text(response) -> str:
    return "".join(block.text for block in response.content if block.type == "text")


def _str(value) -> str | None:
    if value is None:
        return None
    if not isinstance(value, str):
        raise ValueError(f"ожидалась строка, пришло {value!r}")
    return value.strip() or None


def _int(value) -> int | None:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, int):
        raise ValueError(f"ожидалось целое, пришло {value!r}")
    return value


def _bool(value) -> bool | None:
    if value is None:
        return None
    if not isinstance(value, bool):
        raise ValueError(f"ожидался boolean, пришло {value!r}")
    return value


def _one_of(value, allowed: tuple[str, ...]) -> str | None:
    value = _str(value)
    if value is not None and value not in allowed:
        raise ValueError(f"{value!r} не из {allowed}")
    return value


def _codes(value) -> list[str]:
    if not isinstance(value, list):
        raise ValueError(f"restrictions не список: {value!r}")
    codes = []
    for item in value:
        code = _str(item)
        if code is not None and code not in codes:
            codes.append(code)
    return codes


def _date(value) -> date | None:
    value = _str(value)
    if value is None:
        return None
    try:
        # Дата — украшение карточки: невнятную молча роняем, пост не теряем.
        return date.fromisoformat(value)
    except ValueError:
        return None
