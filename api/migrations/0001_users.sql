-- +goose Up

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    tg_id       bigint NOT NULL UNIQUE,
    name        text   NOT NULL,
    tg_username text,

    age smallint CHECK (age BETWEEN 16 AND 100),

    gender text CHECK (gender IN ('male', 'female')),

    gender_pref text NOT NULL DEFAULT 'any' CHECK (gender_pref IN ('any', 'same')),

    occupation text CHECK (occupation IN ('study', 'office', 'hybrid', 'remote', 'shift')),
    schedule text CHECK (schedule IN ('owl', 'lark', 'floating')),
    smoking text CHECK (smoking IN ('no', 'outside', 'home')),
    pets_own text CHECK (pets_own IN ('none', 'cat', 'dog', 'other')),
    pets_ok text CHECK (pets_ok IN ('ok', 'allergy', 'against')),
    noise text CHECK (noise IN ('silence', 'normal', 'loud')),
    guests text CHECK (guests IN ('rare', 'sometimes', 'often', 'partner')),
    cleaning text CHECK (cleaning IN ('schedule', 'when_dirty', 'creative_mess')),
    alcohol text CHECK (alcohol IN ('none', 'with_neighbor', 'parties')),
    expenses text CHECK (expenses IN ('split_equally', 'tracked', 'as_we_go')),

    about text CHECK (char_length(about) <= 200),

    photo_key text,

    photo_status text NOT NULL DEFAULT 'pending'
        CHECK (photo_status IN ('pending', 'ok', 'rejected')),

    about_status text NOT NULL DEFAULT 'pending'
        CHECK (about_status IN ('pending', 'ok', 'rejected')),

    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'pending', 'active', 'banned')),

    onboarding_step smallint NOT NULL DEFAULT 0
        CHECK (onboarding_step BETWEEN 0 AND 11),

    last_seen  timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT photo_ok_needs_a_file
        CHECK (photo_status <> 'ok' OR photo_key IS NOT NULL)
);

-- +goose Down

DROP TABLE users;
