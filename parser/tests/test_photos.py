"""Скачивание фото: тип решают magic bytes, живых обращений к CDN нет."""

from pathlib import Path

import httpx

import photos

JPEG = b"\xff\xd8\xff" + b"\x00" * 20
PNG = b"\x89PNG\r\n\x1a\n" + b"\x00" * 20
WEBP = b"RIFF" + b"\x00" * 4 + b"WEBPVP8 " + b"\x00" * 20
NOT_IMAGE = b"<html><body>not a photo</body></html>"


def client_for(handler) -> httpx.Client:
    return httpx.Client(transport=httpx.MockTransport(handler))


def responder(body: bytes, status: int = 200):
    return lambda request: httpx.Response(status, content=body)


def test_jpeg_is_saved_under_a_points_key(tmp_path):
    with client_for(responder(JPEG)) as client:
        keys = photos.download(client, ["https://cdn/1.jpg"], str(tmp_path))

    assert len(keys) == 1
    assert keys[0].startswith("points/") and keys[0].endswith(".jpg")
    assert (tmp_path / keys[0]).read_bytes() == JPEG


def test_png_and_webp_are_recognized_by_magic_bytes(tmp_path):
    def handler(request):
        body = PNG if request.url.path.endswith("1") else WEBP
        return httpx.Response(200, content=body)

    with client_for(handler) as client:
        keys = photos.download(client, ["https://cdn/1", "https://cdn/2"], str(tmp_path))

    assert sorted(Path(key).suffix for key in keys) == [".png", ".webp"]


def test_non_image_is_skipped_not_the_whole_post(tmp_path):
    with client_for(responder(NOT_IMAGE)) as client:
        keys = photos.download(client, ["https://cdn/page.html"], str(tmp_path))

    assert keys == []


def test_http_error_is_skipped(tmp_path):
    with client_for(responder(b"", status=404)) as client:
        keys = photos.download(client, ["https://cdn/gone.jpg"], str(tmp_path))

    assert keys == []


def test_network_error_is_skipped(tmp_path):
    def boom(request):
        raise httpx.ConnectError("нет сети", request=request)

    with client_for(boom) as client:
        keys = photos.download(client, ["https://cdn/1.jpg"], str(tmp_path))

    assert keys == []


def test_one_bad_url_does_not_drop_the_good_ones(tmp_path):
    def handler(request):
        if "bad" in request.url.path:
            return httpx.Response(404)
        return httpx.Response(200, content=JPEG)

    with client_for(handler) as client:
        keys = photos.download(client, ["https://cdn/bad", "https://cdn/good.jpg"], str(tmp_path))

    assert len(keys) == 1


def test_limit_caps_the_number_of_downloads(tmp_path):
    urls = [f"https://cdn/{i}.jpg" for i in range(8)]
    with client_for(responder(JPEG)) as client:
        keys = photos.download(client, urls, str(tmp_path), limit=5)

    assert len(keys) == 5


def test_keys_are_unique(tmp_path):
    urls = ["https://cdn/1.jpg", "https://cdn/2.jpg"]
    with client_for(responder(JPEG)) as client:
        keys = photos.download(client, urls, str(tmp_path))

    assert len(set(keys)) == 2


def test_missing_photo_dir_is_created(tmp_path):
    target = tmp_path / "nested" / "dir"
    with client_for(responder(JPEG)) as client:
        keys = photos.download(client, ["https://cdn/1.jpg"], str(target))

    assert (target / keys[0]).exists()


def test_no_urls_returns_empty_list_without_touching_the_client():
    def boom(request):
        raise AssertionError("не должно быть запросов без urls")

    with client_for(boom) as client:
        assert photos.download(client, [], "/nonexistent") == []
