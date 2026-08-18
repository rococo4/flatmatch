-- +goose Up

CREATE TABLE likes_users (
    from_id  uuid NOT NULL REFERENCES users (id),
    to_id    uuid NOT NULL REFERENCES users (id),
    zone_id  text NULL,
    point_id uuid NULL,

    spent      boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (from_id, to_id),
    CHECK (from_id <> to_id)
);

CREATE INDEX likes_users_to_idx ON likes_users (to_id);

CREATE INDEX likes_users_budget_idx ON likes_users (from_id, created_at) WHERE spent;

CREATE TABLE matches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    a_id uuid NOT NULL REFERENCES users (id),
    b_id uuid NOT NULL REFERENCES users (id),
    context_zone_id  text NULL,
    context_point_id uuid NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),

    CHECK (a_id < b_id)
);

CREATE UNIQUE INDEX matches_pair ON matches (a_id, b_id);

-- +goose Down

DROP TABLE matches;
DROP TABLE likes_users;
