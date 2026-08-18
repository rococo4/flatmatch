-- +goose Up

ALTER TABLE points
    ADD COLUMN last_seen_alive timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN gone_count      smallint    NOT NULL DEFAULT 0,
    ADD COLUMN ping_asked_at   timestamptz NULL;
UPDATE points SET last_seen_alive = created_at;

CREATE TABLE point_votes (
    user_id    uuid NOT NULL REFERENCES users (id),
    point_id   uuid NOT NULL REFERENCES points (id),
    alive      boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, point_id)
);
CREATE INDEX point_votes_gone_idx ON point_votes (point_id) WHERE NOT alive;

ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK (type IN (
    'zone_new', 'pool_new', 'liked', 'matched',
    'profile_ok', 'photo_rejected', 'about_rejected',
    'point_ok', 'point_rejected',
    'still_searching', 'post_match',
    'point_gone', 'point_ping'));

-- +goose Down

ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK (type IN (
    'zone_new', 'pool_new', 'liked', 'matched',
    'profile_ok', 'photo_rejected', 'about_rejected',
    'point_ok', 'point_rejected',
    'still_searching', 'post_match'));
DROP TABLE point_votes;
ALTER TABLE points DROP COLUMN ping_asked_at, DROP COLUMN gone_count, DROP COLUMN last_seen_alive;
