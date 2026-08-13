// Command devseed fills a local database with fake people so that zone pools,
// their three sections and the verdict chips have something to show (§2.1),
// and with a few points so that the feed, the flat pools and «к себе» are not
// empty either (§2.2). Without it every pool is empty and the feed's promo
// block counts nobody.
//
// The crowd is deterministic: fixed Telegram ids from seedTGBase up, upserted
// by tg_id, searches wiped and re-inserted; points carry fixed ids that all
// start with «5eed». Reruns converge to the same state instead of piling up
// copies, and the same two markers remove everything — in reference order,
// and likes and matches name a seed person on either side (the other one is
// usually the dev account of cmd/devinitdata):
//
//	DELETE FROM matches USING users u
//	 WHERE u.tg_id >= 900000001 AND (a_id = u.id OR b_id = u.id);
//	DELETE FROM likes_users USING users u
//	 WHERE u.tg_id >= 900000001 AND (from_id = u.id OR to_id = u.id);
//	DELETE FROM likes_points
//	 WHERE point_id IN (SELECT id FROM points WHERE id::text LIKE '5eed%')
//	    OR user_id IN (SELECT id FROM users WHERE tg_id >= 900000001);
//	DELETE FROM points WHERE id::text LIKE '5eed%';
//	DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id >= 900000001);
//	DELETE FROM users WHERE tg_id >= 900000001;
//
// Usage:
//
//	cd api && go run ./cmd/devseed
//
// It refuses to run against anything but a local database — twenty fake active
// people in front of real users would be a product incident, not a typo.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/config"
	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/photostore"
	"flatmatch/api/internal/postgres"
)

const (
	// seedTGBase is where the fake Telegram ids start. Real accounts never
	// reach this range, so «is this a seed person?» stays a single comparison.
	seedTGBase int64 = 900000001

	// avatarKey is the one photo the whole crowd wears. It ships with
	// photo_status = 'ok' because pool cards hide a photo moderation has not
	// approved (§5) — with anything else the avatar stack comes back empty.
	avatarKey  = "seed/avatar.png"
	avatarSize = 128

	// mediaPrefix mirrors cmd/api. The seeder only needs the store to write a
	// file, but the two must agree on the URL base or the key would be
	// addressed differently by the writer and the reader.
	mediaPrefix = "/media"

	// onboardingDone is the last questionnaire step (§4): these people are past
	// it, and the bookmark should not send anyone back into the form.
	onboardingDone = entity.OnboardingSteps - 1

	// moveInLayout is how the table below spells a move-in month. Only the
	// month is a product concept (§2.1), the day is always the first.
	moveInLayout = "2006-01-02"

	// devTGID is the mock account the browser flow logs in as (cmd/devinitdata,
	// see the README). The seeder never creates it — it only hangs incoming
	// likes on it when it is already there, so «Тебя лайкнули» has something to
	// show without a second Telegram account on the other end.
	devTGID int64 = 777000001

	// devLikeZone is the context of those likes: the pool they were sent from.
	// Хамовники for both, including Ира — her search is on line-1, and line-1
	// reaches the Хамовники pool through its stations (§2.1).
	devLikeZone = "d-khamovniki"
)

// seedPerson is one row of the table: the questionnaire a pool card renders,
// plus the single search that puts them into a zone. Zero values are the NULL
// parameters of §2.1 — «неважно», which intersects everything.
//
// Only the habits that tell people apart on a card are spelled out here; the
// rest of the questionnaire is derived from n (see profile), because those
// answers exist to make SharedAnswers vary, not to be read in this file.
type seedPerson struct {
	n        int64 // offset from seedTGBase, and the seed of the derived habits
	name     string
	age      int
	gender   entity.Gender
	pref     entity.GenderPref
	schedule entity.Schedule
	smoking  entity.Smoking
	petsOwn  entity.PetsOwn
	about    string

	zone     string
	priceMin int         // 0 = NULL
	priceMax int         // mandatory (§2.1 — «до» обязательна)
	roomsMin int         // 0 = NULL
	roomsMax int         // 0 = NULL
	moveIn   string      // "" = NULL, otherwise moveInLayout
	term     entity.Term // "" = NULL
}

// people is the crowd. The verdicts in the comments are against the canonical
// viewer search — Хамовники, 35 000–45 000, сентябрь 2026 — which is what the
// feed and the pool are checked with. Sections follow §2.1: hard parameters
// are price and move-in, and «краем» counts as intersected.
//
// Zones intersect by station sets, so line-1, line-3, line-5, d-arbat,
// d-yakimanka, d-ramenki, d-dorogomilovo, d-presnenskii, d-tverskoi and
// st-park-kultury all reach the Хамовники pool. st-sokol, d-aeroport and
// line-2 deliberately do not: someone has to be outside, or the boundary of a
// pool is untested.
var people = []seedPerson{
	// Полные совпадения: цена и заезд пересеклись.
	{1, "Катя", 22, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: сплю поздно, работаю из дома, ищу спокойную квартиру.",
		"d-khamovniki", 40000, 55000, 0, 0, "2026-09-01", entity.TermYear},
	{2, "Олег", 25, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: со мной переезжает кот, он тихий.",
		"d-khamovniki", 35000, 45000, 2, 2, "2026-09-01", ""},
	// Приходит в пул через ветку: красная проходит по Хамовникам.
	{3, "Ира", 23, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingOutside, entity.PetsOwnNone,
		"Сид для разработки: курю только на балконе, готова смотреть от Сокольников до Юго-Западной.",
		"line-1", 30000, 42000, 0, 0, "2026-10-01", entity.TermHalfYear},
	// «Краем» по цене: моё «до 45» упирается в его «от 45».
	{4, "Дима", 27, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnDog,
		"Сид для разработки: собака среднего размера, гуляем рано утром.",
		"d-khamovniki", 45000, 60000, 0, 0, "2026-09-01", ""},
	// Частично: цена мимо, заезд совпал.
	{5, "Настя", 24, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: бюджет выше среднего, зато готова на весь год.",
		"d-khamovniki", 60000, 80000, 0, 0, "2026-09-01", entity.TermYear},
	// Не совпали: мимо и цена, и заезд.
	{6, "Глеб", 30, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingHome, entity.PetsOwnNone,
		"Сид для разработки: заезжаю ближе к зиме, ищу вариант подороже.",
		"d-khamovniki", 70000, 90000, 0, 0, "2026-12-01", ""},
	// Минимальный поиск: только «до». Комнатность, заезд и срок — «не указана»,
	// три серых чипа-«none» на карточке.
	{7, "Лена", 21, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: сроки плавают, смотрю всё подряд.",
		"d-khamovniki", 0, 50000, 0, 0, "", ""},
	// Обоюдная невидимость (§4): «только свой пол» прячет её от мужчин целиком.
	{8, "Аня", 26, entity.GenderFemale, entity.GenderPrefSame, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: съезжаюсь только с девушками.",
		"d-khamovniki", 35000, 45000, 0, 0, "2026-09-01", ""},
	{9, "Марк", 29, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingOutside, entity.PetsOwnDog,
		"Сид для разработки: нужна двушка или трёшка, работаю в офисе рядом с кольцом.",
		"line-5", 38000, 48000, 2, 3, "2026-09-01", entity.TermYear},
	{10, "Соня", 20, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: студентка, заезжаю в августе, если найду с кем.",
		"d-arbat", 33000, 41000, 1, 1, "2026-08-01", entity.TermHalfYear},
	{11, "Тимур", 31, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingHome, entity.PetsOwnNone,
		"Сид для разработки: сменный график, дома бываю днём.",
		"d-yakimanka", 44000, 52000, 0, 0, "2026-10-01", entity.TermOpen},
	// Частично: цена мимо снизу, заезд совпал.
	{12, "Полина", 24, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnOther,
		"Сид для разработки: бюджет скромный, зато со мной только черепаха.",
		"d-ramenki", 25000, 34000, 0, 0, "2026-09-01", ""},
	// Частично: цена совпала, заезд мимо на два месяца.
	{13, "Женя", 28, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingOutside, entity.PetsOwnNone,
		"Сид для разработки: освобождаюсь только к ноябрю, ищу заранее.",
		"d-dorogomilovo", 36000, 46000, 2, 2, "2026-11-01", entity.TermYear},
	{14, "Артём", 33, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: готов платить больше за отдельную комнату побольше.",
		"line-3", 50000, 70000, 0, 0, "2026-09-01", entity.TermOpen},
	// Не совпали: другой ценовой сегмент и заезд через зиму.
	{15, "Даша", 27, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: ищу трёшку в центре, заезд после Нового года.",
		"d-presnenskii", 80000, 120000, 3, 3, "2027-01-01", entity.TermOpen},
	{16, "Кирилл", 22, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: учусь рядом, с датой заезда пока не определился.",
		"st-park-kultury", 30000, 45000, 1, 2, "", entity.TermHalfYear},
	// Зеркало Ани: «только свой пол» у мужчины. Мужчина его видит, женщина нет.
	{17, "Гриша", 25, entity.GenderMale, entity.GenderPrefSame, entity.ScheduleLark, entity.SmokingOutside, entity.PetsOwnDog,
		"Сид для разработки: живу только с парнями, собака едет со мной.",
		"d-tverskoi", 42000, 60000, 0, 0, "2026-09-01", ""},
	// Дальше — зоны, которые с Хамовниками не пересекаются: плотность в других
	// пулах и проверка, что граница пула действительно граница.
	{18, "Максим", 26, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: хочу жить у Сокола, оттуда удобно на работу.",
		"st-sokol", 35000, 45000, 0, 0, "2026-09-01", entity.TermYear},
	{19, "Вера", 23, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingHome, entity.PetsOwnCat,
		"Сид для разработки: район знаю, ищу соседку на севере.",
		"d-aeroport", 30000, 40000, 1, 1, "2026-09-01", ""},
	{20, "Оля", 29, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnOther,
		"Сид для разработки: смотрю всю зелёную ветку, срок не принципиален.",
		"line-2", 45000, 65000, 2, 3, "2026-10-01", entity.TermOpen},
}

// The variants the rest of the questionnaire cycles through. The periods are
// deliberately different — 5, 3, 3, 4, 3, 3, 3 — and the indices are shifted,
// so no two answers move together and «N совпадений анкеты» comes out spread
// across the crowd instead of jumping in blocks.
var (
	occupations = []entity.Occupation{
		entity.OccupationStudy, entity.OccupationOffice, entity.OccupationHybrid,
		entity.OccupationRemote, entity.OccupationShift,
	}
	petsOKs   = []entity.PetsOK{entity.PetsOKFine, entity.PetsOKAllergy, entity.PetsOKAgainst}
	noises    = []entity.Noise{entity.NoiseNormal, entity.NoiseSilence, entity.NoiseLoud}
	guests    = []entity.Guests{entity.GuestsRare, entity.GuestsSometimes, entity.GuestsOften, entity.GuestsPartner}
	cleanings = []entity.Cleaning{entity.CleaningSchedule, entity.CleaningWhenDirty, entity.CleaningCreativeMess}
	alcohols  = []entity.Alcohol{entity.AlcoholNone, entity.AlcoholWithNeighbor, entity.AlcoholParties}
	expenses  = []entity.Expenses{entity.ExpensesSplitEqually, entity.ExpensesTracked, entity.ExpensesAsWeGo}
)

// devLikers are the people who like the dev account, by offset in people:
// Катя (n=1) and Ира (n=3). Two of them, so «Тебя лайкнули» is a list rather
// than one card, and Ира comes from a line search — the mini-card of the
// context then has to render a zone nobody's search names literally.
//
// The match itself is deliberately not seeded: answering one of these is the
// manual half of the acceptance walkthrough.
var devLikers = []int64{1, 3}

// The fixed ids of everything this tool writes into points. «5eed» reads as
// «seed» in hex, and the prefix is what the wipe above matches on.
var (
	flatCrowded = uuid.MustParse("5eed0000-0000-4000-8000-000000000001")
	flatFresh   = uuid.MustParse("5eed0000-0000-4000-8000-000000000002")
	flatRedLine = uuid.MustParse("5eed0000-0000-4000-8000-000000000003")
	homeOfKatya = uuid.MustParse("5eed0000-0000-4000-8000-000000000004")
	devFlat     = uuid.MustParse("5eed0000-0000-4000-8000-000000000005")
)

// seedFlat is a market flat: source 'manual' and no created_by, because such a
// point is impersonal — the author is nobody and the card shows no face (§2.2).
type seedFlat struct {
	id      uuid.UUID
	zone    string
	price   int // общая цена; «с человека» карточка считает сама
	rooms   int
	ageDays int
	// pool is who already entered, by seedPerson.n. Nobody with «только свой
	// пол» is here: their membership would be counted for one viewer and not
	// for another, and «в пуле N» is the number this seed is checked by.
	pool []int64
}

// flats are three, and the three differ in both terms of the ranking formula
// (§8: pool size against age), so the order of the feed is something to look
// at rather than something to take on trust.
var flats = []seedFlat{
	// Неделя в ленте и самый людный пул.
	{flatCrowded, "st-frunzenskaya", 165000, 3, 7, []int64{2, 3, 4, 5, 7}},
	// Свежая и почти пустая — и всё равно выше первой.
	{flatFresh, "st-sportivnaya", 135000, 2, 0, []int64{1, 16}},
	// На красной ветке, но вне Хамовников: в выдаче хамовнического поиска её
	// нет, в выдаче Иры (line-1) есть — граница зоны видна глазами.
	{flatRedLine, "st-sokolniki", 128000, 2, 21, []int64{3, 9, 13}},
}

// localHosts is the whole permission of this tool. It writes twenty active
// people with approved photos; against a production database that is not a
// mistake to notice the next day. A unix socket is not listed — any host that
// is a path is one, and a socket cannot reach another machine.
var localHosts = []string{"localhost", "127.0.0.1", "::1", "postgres"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := openLocal(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	photos, err := photostore.NewDir(cfg.PhotoDir, mediaPrefix)
	if err != nil {
		return err
	}
	if err := photos.Put(ctx, avatarKey, bytes.NewReader(avatarPNG()), "image/png"); err != nil {
		return fmt.Errorf("upload the placeholder avatar: %w", err)
	}

	var users, searches, members int
	var hasDev bool
	err = postgres.InTx(ctx, pool, func(tx pgx.Tx) error {
		ids, err := upsertPeople(ctx, tx)
		if err != nil {
			return err
		}
		users = len(ids)

		if searches, err = replaceSearches(ctx, tx, ids); err != nil {
			return err
		}

		byOffset := offsetIndex(ids)
		if members, err = seedFlats(ctx, tx, byOffset); err != nil {
			return err
		}
		if err := seedIntentHome(ctx, tx, byOffset); err != nil {
			return err
		}

		devID, err := findDev(ctx, tx)
		if err != nil || devID == uuid.Nil {
			return err
		}
		hasDev = true

		if err := likeDevUser(ctx, tx, devID); err != nil {
			return err
		}
		return seedDevFlat(ctx, tx, devID)
	})
	if err != nil {
		return err
	}

	fmt.Printf("seeded %d people and %d searches; tg_id %d…%d, photo %s\n",
		users, searches, seedTGBase, seedTGBase+int64(len(people))-1, photos.URL(avatarKey))
	fmt.Printf("seeded %d market flats with %d pool entries, and Катя's «к себе» with one response\n",
		len(flats), members)
	if hasDev {
		fmt.Printf("dev user %d has %d incoming likes from the crowd and one flat of their own on moderation\n",
			devTGID, len(devLikers))
	} else {
		fmt.Printf("dev user %d is not in the database — incoming likes and the pending flat skipped\n", devTGID)
	}
	return nil
}

// upsertPeople writes the crowd and returns their database ids in table order.
// Every column the table owns is listed twice — once in VALUES and once in DO
// UPDATE — so a rerun after an edit of this file actually changes the rows
// instead of quietly keeping the old ones.
func upsertPeople(ctx context.Context, tx pgx.Tx) ([]uuid.UUID, error) {
	const query = `
		INSERT INTO users (tg_id, name, tg_username, age, gender, gender_pref,
			occupation, schedule, smoking, pets_own, pets_ok, noise, guests,
			cleaning, alcohol, expenses, about, photo_key,
			photo_status, about_status, status, onboarding_step, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, 'ok', 'ok', 'active', $19, now())
		ON CONFLICT (tg_id) DO UPDATE SET
			name = EXCLUDED.name, tg_username = EXCLUDED.tg_username,
			age = EXCLUDED.age, gender = EXCLUDED.gender,
			gender_pref = EXCLUDED.gender_pref, occupation = EXCLUDED.occupation,
			schedule = EXCLUDED.schedule, smoking = EXCLUDED.smoking,
			pets_own = EXCLUDED.pets_own, pets_ok = EXCLUDED.pets_ok,
			noise = EXCLUDED.noise, guests = EXCLUDED.guests,
			cleaning = EXCLUDED.cleaning, alcohol = EXCLUDED.alcohol,
			expenses = EXCLUDED.expenses, about = EXCLUDED.about,
			photo_key = EXCLUDED.photo_key, photo_status = 'ok',
			about_status = 'ok', status = 'active',
			onboarding_step = EXCLUDED.onboarding_step, last_seen = now()
		RETURNING id`

	ids := make([]uuid.UUID, 0, len(people))
	for _, p := range people {
		profile := p.profile()

		var id uuid.UUID
		err := tx.QueryRow(ctx, query,
			seedTGBase+p.n-1, p.name, fmt.Sprintf("seed_%02d", p.n),
			profile.Age, string(profile.Gender), string(profile.GenderPref),
			string(profile.Occupation), string(profile.Schedule), string(profile.Smoking),
			string(profile.PetsOwn), string(profile.PetsOK), string(profile.Noise),
			string(profile.Guests), string(profile.Cleaning), string(profile.Alcohol),
			string(profile.Expenses), profile.About, avatarKey, onboardingDone,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert %s (n=%d): %w", p.name, p.n, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// replaceSearches rewrites the subscriptions from scratch. Searches are not
// upserted the way people are: they have no natural key, and editing a row of
// the table would otherwise leave the previous parameters behind as a second
// active search — over the limit of three and in a zone nobody asked for.
func replaceSearches(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (int, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM searches WHERE user_id = ANY($1)`, ids); err != nil {
		return 0, fmt.Errorf("wipe seed searches: %w", err)
	}

	const query = `
		INSERT INTO searches (user_id, zone_id, price_min, price_max,
			rooms_min, rooms_max, move_in, term)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	for i, p := range people {
		moveIn, err := optMonth(p.moveIn)
		if err != nil {
			return 0, fmt.Errorf("search of %s (n=%d): %w", p.name, p.n, err)
		}
		_, err = tx.Exec(ctx, query, ids[i], p.zone,
			optInt(p.priceMin), p.priceMax, optInt(p.roomsMin), optInt(p.roomsMax),
			moveIn, optText(p.term))
		if err != nil {
			return 0, fmt.Errorf("search of %s (n=%d): %w", p.name, p.n, err)
		}
	}
	return len(people), nil
}

// seedFlats writes the market flats and fills their pools, returning how many
// memberships the pools hold in total — the number this run inserted is zero on
// a rerun, and the report has to say the same thing both times.
//
// The points are upserted by their fixed ids: unlike a search, a point cannot
// be wiped and rewritten, because the pool under it references it and entering
// a pool is a thing the dev does by hand during a walkthrough. A rerun states
// the point again rather than patches it — close_reason goes with the status,
// and half of a closed point would break the CHECK that pairs them.
func seedFlats(ctx context.Context, tx pgx.Tx, byOffset map[int64]uuid.UUID) (int, error) {
	const query = `
		INSERT INTO points (id, type, status, source, zone_id, price_total, rooms,
			isolated, photo_key, created_at, edited_at)
		VALUES ($1, 'flat', 'active', 'manual', $2, $3, $4, true, $5,
			now() - make_interval(days => $6::int), now() - make_interval(days => $6::int))
		ON CONFLICT (id) DO UPDATE SET
			status = 'active', close_reason = NULL, zone_id = EXCLUDED.zone_id,
			price_total = EXCLUDED.price_total, rooms = EXCLUDED.rooms,
			isolated = EXCLUDED.isolated, photo_key = EXCLUDED.photo_key,
			created_at = EXCLUDED.created_at, edited_at = EXCLUDED.edited_at`

	members := 0
	for _, f := range flats {
		if _, err := tx.Exec(ctx, query, f.id, f.zone, f.price, f.rooms, avatarKey, f.ageDays); err != nil {
			return 0, fmt.Errorf("upsert flat %s: %w", f.zone, err)
		}
		if err := enterPool(ctx, tx, f.id, offsetIDs(byOffset, f.pool)); err != nil {
			return 0, fmt.Errorf("fill the pool of %s: %w", f.zone, err)
		}
		members += len(f.pool)
	}
	return members, nil
}

// enterPool is the cheap like of §2.2 written directly. The rows are hours
// apart because the pool is ordered by created_at: everything written in one
// transaction shares now(), and with a plain default the order of the cards
// would flip between runs.
func enterPool(ctx context.Context, tx pgx.Tx, pointID uuid.UUID, userIDs []uuid.UUID) error {
	const query = `
		INSERT INTO likes_points (user_id, point_id, created_at)
		SELECT member.id, $1, now() - make_interval(hours => member.ord::int)
		FROM unnest($2::uuid[]) WITH ORDINALITY AS member(id, ord)
		ON CONFLICT DO NOTHING`
	_, err := tx.Exec(ctx, query, pointID, userIDs)
	return err
}

// seedIntentHome gives Катя a room to fill and Олег one response to it. Both
// of them are «с любым полом», so the response is visible from either side
// (§4) — a like across a gender_pref wall would exist in the table and nowhere
// in the product.
//
// There is no pool here: everyone who responds wants the same room, and a
// match between them would be about nothing (§2.2). The response is a spent
// expensive like, exactly what «Хочу эту комнату» sends.
func seedIntentHome(ctx context.Context, tx pgx.Tx, byOffset map[int64]uuid.UUID) error {
	katya, oleg := byOffset[1], byOffset[2]

	const point = `
		INSERT INTO points (id, type, stage, status, source, created_by, zone_id,
			price_share, rooms, isolated, restrictions, photo_key, deposit,
			created_at, edited_at)
		VALUES ($1, 'intent_home', 'live', 'active', 'user_link', $2, $3,
			45000, 2, true, ARRAY['no_smoking']::text[], $4, 45000,
			now() - make_interval(days => 3), now() - make_interval(days => 3))
		ON CONFLICT (id) DO UPDATE SET
			status = 'active', close_reason = NULL, created_by = EXCLUDED.created_by,
			zone_id = EXCLUDED.zone_id, price_share = EXCLUDED.price_share,
			rooms = EXCLUDED.rooms, isolated = EXCLUDED.isolated,
			restrictions = EXCLUDED.restrictions, photo_key = EXCLUDED.photo_key,
			deposit = EXCLUDED.deposit, created_at = EXCLUDED.created_at,
			edited_at = EXCLUDED.edited_at`
	if _, err := tx.Exec(ctx, point, homeOfKatya, katya, "st-sportivnaya", avatarKey); err != nil {
		return fmt.Errorf("upsert the room of Катя: %w", err)
	}

	const response = `
		INSERT INTO likes_users (from_id, to_id, point_id, spent, created_at)
		VALUES ($1, $2, $3, true, now() - make_interval(hours => 5))
		ON CONFLICT (from_id, to_id) DO NOTHING`
	if _, err := tx.Exec(ctx, response, oleg, katya, homeOfKatya); err != nil {
		return fmt.Errorf("respond to the room of Катя: %w", err)
	}
	return nil
}

// findDev looks up the dev account and returns uuid.Nil when it is absent,
// which is the normal state of a fresh database: it appears when the dev logs
// in, and the seeder has no business creating it.
func findDev(ctx context.Context, tx pgx.Tx) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE tg_id = $1`, devTGID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("look up the dev user: %w", err)
	}
	return id, nil
}

// likeDevUser hangs the incoming likes of devLikers on the dev account so that
// «Тебя лайкнули» is not empty without a second Telegram on the other end.
//
// The likes are spent ones, as if sent from the pool: an unspent like is what
// the reply is (§2.3), and the reply is the manual part of the walkthrough.
func likeDevUser(ctx context.Context, tx pgx.Tx, devID uuid.UUID) error {
	tgIDs := make([]int64, len(devLikers))
	for i, n := range devLikers {
		tgIDs[i] = seedTGBase + n - 1
	}

	// The rows are minutes apart on purpose: «Тебя лайкнули» is ordered by
	// created_at, and everything written in one transaction shares now(), so
	// with a plain default the order of the two cards would flip between runs.
	const query = `
		INSERT INTO likes_users (from_id, to_id, zone_id, spent, created_at)
		SELECT u.id, $1, $2, true, now() - make_interval(mins => liker.ord::int)
		FROM unnest($3::bigint[]) WITH ORDINALITY AS liker(tg_id, ord)
		JOIN users u ON u.tg_id = liker.tg_id
		ON CONFLICT (from_id, to_id) DO NOTHING`
	if _, err := tx.Exec(ctx, query, devID, devLikeZone, tgIDs); err != nil {
		return fmt.Errorf("like the dev user: %w", err)
	}
	return nil
}

// seedDevFlat gives the dev account a flat of their own, waiting for a verdict:
// it is what «Присмотренные» and the moderation queue are looked at with.
//
// The creator is put into the pool by hand — the same row the create
// transaction writes, so that «ты первый в пуле» (§6) holds for a seeded point
// too.
func seedDevFlat(ctx context.Context, tx pgx.Tx, devID uuid.UUID) error {
	const query = `
		INSERT INTO points (id, type, status, source, created_by, zone_id,
			price_total, rooms, isolated, photo_key, url_norm, created_at, edited_at)
		VALUES ($1, 'flat', 'pending', 'user_link', $2, 'st-frunzenskaya',
			150000, 2, true, $3, $4,
			now() - make_interval(hours => 2), now() - make_interval(hours => 2))
		ON CONFLICT (id) DO UPDATE SET
			created_by = EXCLUDED.created_by, zone_id = EXCLUDED.zone_id,
			price_total = EXCLUDED.price_total, rooms = EXCLUDED.rooms,
			isolated = EXCLUDED.isolated, photo_key = EXCLUDED.photo_key,
			url_norm = EXCLUDED.url_norm, created_at = EXCLUDED.created_at,
			edited_at = EXCLUDED.edited_at`
	// Уже нормализованная ссылка: так её пишет форма (§6, валидация домена).
	const url = "https://avito.ru/moskva/kvartiry/2-k._kvartira_55m_57et._1234567890"
	if _, err := tx.Exec(ctx, query, devFlat, devID, avatarKey, url); err != nil {
		return fmt.Errorf("upsert the flat of the dev user: %w", err)
	}
	if err := enterPool(ctx, tx, devFlat, []uuid.UUID{devID}); err != nil {
		return fmt.Errorf("put the dev user into their own pool: %w", err)
	}
	return nil
}

// offsetIndex keys the crowd by seedPerson.n, so the tables above can name
// people the way the comments do — «Катя (n=1)» — instead of by row number.
func offsetIndex(ids []uuid.UUID) map[int64]uuid.UUID {
	byOffset := make(map[int64]uuid.UUID, len(people))
	for i, p := range people {
		byOffset[p.n] = ids[i]
	}
	return byOffset
}

func offsetIDs(byOffset map[int64]uuid.UUID, offsets []int64) []uuid.UUID {
	ids := make([]uuid.UUID, len(offsets))
	for i, n := range offsets {
		ids[i] = byOffset[n]
	}
	return ids
}

// profile completes the questionnaire. Every field is filled in: these people
// are active, and an active card with NULL habits reads as a bug rather than
// as a person who skipped a question — the onboarding does not let anyone.
func (p seedPerson) profile() entity.Profile {
	return entity.Profile{
		Age:        p.age,
		Gender:     p.gender,
		GenderPref: p.pref,
		Occupation: pick(occupations, p.n),
		Schedule:   p.schedule,
		Smoking:    p.smoking,
		PetsOwn:    p.petsOwn,
		PetsOK:     pick(petsOKs, p.n),
		Noise:      pick(noises, p.n/2),
		Guests:     pick(guests, p.n),
		Cleaning:   pick(cleanings, p.n+1),
		Alcohol:    pick(alcohols, p.n/3),
		Expenses:   pick(expenses, p.n+2),
		About:      p.about,
	}
}

// openLocal connects and refuses anything that is not this machine or the
// compose service next to it.
func openLocal(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	parsed, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	host := parsed.ConnConfig.Host
	if !strings.HasPrefix(host, "/") && !slices.Contains(localHosts, host) {
		return nil, fmt.Errorf(
			"refusing to seed fake people into %q: devseed only runs against a local database (%v or a unix socket)",
			host, localHosts)
	}
	return postgres.NewPool(ctx, databaseURL)
}

// avatarPNG draws the placeholder every seed person wears — a flat grey
// square. A real face would have to come from somewhere, and a repository is
// the wrong place to keep photographs of people who do not exist.
func avatarPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	grey := color.RGBA{R: 0xB0, G: 0xB6, B: 0xBE, A: 0xFF}
	for y := range avatarSize {
		for x := range avatarSize {
			img.Set(x, y, grey)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		// Encoding a in-memory RGBA image into a bytes.Buffer has no failure
		// mode; if it ever grows one, the seeder is the wrong place to find out.
		panic(fmt.Sprintf("encode the placeholder avatar: %v", err))
	}
	return buf.Bytes()
}

func pick[T ~string](variants []T, i int64) T {
	return variants[int(i)%len(variants)]
}

func optInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func optText[T ~string](v T) *string {
	if v == "" {
		return nil
	}
	s := string(v)
	return &s
}

func optMonth(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	month, err := time.Parse(moveInLayout, raw)
	if err != nil {
		return nil, fmt.Errorf("move-in %q: %w", raw, err)
	}
	return &month, nil
}
