"""Репосты одного объявления по разным каналам (README, шаг 4).

Каналы переливают одни и те же объявления, добавляя свои эмодзи, подписи и
хэштеги, а иногда режут текст — точное совпадение ловит единицы. Сравниваем
множества слов долей общих от меньшего текста, чтобы обрезанный репост тоже
считался тем же объявлением. Шаблонные каналы пишут все посты одними словами,
поэтому решают числа: площадь, этаж, дом, телефон у разных квартир разные.
"""

import re

MIN_TOKENS = 12
MIN_NUMBERS = 3
WORDS_THRESHOLD = 0.6
WORDS_ALONE_THRESHOLD = 0.9
NUMBERS_THRESHOLD = 0.8

_NOISE = re.compile(r"https?://\S+|\bt\.me/\S+|[@#]\w+", re.IGNORECASE)
_TOKEN = re.compile(r"[^\W_]+")
_DIGIT = re.compile(r"\d")


def tokens(text: str) -> frozenset[str]:
    cleaned = _NOISE.sub(" ", text.lower())
    return frozenset(t for t in _TOKEN.findall(cleaned) if len(t) >= 2)


def same_listing(a: frozenset[str], b: frozenset[str]) -> bool:
    if min(len(a), len(b)) < MIN_TOKENS:
        return False
    words = _containment(a, b)
    if words < WORDS_THRESHOLD:
        return False
    numbers_a, numbers_b = _numbers(a), _numbers(b)
    if min(len(numbers_a), len(numbers_b)) < MIN_NUMBERS:
        return words >= WORDS_ALONE_THRESHOLD
    return _containment(numbers_a, numbers_b) >= NUMBERS_THRESHOLD


def _containment(a: frozenset[str], b: frozenset[str]) -> float:
    return len(a & b) / min(len(a), len(b))


def _numbers(words: frozenset[str]) -> frozenset[str]:
    return frozenset(w for w in words if _DIGIT.search(w))
