-- +goose Up

CREATE TABLE points (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    type   text NOT NULL CHECK (type IN ('flat', 'intent_home')),
    stage  text NULL CHECK (stage IN ('live', 'moving_in')),
    status text NOT NULL DEFAULT 'pending'
           CHECK (status IN ('pending', 'active', 'paused', 'closed')),
    source text NOT NULL
           CHECK (source IN ('tg_channel', 'user_link', 'manual', 'realtor')),
    created_by uuid NULL REFERENCES users (id),

    zone_id text NOT NULL,

    price_total int NULL,
    price_share int NULL,
    rooms       int NULL,
    isolated    boolean NULL,
    bathrooms text NULL CHECK (bathrooms IN ('combined', 'separate', 'two')),
    utilities text NULL CHECK (utilities IN ('included', 'meters')),
    restrictions text[] NOT NULL DEFAULT '{}'
                 CHECK (restrictions <@ ARRAY['no_pets', 'no_couples', 'no_smoking']::text[]),
    photo_key text NULL,
    url_norm text NULL,

    move_in      date NULL,
    deposit      int NULL,
    close_reason text NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at  timestamptz NOT NULL DEFAULT now(),

    CHECK ((created_by IS NULL) = (source IN ('tg_channel', 'manual'))),
    CHECK ((stage IS NOT NULL) = (type = 'intent_home')),
    CHECK (type <> 'intent_home' OR created_by IS NOT NULL),
    CHECK ((status = 'closed') = (close_reason IS NOT NULL)),
    CHECK (stage IS DISTINCT FROM 'moving_in' OR move_in IS NOT NULL)
);

CREATE UNIQUE INDEX points_url_norm_active ON points (url_norm) WHERE status <> 'closed';

CREATE INDEX points_active_zone_idx ON points (zone_id) WHERE status = 'active';

CREATE INDEX points_created_by_idx ON points (created_by);

CREATE TABLE likes_points (
    user_id    uuid NOT NULL REFERENCES users (id),
    point_id   uuid NOT NULL REFERENCES points (id),
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, point_id)
);

CREATE INDEX likes_points_point_idx ON likes_points (point_id);

-- +goose Down

DROP TABLE likes_points;
DROP TABLE points;
