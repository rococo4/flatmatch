-- +goose Up

ALTER TABLE users DROP CONSTRAINT users_occupation_check;

ALTER TABLE users ADD CONSTRAINT users_occupation_check
    CHECK (occupation IN ('study', 'office', 'hybrid', 'remote', 'shift', 'none'));

ALTER TABLE users ADD COLUMN photo_keys text[] NOT NULL DEFAULT '{}';

UPDATE users SET photo_keys = ARRAY[photo_key] WHERE photo_key IS NOT NULL;

ALTER TABLE users DROP COLUMN photo_key;

ALTER TABLE users ADD CONSTRAINT photo_ok_needs_a_file
    CHECK (photo_status <> 'ok' OR cardinality(photo_keys) > 0);

ALTER TABLE points ADD COLUMN description text NULL CHECK (char_length(description) <= 400);

CREATE TABLE passes (
    user_id    uuid NOT NULL REFERENCES users (id),
    target_id  uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, target_id)
);

CREATE INDEX passes_user_created_idx ON passes (user_id, created_at);

-- +goose Down

DROP TABLE passes;

ALTER TABLE points DROP COLUMN description;

ALTER TABLE users DROP CONSTRAINT photo_ok_needs_a_file;

ALTER TABLE users ADD COLUMN photo_key text;

UPDATE users SET photo_key = photo_keys[1] WHERE cardinality(photo_keys) > 0;

ALTER TABLE users DROP COLUMN photo_keys;

ALTER TABLE users ADD CONSTRAINT photo_ok_needs_a_file
    CHECK (photo_status <> 'ok' OR photo_key IS NOT NULL);

UPDATE users SET occupation = NULL WHERE occupation = 'none';

ALTER TABLE users DROP CONSTRAINT users_occupation_check;

ALTER TABLE users ADD CONSTRAINT users_occupation_check
    CHECK (occupation IN ('study', 'office', 'hybrid', 'remote', 'shift'));
