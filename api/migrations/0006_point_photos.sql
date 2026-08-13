-- +goose Up

-- У точки много фото (решение от 2026-08-14): скрины объявления или несколько
-- ракурсов комнаты. Первое — обложка карточки и кадр для модерации.
ALTER TABLE points ADD COLUMN photo_keys text[] NOT NULL DEFAULT '{}';

UPDATE points SET photo_keys = ARRAY[photo_key] WHERE photo_key IS NOT NULL;

ALTER TABLE points DROP COLUMN photo_key;

-- +goose Down

ALTER TABLE points ADD COLUMN photo_key text NULL;

UPDATE points SET photo_key = photo_keys[1] WHERE cardinality(photo_keys) > 0;

ALTER TABLE points DROP COLUMN photo_keys;
