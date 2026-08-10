// Command devseed fills a local database with fake people so that zone pools,
// their three sections and the verdict chips have something to show (§2.1).
// Without it every pool is empty and the feed's promo block counts nobody.
//
// The crowd is deterministic: fixed Telegram ids from seedTGBase up, upserted
// by tg_id, searches wiped and re-inserted. Reruns converge to the same state
// instead of piling up copies, and one predicate removes everything:
//
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

	var users, searches int
	err = postgres.InTx(ctx, pool, func(tx pgx.Tx) error {
		ids, err := upsertPeople(ctx, tx)
		if err != nil {
			return err
		}
		users = len(ids)

		searches, err = replaceSearches(ctx, tx, ids)
		return err
	})
	if err != nil {
		return err
	}

	fmt.Printf("seeded %d people and %d searches; tg_id %d…%d, photo %s\n",
		users, searches, seedTGBase, seedTGBase+int64(len(people))-1, photos.URL(avatarKey))
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
