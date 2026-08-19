"""Нормализация ссылки объявления — порт normalizeURL из
api/internal/service/points/validate.go.

Одна и та же ссылка обязана давать одну и ту же строку в обоих сервисах:
url_norm — носитель дедупа между парсерными точками и пользовательскими
подачами (§8), расхождение здесь тихо ломает его.
"""

import re
from urllib.parse import unquote, urlsplit

LISTING_HOSTS = ("avito.ru", "cian.ru")
HOST_ALIASES = ("www.", "m.")

_ESCAPE_RE = re.compile("%(?![0-9A-Fa-f]{2})")
_CTL_RE = re.compile("[\x00-\x1f\x7f]")


def normalize_listing_url(raw: str) -> str | None:
    raw = raw.strip()
    if not raw:
        return None
    # net/url отвергает управляющие символы ещё до разбора.
    if _CTL_RE.search(raw):
        return None
    if "://" not in raw:
        raw = "https://" + raw

    try:
        parsed = urlsplit(raw)
        host = parsed.hostname or ""
    except ValueError:
        return None
    if parsed.scheme not in ("http", "https"):
        return None
    if _ESCAPE_RE.search(parsed.netloc + parsed.path + parsed.fragment):
        return None

    host = host.lower()
    for alias in HOST_ALIASES:
        host = host.removeprefix(alias)
    if host not in LISTING_HOSTS:
        return None

    # Go отдаёт u.Path уже раскодированным — %61 обязан стать «a».
    path = unquote(parsed.path).rstrip("/")
    if not path:
        return None
    return "https://" + host + path
