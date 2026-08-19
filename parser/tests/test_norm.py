"""Нормализация ссылок — порт кейсов из api/internal/service/points/service_test.go.

Расхождение с Go = провал дедупа с пользовательскими подачами той же ссылки,
поэтому кейсы перенесены один в один.
"""

import pytest

from norm import normalize_listing_url

WANT = "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567"

GOOD = {
    "plain": "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
    "www": "https://www.avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
    "mobile mirror": "https://m.avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
    "query": "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567?context=H4sI&slocation=1",
    "trailing slash": "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567/",
    "two slashes": "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567//",
    "percent-encoded": "https://avito.ru/moskva/kvartiry/2-k_kv%61rtira_1234567",
    "http": "http://avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
    "no scheme": "avito.ru/moskva/kvartiry/2-k_kvartira_1234567",
    "uppercase host": "https://AVITO.RU/moskva/kvartiry/2-k_kvartira_1234567",
    "spaces around it": "  https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567  ",
    "fragment": "https://avito.ru/moskva/kvartiry/2-k_kvartira_1234567#photo",
}

BAD = {
    "empty": "",
    "other classified": "https://realty.yandex.ru/offer/1",
    "lookalike domain": "https://avito.ru.evil.com/1",
    "subdomain": "https://spb.avito.ru/kvartira/1",
    "not a url": "квартира на авито",
    "other scheme": "ftp://avito.ru/1",
    "bare host": "https://avito.ru",
    "bare host with a slash": "https://avito.ru/",
    "bare host no scheme": "avito.ru",
}


@pytest.mark.parametrize("raw", GOOD.values(), ids=list(GOOD))
def test_good_links_normalize(raw):
    assert normalize_listing_url(raw) == WANT


@pytest.mark.parametrize("raw", BAD.values(), ids=list(BAD))
def test_bad_links_are_refused(raw):
    assert normalize_listing_url(raw) is None


def test_path_case_survives():
    assert normalize_listing_url("https://avito.ru/Moskva/Kvartiry/1") == (
        "https://avito.ru/Moskva/Kvartiry/1"
    )


def test_cian_link():
    assert normalize_listing_url("https://www.cian.ru/rent/flat/123/") == (
        "https://cian.ru/rent/flat/123"
    )
