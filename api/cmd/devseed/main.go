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
	seedTGBase int64 = 900000001

	avatarKey  = "seed/avatar.png"
	avatarSize = 128

	mediaPrefix = "/media"

	onboardingDone = entity.OnboardingSteps - 1

	moveInLayout = "2006-01-02"

	devTGID int64 = 777000001

	devLikeZone = "d-khamovniki"
)

type seedPerson struct {
	n        int64
	name     string
	age      int
	gender   entity.Gender
	pref     entity.GenderPref
	schedule entity.Schedule
	smoking  entity.Smoking
	petsOwn  entity.PetsOwn
	about    string

	zone     string
	priceMin int
	priceMax int
	rooms    []int
	moveIn   string
	term     entity.Term
}

var people = []seedPerson{
	{1, "Катя", 22, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: сплю поздно, работаю из дома, ищу спокойную квартиру.",
		"d-khamovniki", 40000, 55000, nil, "2026-09-01", entity.TermYear},
	{2, "Олег", 25, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: со мной переезжает кот, он тихий.",
		"d-khamovniki", 35000, 45000, []int{2}, "2026-09-01", ""},

	{3, "Ира", 23, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingOutside, entity.PetsOwnNone,
		"Сид для разработки: курю только на балконе, готова смотреть от Сокольников до Юго-Западной.",
		"line-1", 30000, 42000, nil, "2026-10-01", entity.TermHalfYear},

	{4, "Дима", 27, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnDog,
		"Сид для разработки: собака среднего размера, гуляем рано утром.",
		"d-khamovniki", 45000, 60000, nil, "2026-09-01", ""},

	{5, "Настя", 24, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: бюджет выше среднего, зато готова на весь год.",
		"d-khamovniki", 60000, 80000, nil, "2026-09-01", entity.TermYear},

	{6, "Глеб", 30, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingHome, entity.PetsOwnNone,
		"Сид для разработки: заезжаю ближе к зиме, ищу вариант подороже.",
		"d-khamovniki", 70000, 90000, nil, "2026-12-01", ""},

	{7, "Лена", 21, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: сроки плавают, смотрю всё подряд.",
		"d-khamovniki", 0, 50000, nil, "", ""},

	{8, "Аня", 26, entity.GenderFemale, entity.GenderPrefSame, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: съезжаюсь только с девушками.",
		"d-khamovniki", 35000, 45000, nil, "2026-09-01", ""},
	{9, "Марк", 29, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingOutside, entity.PetsOwnDog,
		"Сид для разработки: нужна двушка или трёшка, работаю в офисе рядом с кольцом.",
		"line-5", 38000, 48000, []int{2, 3}, "2026-09-01", entity.TermYear},
	{10, "Соня", 20, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: студентка, заезжаю в августе, если найду с кем.",
		"d-arbat", 33000, 41000, []int{1}, "2026-08-01", entity.TermHalfYear},
	{11, "Тимур", 31, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingHome, entity.PetsOwnNone,
		"Сид для разработки: сменный график, дома бываю днём.",
		"d-yakimanka", 44000, 52000, nil, "2026-10-01", entity.TermOpen},

	{12, "Полина", 24, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnOther,
		"Сид для разработки: бюджет скромный, зато со мной только черепаха.",
		"d-ramenki", 25000, 34000, nil, "2026-09-01", ""},

	{13, "Женя", 28, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingOutside, entity.PetsOwnNone,
		"Сид для разработки: освобождаюсь только к ноябрю, ищу заранее.",
		"d-dorogomilovo", 36000, 46000, []int{2}, "2026-11-01", entity.TermYear},
	{14, "Артём", 33, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnCat,
		"Сид для разработки: готов платить больше за отдельную комнату побольше.",
		"line-3", 50000, 70000, nil, "2026-09-01", entity.TermOpen},

	{15, "Даша", 27, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: ищу трёшку в центре, заезд после Нового года.",
		"d-presnenskii", 80000, 120000, []int{3}, "2027-01-01", entity.TermOpen},
	{16, "Кирилл", 22, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: учусь рядом, с датой заезда пока не определился.",
		"st-park-kultury", 30000, 45000, []int{1, 2}, "", entity.TermHalfYear},

	{17, "Гриша", 25, entity.GenderMale, entity.GenderPrefSame, entity.ScheduleLark, entity.SmokingOutside, entity.PetsOwnDog,
		"Сид для разработки: живу только с парнями, собака едет со мной.",
		"d-tverskoi", 42000, 60000, nil, "2026-09-01", ""},

	{18, "Максим", 26, entity.GenderMale, entity.GenderPrefAny, entity.ScheduleFloating, entity.SmokingNo, entity.PetsOwnNone,
		"Сид для разработки: хочу жить у Сокола, оттуда удобно на работу.",
		"st-sokol", 35000, 45000, nil, "2026-09-01", entity.TermYear},
	{19, "Вера", 23, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleOwl, entity.SmokingHome, entity.PetsOwnCat,
		"Сид для разработки: район знаю, ищу соседку на севере.",
		"d-aeroport", 30000, 40000, []int{1}, "2026-09-01", ""},
	{20, "Оля", 29, entity.GenderFemale, entity.GenderPrefAny, entity.ScheduleLark, entity.SmokingNo, entity.PetsOwnOther,
		"Сид для разработки: смотрю всю зелёную ветку, срок не принципиален.",
		"line-2", 45000, 65000, []int{2, 3}, "2026-10-01", entity.TermOpen},
}

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

var devLikers = []int64{1, 3}

var (
	flatCrowded = uuid.MustParse("5eed0000-0000-4000-8000-000000000001")
	flatFresh   = uuid.MustParse("5eed0000-0000-4000-8000-000000000002")
	flatRedLine = uuid.MustParse("5eed0000-0000-4000-8000-000000000003")
	homeOfKatya = uuid.MustParse("5eed0000-0000-4000-8000-000000000004")
	devFlat     = uuid.MustParse("5eed0000-0000-4000-8000-000000000005")
)

type seedFlat struct {
	id      uuid.UUID
	zone    string
	price   int
	rooms   int
	ageDays int

	pool []int64
}

var flats = []seedFlat{
	{flatCrowded, "st-frunzenskaya", 165000, 3, 7, []int64{2, 3, 4, 5, 7}},

	{flatFresh, "st-sportivnaya", 135000, 2, 0, []int64{1, 16}},

	{flatRedLine, "st-sokolniki", 128000, 2, 21, []int64{3, 9, 13}},
}

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

func upsertPeople(ctx context.Context, tx pgx.Tx) ([]uuid.UUID, error) {
	const query = `
		INSERT INTO users (tg_id, name, tg_username, age, gender, gender_pref,
			occupation, schedule, smoking, pets_own, pets_ok, noise, guests,
			cleaning, alcohol, expenses, about, photo_keys,
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
			photo_keys = EXCLUDED.photo_keys, photo_status = 'ok',
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
			string(profile.Expenses), profile.About, []string{avatarKey}, onboardingDone,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("upsert %s (n=%d): %w", p.name, p.n, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func replaceSearches(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (int, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM searches WHERE user_id = ANY($1)`, ids); err != nil {
		return 0, fmt.Errorf("wipe seed searches: %w", err)
	}

	const query = `
		INSERT INTO searches (user_id, zone_id, price_min, price_max,
			rooms, move_in_months, terms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	for i, p := range people {
		moveIn, err := optMonths(p.moveIn)
		if err != nil {
			return 0, fmt.Errorf("search of %s (n=%d): %w", p.name, p.n, err)
		}
		_, err = tx.Exec(ctx, query, ids[i], p.zone,
			optInt(p.priceMin), p.priceMax, setArg(p.rooms), setArg(moveIn),
			setArg(optTerms(p.term)))
		if err != nil {
			return 0, fmt.Errorf("search of %s (n=%d): %w", p.name, p.n, err)
		}
	}
	return len(people), nil
}

func seedFlats(ctx context.Context, tx pgx.Tx, byOffset map[int64]uuid.UUID) (int, error) {
	const query = `
		INSERT INTO points (id, type, status, source, zone_id, price_total, rooms,
			isolated, photo_keys, created_at, edited_at, last_seen_alive)
		VALUES ($1, 'flat', 'active', 'manual', $2, $3, $4, true, ARRAY[$5],
			now() - make_interval(days => $6::int), now() - make_interval(days => $6::int),
			now() - make_interval(days => $6::int))
		ON CONFLICT (id) DO UPDATE SET
			status = 'active', close_reason = NULL, zone_id = EXCLUDED.zone_id,
			price_total = EXCLUDED.price_total, rooms = EXCLUDED.rooms,
			isolated = EXCLUDED.isolated, photo_keys = EXCLUDED.photo_keys,
			created_at = EXCLUDED.created_at, edited_at = EXCLUDED.edited_at,
			last_seen_alive = EXCLUDED.last_seen_alive, gone_count = 0,
			ping_asked_at = NULL`

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

func enterPool(ctx context.Context, tx pgx.Tx, pointID uuid.UUID, userIDs []uuid.UUID) error {
	const query = `
		INSERT INTO likes_points (user_id, point_id, created_at)
		SELECT member.id, $1, now() - make_interval(hours => member.ord::int)
		FROM unnest($2::uuid[]) WITH ORDINALITY AS member(id, ord)
		ON CONFLICT DO NOTHING`
	_, err := tx.Exec(ctx, query, pointID, userIDs)
	return err
}

func seedIntentHome(ctx context.Context, tx pgx.Tx, byOffset map[int64]uuid.UUID) error {
	katya, oleg := byOffset[1], byOffset[2]

	const point = `
		INSERT INTO points (id, type, stage, status, source, created_by, zone_id,
			price_share, rooms, isolated, restrictions, photo_keys, deposit,
			created_at, edited_at)
		VALUES ($1, 'intent_home', 'live', 'active', 'user_link', $2, $3,
			45000, 2, true, ARRAY['no_smoking']::text[], ARRAY[$4], 45000,
			now() - make_interval(days => 3), now() - make_interval(days => 3))
		ON CONFLICT (id) DO UPDATE SET
			status = 'active', close_reason = NULL, created_by = EXCLUDED.created_by,
			zone_id = EXCLUDED.zone_id, price_share = EXCLUDED.price_share,
			rooms = EXCLUDED.rooms, isolated = EXCLUDED.isolated,
			restrictions = EXCLUDED.restrictions, photo_keys = EXCLUDED.photo_keys,
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

func likeDevUser(ctx context.Context, tx pgx.Tx, devID uuid.UUID) error {
	tgIDs := make([]int64, len(devLikers))
	for i, n := range devLikers {
		tgIDs[i] = seedTGBase + n - 1
	}

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

func seedDevFlat(ctx context.Context, tx pgx.Tx, devID uuid.UUID) error {
	const query = `
		INSERT INTO points (id, type, status, source, created_by, zone_id,
			price_total, rooms, isolated, photo_keys, url_norm, created_at, edited_at)
		VALUES ($1, 'flat', 'pending', 'user_link', $2, 'st-frunzenskaya',
			150000, 2, true, ARRAY[$3], $4,
			now() - make_interval(hours => 2), now() - make_interval(hours => 2))
		ON CONFLICT (id) DO UPDATE SET
			created_by = EXCLUDED.created_by, zone_id = EXCLUDED.zone_id,
			price_total = EXCLUDED.price_total, rooms = EXCLUDED.rooms,
			isolated = EXCLUDED.isolated, photo_keys = EXCLUDED.photo_keys,
			url_norm = EXCLUDED.url_norm, created_at = EXCLUDED.created_at,
			edited_at = EXCLUDED.edited_at`

	const url = "https://avito.ru/moskva/kvartiry/2-k._kvartira_55m_57et._1234567890"
	if _, err := tx.Exec(ctx, query, devFlat, devID, avatarKey, url); err != nil {
		return fmt.Errorf("upsert the flat of the dev user: %w", err)
	}
	if err := enterPool(ctx, tx, devFlat, []uuid.UUID{devID}); err != nil {
		return fmt.Errorf("put the dev user into their own pool: %w", err)
	}
	return nil
}

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

func optMonths(raw string) ([]time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	month, err := time.Parse(moveInLayout, raw)
	if err != nil {
		return nil, fmt.Errorf("move-in %q: %w", raw, err)
	}
	return []time.Time{month}, nil
}

func setArg[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func optTerms(t entity.Term) []string {
	if t == "" {
		return nil
	}
	return []string{string(t)}
}
