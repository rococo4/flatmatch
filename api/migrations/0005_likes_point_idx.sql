-- +goose Up

-- «N откликов» на карточке «к себе» читается по точке, а первичный ключ
-- likes_users начинается с from_id. Частичный: зонных лайков большинство,
-- и в индексе им делать нечего.
CREATE INDEX likes_users_point_idx ON likes_users (point_id) WHERE point_id IS NOT NULL;

-- +goose Down

DROP INDEX likes_users_point_idx;
