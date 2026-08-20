-- +goose Up

ALTER TABLE users ADD COLUMN source text NULL;

CREATE TABLE reports (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id uuid NOT NULL REFERENCES users (id),
    target_id   uuid NOT NULL REFERENCES users (id),
    reason      text NOT NULL CHECK (reason IN ('spam', 'scam', 'abuse', 'other')),
    context_key text NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz NULL,
    verdict     text NULL CHECK (verdict IN ('banned', 'skipped')),
    UNIQUE (reporter_id, target_id)
);

-- +goose Down

DROP TABLE reports;
ALTER TABLE users DROP COLUMN source;
