-- +goose Up

CREATE TABLE likes_users (
    from_id  uuid NOT NULL REFERENCES users (id),
    to_id    uuid NOT NULL REFERENCES users (id),
    -- Контекст «где пересеклись» (§2.3). У ответного лайка его может не быть:
    -- отвечают с экрана «Тебя лайкнули», а не из пула.
    zone_id  text NULL,
    point_id uuid NULL, -- заготовка шага 5, пока всегда NULL

    -- Тратил ли лайк слот из 5. Решается в момент вставки и больше не
    -- пересчитывается: после мэтча встречные лайки есть у обоих, и «был ли
    -- этот ответом» задним числом уже не восстановить.
    spent      boolean NOT NULL,
    -- Несёт окно бюджета (скользящие 24ч), next_free_at и выбор первого лайка
    -- пары, чей контекст становится контекстом мэтча.
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (from_id, to_id),
    CHECK (from_id <> to_id)
);

CREATE INDEX likes_users_to_idx ON likes_users (to_id);

CREATE INDEX likes_users_budget_idx ON likes_users (from_id, created_at) WHERE spent;

CREATE TABLE matches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    a_id uuid NOT NULL REFERENCES users (id),
    b_id uuid NOT NULL REFERENCES users (id),
    -- Копия контекста ПЕРВОГО лайка пары: саммари мэтча говорит, где
    -- пересеклись, а поиски к тому моменту могут быть уже закрыты (§2.3).
    context_zone_id  text NULL,
    context_point_id uuid NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),

    -- Пара канонична, поэтому одновременные взаимные лайки бьются в один
    -- уникальный ключ и дубля мэтча не создают (§2.3).
    CHECK (a_id < b_id)
);

CREATE UNIQUE INDEX matches_pair ON matches (a_id, b_id);

-- +goose Down

DROP TABLE matches;
DROP TABLE likes_users;
