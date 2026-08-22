"""Похожесть текстов объявлений: чистые функции, БД не нужна."""

import dedup

ORIGINAL = """м.Ботанический сад6️⃣

📌Просторная, светлая евродвушка в бизнес ЖК Грин парк. Потолки 2,8 метра.
B квартире выполнен качественный дизайнерский ремонт. 38 м².

Адрес: Олонецкая улица, 6

Бытовая техника: стиральная машина с сушкой, холодильник, варочная панель,
микроволновая печь, телевизор, посудомоечная машина, кондиционер.

✔️Цена: 89.900₽ + счетчики.

📱+7-912-272-79-45
Аренда квартиры Москва"""

REPOST = """2-к квартира🏙️
м.Ботанический сад6️⃣

Просторная, светлая евродвушка в бизнес ЖК Грин парк. Потолки 2,8 метра.
B квартире выполнен качественный дизайнерский ремонт. 38 м².

📍Адрес: Олонецкая улица, 6

Бытовая техника: 🔹стиральная машина с сушкой, 🔹холодильник, 🔹варочная панель,
🔹микроволновая печь, 🔹телевизор, 🔹посудомоечная машина, 🔹кондиционер.

💳Цена: 89.900₽ + счетчики.

💌Для связи: +7-912-272-79-45
@fanil1 https://t.me/ugnest_msk #ботанический"""

TEMPLATE_A = """СТУДИЯ, 20.0 м²
м Братиславская
• 17 мин пешком
• 22 этаж из 33
57 000 ₽/мес, БЕЗ КОМИССИИ
Холодильник • Стиральная машина • Кондиционер • Мебель • Интернет • Телевизор • Лифт
Видовая студия в новом ЖК «Люблинский парк» с закрытым двором
#студия #55K60K
Контакт для связи: @leriny
🛡 Проверено Администратором Ливингрум
выложить квартиру бесплатно"""

TEMPLATE_B = """ДВУШКА, 54.0 м²
м Братиславская
• 5 мин пешком
• 3 этаж из 12
57 000 ₽/мес, БЕЗ КОМИССИИ
Холодильник • Стиральная машина • Мебель • Интернет • Телевизор • Лифт
Тихая квартира после косметического ремонта, окна во двор, рядом школа и парк
#двушка #55K60K
Контакт для связи: @ivanov
🛡 Проверено Администратором Ливингрум
выложить квартиру бесплатно"""


def test_tokens_drop_links_mentions_tags_and_decorations():
    got = dedup.tokens("Сдаётся 🏙️ студия! @leriny #студия https://t.me/x/1 t.me/y/2 25 м²")

    assert "сдаётся" in got
    assert "студия" in got
    assert "25" in got
    assert not {"leriny", "@leriny", "#студия", "https", "t.me", "🏙️"} & got


def test_repost_with_other_decorations_and_footer_is_the_same_listing():
    assert dedup.same_listing(dedup.tokens(ORIGINAL), dedup.tokens(REPOST))


def test_truncated_repost_is_the_same_listing():
    cut = ORIGINAL[:200] + "…"

    assert dedup.same_listing(dedup.tokens(ORIGINAL), dedup.tokens(cut))


def test_two_flats_from_one_templated_channel_differ():
    assert not dedup.same_listing(dedup.tokens(TEMPLATE_A), dedup.tokens(TEMPLATE_B))


def test_same_template_with_other_numbers_is_another_flat():
    other = (
        TEMPLATE_A.replace("20.0 м²", "36.0 м²")
        .replace("17 мин", "4 мин")
        .replace("22 этаж из 33", "4 этаж из 9")
    )

    assert not dedup.same_listing(dedup.tokens(TEMPLATE_A), dedup.tokens(other))


def test_short_texts_are_never_the_same_listing():
    short = dedup.tokens("Сдаётся двушка у метро Сокол")

    assert not dedup.same_listing(short, short)
