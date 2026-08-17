-- +goose Up

CREATE TABLE notifications (
    id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id), -- адресат

    type text NOT NULL CHECK (type IN (
        'zone_new', 'pool_new', 'liked', 'matched',
        'profile_ok', 'photo_rejected', 'about_rejected',
        'point_ok', 'point_rejected',
        'still_searching', 'post_match')),
    actor_key text NULL,
    context_key text NULL,
    payload jsonb NOT NULL DEFAULT '{}',

    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at    timestamptz NULL,
    failed_at  timestamptz NULL,
    attempts   int NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX notifications_actor
    ON notifications (user_id, type, actor_key, coalesce(context_key, ''))
    WHERE actor_key IS NOT NULL;

CREATE INDEX notifications_queue ON notifications (user_id, created_at)
    WHERE sent_at IS NULL AND failed_at IS NULL;

CREATE INDEX notifications_sent ON notifications (user_id, type, context_key, sent_at DESC)
    WHERE sent_at IS NOT NULL;

ALTER TABLE users ADD COLUMN push_blocked_at timestamptz NULL;

ALTER TABLE searches ADD COLUMN last_active_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE searches ADD COLUMN ttl_asked_at timestamptz NULL;

CREATE TABLE match_polls (
    match_id uuid NOT NULL REFERENCES matches (id),
    user_id  uuid NOT NULL REFERENCES users (id),
    answer   text NOT NULL CHECK (answer IN ('moving', 'talking', 'failed')),

    answered_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (match_id, user_id)
);

-- +goose Down

DROP TABLE match_polls;

ALTER TABLE searches DROP COLUMN ttl_asked_at;

ALTER TABLE searches DROP COLUMN last_active_at;

ALTER TABLE users DROP COLUMN push_blocked_at;

DROP TABLE notifications;
