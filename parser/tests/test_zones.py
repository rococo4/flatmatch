"""Справочник станций: словарь для мапинга ответа LLM и список имён для промпта."""

import json
from pathlib import Path

import pytest

import zones

MOSCOW = Path(__file__).parents[2] / "api" / "internal" / "service" / "zones" / "moscow.json"


def test_dictionary_holds_only_stations():
    table = zones.stations(MOSCOW)
    raw = json.loads(MOSCOW.read_text(encoding="utf-8"))
    station_ids = {z["id"] for z in raw["zones"] if z["kind"] == "station"}

    assert set(table.values()) == station_ids
    assert len(table) == len(station_ids)


def test_lookup_is_case_and_yo_insensitive():
    table = zones.stations(MOSCOW)

    assert table[zones.normalized("Сокол")] == "st-sokol"
    assert table[zones.normalized("сокол")] == "st-sokol"
    assert table[zones.normalized("  СОКОЛ  ")] == "st-sokol"
    assert table[zones.normalized("Савеловская")] == "st-savelovskaya"
    assert table[zones.normalized("Савёловская")] == "st-savelovskaya"


def test_unknown_station_is_absent():
    assert zones.normalized("Химки") not in zones.stations(MOSCOW)


def test_names_are_stable_and_human():
    names = zones.station_names(MOSCOW)

    assert names == zones.station_names(MOSCOW)
    assert len(names) == len(zones.stations(MOSCOW))
    assert "Сокол" in names
    assert "Савёловская" in names


def test_missing_file_is_an_error(tmp_path):
    with pytest.raises(OSError):
        zones.stations(tmp_path / "nope.json")
