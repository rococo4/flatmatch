-- +goose Up

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity from Telegram. tg_id is the natural key: it is what initData
    -- carries and what find-or-create looks up.
    tg_id       bigint NOT NULL UNIQUE,
    -- Display name. Seeded from Telegram on first launch, editable afterwards,
    -- which is why later launches must not overwrite it.
    name        text   NOT NULL,
    -- @username, needed for the "Написать @username" button on a match (§6).
    -- Not every Telegram account has one.
    tg_username text,

    age smallint CHECK (age BETWEEN 16 AND 100),

    -- «Пол»
    gender text CHECK (gender IN ('male', 'female')),

    gender_pref text NOT NULL DEFAULT 'any' CHECK (gender_pref IN ('any', 'same')),

    -- «Чем занимаешься?» — «Учусь» / «Офис» / «Гибрид» / «Удалёнка» / «Сменный график»
    occupation text CHECK (occupation IN ('study', 'office', 'hybrid', 'remote', 'shift')),
    -- «Когда ты активен?» — «Сова» / «Жаворонок» / «Плавает»
    schedule text CHECK (schedule IN ('owl', 'lark', 'floating')),
    -- «Куришь?» — «Не курю» / «На балконе или улице» / «Дома»
    smoking text CHECK (smoking IN ('no', 'outside', 'home')),
    -- «Свои животные?» — «Нет» / «Кот» / «Собака» / «Другое»
    pets_own text CHECK (pets_own IN ('none', 'cat', 'dog', 'other')),
    -- «Как к чужим?» — «Ок» / «Аллергия» / «Против»
    pets_ok text CHECK (pets_ok IN ('ok', 'allergy', 'against')),
    -- «Какой звук дома норм?» — «Нужна тишина» / «Нормально» / «Сам шумный»
    noise text CHECK (noise IN ('silence', 'normal', 'loud')),
    -- «Как часто у тебя гости?» — «Редко» / «Иногда» / «Часто» / «Партнёр регулярно ночует»
    guests text CHECK (guests IN ('rare', 'sometimes', 'often', 'partner')),
    -- «Как у тебя с уборкой?» — «По расписанию» / «Убираю, когда грязно» / «Творческий беспорядок»
    cleaning text CHECK (cleaning IN ('schedule', 'when_dirty', 'creative_mess')),
    -- «Алкоголь дома?» — «Не пью» / «С соседом за сериалом — да» / «Тусовки — норм»
    alcohol text CHECK (alcohol IN ('none', 'with_neighbor', 'parties')),

    about text CHECK (char_length(about) <= 200),

    photo_url text,

    photo_status text NOT NULL DEFAULT 'pending'
        CHECK (photo_status IN ('pending', 'ok', 'rejected')),

    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'pending', 'active', 'banned')),


    last_seen  timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE users;
