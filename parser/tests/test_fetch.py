"""Разбор сохранённых фикстур t.me — живых запросов в тестах нет."""

from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

import fetch

FIXTURES = Path(__file__).parent / "fixtures"


def fixture(name: str) -> str:
    return (FIXTURES / name).read_text(encoding="utf-8")


def client_for(handler) -> httpx.Client:
    return httpx.Client(transport=httpx.MockTransport(handler))


def responder(html: str, status: int = 200):
    return lambda request: httpx.Response(status, html=html)


def test_feed_parses_fixture():
    with client_for(responder(fixture("feed_arendamsc.html"))) as client:
        posts = fetch.feed(client, "arendamsc")

    assert [p.message_id for p in posts] == [26240, 26243, 26244, 26245]
    assert all(p.channel == "arendamsc" for p in posts)


def test_feed_keeps_line_breaks_and_photos():
    with client_for(responder(fixture("feed_arendamsc.html"))) as client:
        posts = {p.message_id: p for p in fetch.feed(client, "arendamsc")}

    post = posts[26240]
    assert post.text.startswith("📌Доброго времени суток!\nМеня зовут Софья")
    assert "Бюджет 30 тыс" in post.text
    assert post.text.count("\n") >= 4
    assert len(post.photo_urls) == 2
    assert all(url.startswith("https://cdn") for url in post.photo_urls)
    assert post.posted_at == datetime(2026, 8, 17, 14, 48, 16, tzinfo=timezone.utc)


def test_feed_post_without_text_and_photos():
    with client_for(responder(fixture("feed_arendamsc.html"))) as client:
        posts = {p.message_id: p for p in fetch.feed(client, "arendamsc")}

    assert posts[26244].text == ""
    assert posts[26244].photo_urls == []


def test_feed_marks_forward():
    with client_for(responder(fixture("feed_arendamsc.html"))) as client:
        posts = {p.message_id: p for p in fetch.feed(client, "arendamsc")}

    assert [mid for mid, p in posts.items() if p.is_forward] == [26245]


def test_feed_uses_requested_channel_name():
    with client_for(responder(fixture("feed_arendamsc.html"))) as client:
        posts = fetch.feed(client, "test-alias")

    assert {p.channel for p in posts} == {"test-alias"}


def test_feed_raises_on_non_200():
    with client_for(responder("", status=404)) as client:
        with pytest.raises(httpx.HTTPStatusError):
            fetch.feed(client, "arendamsc")


def test_post_alive_true_on_live_embed():
    with client_for(responder(fixture("embed_alive.html"))) as client:
        assert fetch.post_alive(client, "arendamsc", 26257) is True


def test_post_alive_false_on_deleted_embed():
    with client_for(responder(fixture("embed_deleted.html"))) as client:
        assert fetch.post_alive(client, "arendamsc", 999999) is False


def test_post_alive_none_on_server_error():
    with client_for(responder("", status=500)) as client:
        assert fetch.post_alive(client, "arendamsc", 26257) is None


def test_post_alive_none_on_network_error():
    def boom(request):
        raise httpx.ConnectError("нет сети", request=request)

    with client_for(boom) as client:
        assert fetch.post_alive(client, "arendamsc", 26257) is None


def test_post_alive_none_on_unknown_page():
    with client_for(responder("<html><body>что-то другое</body></html>")) as client:
        assert fetch.post_alive(client, "arendamsc", 26257) is None
