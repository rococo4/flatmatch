-- +goose Up

CREATE TABLE channel_posts (
    channel    text NOT NULL,
    message_id bigint NOT NULL,
    posted_at  timestamptz NULL,
    text       text NOT NULL,
    text_hash  text NOT NULL,
    photo_urls text[] NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'new'
           CHECK (status IN ('new', 'skipped', 'point_created', 'error')),
    skip_reason  text NULL,
    llm          jsonb NULL,
    point_id     uuid NULL REFERENCES points (id),
    possible_dup boolean NOT NULL DEFAULT false,
    first_seen_at   timestamptz NOT NULL DEFAULT now(),
    last_checked_at timestamptz NULL,
    deleted_at      timestamptz NULL,
    PRIMARY KEY (channel, message_id)
);
CREATE INDEX channel_posts_new_idx ON channel_posts (first_seen_at) WHERE status = 'new';
CREATE INDEX channel_posts_point_idx ON channel_posts (point_id) WHERE point_id IS NOT NULL;

-- +goose Down

DROP TABLE channel_posts;
