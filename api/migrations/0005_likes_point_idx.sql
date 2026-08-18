-- +goose Up

CREATE INDEX likes_users_point_idx ON likes_users (point_id) WHERE point_id IS NOT NULL;

-- +goose Down

DROP INDEX likes_users_point_idx;
