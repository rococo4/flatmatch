"""LLM-экстракция на моке клиента: настоящий API в тестах не дёргаем.

Правила промпта (цена total, нижняя граница диапазона, одна станция) юнитом
не проверить — фиксируем контракт вызова и разбора ответа.
"""

import json
from datetime import date

import pytest

import extract
from conftest import FakeBlock, FakeClient, FakeResponse
from extract import ExtractError

STATIONS = ["Сокол", "Савёловская", "Хамовники"]

FULL = {
    "kind": "rent_flat",
    "station": "Сокол",
    "price_month_total": 85000,
    "rooms": 2,
    "isolated": True,
    "bathrooms": "separate",
    "utilities": "meters",
    "deposit": 45000,
    "restrictions": ["no_pets", "no_smoking"],
    "move_in": "2026-09-01",
    "listing_url": "https://avito.ru/moskva/kvartiry/1",
}

EMPTY = {
    "kind": "other",
    "station": None,
    "price_month_total": None,
    "rooms": None,
    "isolated": None,
    "bathrooms": None,
    "utilities": None,
    "deposit": None,
    "restrictions": [],
    "move_in": None,
    "listing_url": None,
}


def test_full_answer_becomes_a_dataclass():
    client = FakeClient(json.dumps(FULL))

    got, raw = extract.extract(client, STATIONS, "текст поста")

    assert raw == FULL
    assert got.kind == "rent_flat"
    assert got.station == "Сокол"
    assert got.price_month_total == 85000
    assert got.rooms == 2
    assert got.isolated is True
    assert got.bathrooms == "separate"
    assert got.utilities == "meters"
    assert got.deposit == 45000
    assert got.restrictions == ["no_pets", "no_smoking"]
    assert got.move_in == date(2026, 9, 1)
    assert got.listing_url == "https://avito.ru/moskva/kvartiry/1"
    assert len(client.messages.calls) == 1


def test_nulls_survive():
    got, _ = extract.extract(FakeClient(json.dumps(EMPTY)), STATIONS, "видео без текста")

    assert got.kind == "other"
    assert got.station is None
    assert got.price_month_total is None
    assert got.move_in is None
    assert got.restrictions == []


def test_call_carries_model_schema_and_stations():
    client = FakeClient(json.dumps(FULL))

    extract.extract(client, STATIONS, "текст поста")
    call = client.messages.calls[0]

    assert call["model"] == extract.MODEL
    assert call["output_config"]["format"]["schema"] is extract.EXTRACT_SCHEMA
    assert call["messages"] == [{"role": "user", "content": "текст поста"}]

    system = call["system"][0]
    assert system["cache_control"] == {"type": "ephemeral"}
    assert "Савёловская" in system["text"]
    assert system["text"].startswith(extract.SYSTEM_PROMPT)


def test_broken_json_is_retried_once():
    client = FakeClient("не json", json.dumps(FULL))

    got, _ = extract.extract(client, STATIONS, "текст поста")

    assert got.kind == "rent_flat"
    assert len(client.messages.calls) == 2


def test_second_failure_raises():
    client = FakeClient("не json", "тоже не json")

    with pytest.raises(ExtractError):
        extract.extract(client, STATIONS, "текст поста")
    assert len(client.messages.calls) == 2


def test_answer_off_schema_is_retried():
    off = dict(FULL)
    del off["price_month_total"]
    client = FakeClient(json.dumps(off), json.dumps(FULL))

    got, _ = extract.extract(client, STATIONS, "текст поста")

    assert got.price_month_total == 85000
    assert len(client.messages.calls) == 2


def test_unknown_kind_is_off_schema():
    off = dict(FULL, kind="аренда")
    client = FakeClient(json.dumps(off), json.dumps(off))

    with pytest.raises(ExtractError):
        extract.extract(client, STATIONS, "текст поста")


def test_garbage_date_does_not_lose_the_post():
    got, _ = extract.extract(
        FakeClient(json.dumps(dict(FULL, move_in="осенью"))), STATIONS, "текст поста"
    )

    assert got.move_in is None
    assert got.kind == "rent_flat"


def test_text_blocks_are_joined():
    parts = json.dumps(FULL)
    client = FakeClient(json.dumps(FULL))
    client.messages.replies = []
    client.messages.create = lambda **kwargs: _split_response(parts)

    got, _ = extract.extract(client, STATIONS, "текст поста")

    assert got.kind == "rent_flat"


def test_nullable_enums_use_anyof():
    # enum рядом с union-типом API отвергает четырёхсотым — регрессия дорогая.
    for name in ("bathrooms", "utilities"):
        prop = extract.EXTRACT_SCHEMA["properties"][name]
        assert "enum" not in prop
        assert prop["anyOf"][1] == {"type": "null"}


def _split_response(text):
    response = FakeResponse("")
    half = len(text) // 2
    response.content = [FakeBlock(text[:half]), FakeBlock(text[half:])]
    return response
