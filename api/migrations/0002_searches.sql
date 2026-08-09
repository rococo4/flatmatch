-- +goose Up

CREATE TABLE searches (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id),
    zone_id      text NOT NULL,
    price_min    int NULL,
    price_max    int NOT NULL,
    rooms_min    int NULL,
    rooms_max    int NULL,
    move_in      date NULL, -- first day of the move-in month
    term         text NULL CHECK (term IN ('half_year', 'year', 'open')),
    has_stuff    text[] NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'paused', 'closed')),
    close_reason text NULL
                 CHECK (close_reason IN ('found_here', 'found_elsewhere', 'changed_mind')),

    pool_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'closed') = (close_reason IS NOT NULL)),
    CHECK (price_min IS NULL OR price_min <= price_max),
    CHECK (rooms_min IS NULL OR rooms_max IS NULL OR rooms_min <= rooms_max)
);

CREATE INDEX searches_user_idx ON searches (user_id);

CREATE INDEX searches_active_zone_idx ON searches (zone_id) WHERE status = 'active';

CREATE TABLE blocks (
    blocker_id uuid NOT NULL REFERENCES users (id),
    blocked_id uuid NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

-- +goose StatementBegin
CREATE FUNCTION visible_people(viewer uuid) RETURNS SETOF uuid
LANGUAGE sql STABLE AS $$
    SELECT c.id
    FROM users c, users v
    WHERE v.id = viewer
      AND c.id <> v.id
      AND c.status = 'active'
      AND (v.gender_pref = 'any' OR c.gender = v.gender)
      AND (c.gender_pref = 'any' OR v.gender = c.gender)
      AND NOT EXISTS (
          SELECT 1 FROM blocks b
          WHERE (b.blocker_id = v.id AND b.blocked_id = c.id)
             OR (b.blocker_id = c.id AND b.blocked_id = v.id)
      )
$$;
-- +goose StatementEnd

-- +goose Down

DROP FUNCTION visible_people(uuid);
DROP TABLE blocks;
DROP TABLE searches;
