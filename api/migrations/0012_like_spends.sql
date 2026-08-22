-- +goose Up

CREATE TABLE like_spends (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id    uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX like_spends_user_idx ON like_spends (user_id, created_at);

INSERT INTO like_spends (user_id, created_at)
SELECT from_id, created_at FROM likes_users WHERE spent;

-- +goose Down

DROP TABLE like_spends;
