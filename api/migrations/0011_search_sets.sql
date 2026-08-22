-- +goose Up

ALTER TABLE searches
    ADD COLUMN rooms          int[]  NOT NULL DEFAULT '{}',
    ADD COLUMN terms          text[] NOT NULL DEFAULT '{}',
    ADD COLUMN move_in_months date[] NOT NULL DEFAULT '{}';

UPDATE searches SET
    rooms = CASE
        WHEN rooms_min IS NULL AND rooms_max IS NULL THEN '{}'
        ELSE (SELECT array_agg(DISTINCT least(n, 4) ORDER BY least(n, 4))
              FROM generate_series(least(coalesce(rooms_min, 1), 4),
                                   least(coalesce(rooms_max, 4), 4)) n)
    END,
    terms = CASE WHEN term IS NULL THEN '{}' ELSE ARRAY[term] END,
    move_in_months = CASE WHEN move_in IS NULL THEN '{}' ELSE ARRAY[move_in] END;

ALTER TABLE searches
    DROP COLUMN rooms_min, DROP COLUMN rooms_max, DROP COLUMN move_in, DROP COLUMN term,
    ADD CONSTRAINT searches_rooms_range CHECK (rooms <@ ARRAY[1, 2, 3, 4]),
    ADD CONSTRAINT searches_terms_known CHECK (terms <@ ARRAY['half_year', 'year', 'open']);

-- +goose Down

ALTER TABLE searches
    DROP CONSTRAINT searches_rooms_range,
    DROP CONSTRAINT searches_terms_known,
    ADD COLUMN rooms_min int NULL,
    ADD COLUMN rooms_max int NULL,
    ADD COLUMN move_in date NULL,
    ADD COLUMN term text NULL CHECK (term IN ('half_year', 'year', 'open')),
    ADD CONSTRAINT searches_rooms_order
        CHECK (rooms_min IS NULL OR rooms_max IS NULL OR rooms_min <= rooms_max);

UPDATE searches SET
    rooms_min = (SELECT min(n) FROM unnest(rooms) n),
    rooms_max = (SELECT nullif(max(n), 4) FROM unnest(rooms) n),
    term = terms[1],
    move_in = move_in_months[1];

ALTER TABLE searches
    DROP COLUMN rooms, DROP COLUMN terms, DROP COLUMN move_in_months;
