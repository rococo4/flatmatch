-- +goose Up

CREATE TABLE points (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    type   text NOT NULL CHECK (type IN ('flat', 'intent_home')),
    -- Стадия «к себе»: «уже живу здесь» / «договорился и заезжаю» (§2.2).
    stage  text NULL CHECK (stage IN ('live', 'moving_in')),
    status text NOT NULL DEFAULT 'pending'
           CHECK (status IN ('pending', 'active', 'paused', 'closed')),
    source text NOT NULL
           CHECK (source IN ('tg_channel', 'user_link', 'manual', 'realtor')),
    -- Provenance, лимиты и модерация — НЕ владение: квартира с рынка
    -- обезличена, в продуктовых выборках создатель не участвует (§2.2, §8).
    created_by uuid NULL REFERENCES users (id),

    zone_id text NOT NULL,

    price_total  int NULL, -- flat: цена квартиры целиком
    price_share  int NULL, -- intent_home: цена доли въезжающего
    rooms        int NULL,
    isolated     boolean NULL,
    bathrooms    int NULL,
    utilities    text NULL CHECK (utilities IN ('included', 'meters')),
    restrictions text[] NOT NULL DEFAULT '{}', -- чипы ограничений квартиры
    -- Ключ объекта, а не URL: адреса собирает хранилище (как users.photo_key).
    photo_key text NULL,
    -- Нормализованная ссылка на оригинал; её UNIQUE и есть дедупликация
    -- квартир («эта квартира уже есть, в пуле N — войти?», §6).
    url_norm text NULL UNIQUE,

    move_in      date NULL,
    deposit      int NULL,
    close_reason text NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz NOT NULL DEFAULT now(),

    -- Источники без человека (парсер, ручная досыпка) создателя не имеют,
    -- пользовательские — имеют всегда, иначе лимит не на ком считать (§8).
    CHECK ((created_by IS NULL) = (source IN ('tg_channel', 'manual'))),
    CHECK ((stage IS NOT NULL) = (type = 'intent_home')),
    -- «К себе» — предложение конкретного человека, обезличенным не бывает.
    CHECK (type <> 'intent_home' OR created_by IS NOT NULL)
);

CREATE INDEX points_active_zone_idx ON points (zone_id) WHERE status = 'active';

CREATE INDEX points_created_by_idx ON points (created_by);

CREATE TABLE likes_points (
    -- Дешёвый вход в пул (§2.2). Пул есть только у type='flat', но CHECK
    -- кросс-табличным не бывает — тип гейтится в транзакции входа.
    user_id    uuid NOT NULL REFERENCES users (id),
    point_id   uuid NOT NULL REFERENCES points (id),
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, point_id)
);

-- Пул читается по точке, а первичный ключ начинается с user_id.
CREATE INDEX likes_points_point_idx ON likes_points (point_id);

-- +goose Down

DROP TABLE likes_points;
DROP TABLE points;
