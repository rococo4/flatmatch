package searchesrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const searchColumns = `id, user_id, zone_id, price_min, price_max, rooms_min,
	rooms_max, move_in, term, has_stuff, status, close_reason, pool_seen_at,
	created_at, updated_at`

const candidateColumns = `s.id, s.user_id, s.zone_id, s.price_min, s.price_max,
	s.rooms_min, s.rooms_max, s.move_in, s.term, s.has_stuff, s.status,
	s.close_reason, s.pool_seen_at, s.created_at, s.updated_at,
	u.id AS u_id, u.name AS u_name, u.age AS u_age, u.gender AS u_gender,
	u.schedule AS u_schedule, u.smoking AS u_smoking,
	u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
	u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
	u.expenses AS u_expenses, u.photo_key AS u_photo_key,
	u.photo_status AS u_photo_status, u.last_seen AS u_last_seen`

type searchRow struct {
	ID          uuid.UUID  `db:"id"`
	UserID      uuid.UUID  `db:"user_id"`
	ZoneID      string     `db:"zone_id"`
	PriceMin    *int       `db:"price_min"`
	PriceMax    int        `db:"price_max"`
	RoomsMin    *int       `db:"rooms_min"`
	RoomsMax    *int       `db:"rooms_max"`
	MoveIn      *time.Time `db:"move_in"`
	Term        *string    `db:"term"`
	HasStuff    []string   `db:"has_stuff"`
	Status      string     `db:"status"`
	CloseReason *string    `db:"close_reason"`
	PoolSeenAt  time.Time  `db:"pool_seen_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// candidateRow carries exactly what a pool card needs, not a whole user: the
// questionnaire fields behind «N совпадений анкеты» and the привычки-строка,
// plus the photo and «был сегодня». Fields the row leaves zero — GenderPref,
// Username, TelegramID — are NOT unknown data to be re-derived: the query has
// already resolved visibility through visible_people (§4), and re-checking it
// downstream is exactly the duplication that function exists to prevent.
// Status is the exception: it is not selected either, but it is filled in from
// what visible_people guarantees rather than left zero (see toEntity).
type candidateRow struct {
	searchRow // s.* columns, unprefixed

	UID          uuid.UUID `db:"u_id"`
	UName        string    `db:"u_name"`
	UAge         *int      `db:"u_age"`
	UGender      *string   `db:"u_gender"`
	USchedule    *string   `db:"u_schedule"`
	USmoking     *string   `db:"u_smoking"`
	UPetsOwn     *string   `db:"u_pets_own"`
	UPetsOK      *string   `db:"u_pets_ok"`
	UNoise       *string   `db:"u_noise"`
	UGuests      *string   `db:"u_guests"`
	UCleaning    *string   `db:"u_cleaning"`
	UAlcohol     *string   `db:"u_alcohol"`
	UExpenses    *string   `db:"u_expenses"`
	UPhotoKey    *string   `db:"u_photo_key"`
	UPhotoStatus string    `db:"u_photo_status"`
	ULastSeen    time.Time `db:"u_last_seen"`
}

func (r searchRow) toEntity() entity.Search {
	s := entity.Search{
		ID:     r.ID,
		UserID: r.UserID,
		SearchParams: entity.SearchParams{
			ZoneID:   r.ZoneID,
			PriceMin: r.PriceMin,
			PriceMax: r.PriceMax,
			RoomsMin: r.RoomsMin,
			RoomsMax: r.RoomsMax,
			MoveIn:   r.MoveIn,
		},
		Status:     entity.ParseSearchStatus(r.Status),
		PoolSeenAt: r.PoolSeenAt,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
	if r.Term != nil {
		term := entity.ParseTerm(*r.Term)
		s.Term = &term
	}
	for _, c := range r.HasStuff {
		s.HasStuff = append(s.HasStuff, entity.ParseStuffChip(c))
	}
	if r.CloseReason != nil {
		s.CloseReason = entity.ParseCloseReason(*r.CloseReason)
	}
	return s
}

func (r candidateRow) toEntity() entity.PoolCandidate {
	return entity.PoolCandidate{
		User: entity.User{
			ID:   r.UID,
			Name: r.UName,
			Profile: entity.Profile{
				Age:      pointer.Get(r.UAge),
				Gender:   entity.ParseGender(pointer.Get(r.UGender)),
				Schedule: entity.ParseSchedule(pointer.Get(r.USchedule)),
				Smoking:  entity.ParseSmoking(pointer.Get(r.USmoking)),
				PetsOwn:  entity.ParsePetsOwn(pointer.Get(r.UPetsOwn)),
				PetsOK:   entity.ParsePetsOK(pointer.Get(r.UPetsOK)),
				Noise:    entity.ParseNoise(pointer.Get(r.UNoise)),
				Guests:   entity.ParseGuests(pointer.Get(r.UGuests)),
				Cleaning: entity.ParseCleaning(pointer.Get(r.UCleaning)),
				Alcohol:  entity.ParseAlcohol(pointer.Get(r.UAlcohol)),
				Expenses: entity.ParseExpenses(pointer.Get(r.UExpenses)),
			},
			PhotoKey:    pointer.Get(r.UPhotoKey),
			PhotoStatus: entity.ParseModerationStatus(r.UPhotoStatus),
			LastSeen:    r.ULastSeen,
			// Not read from the row: visible_people only ever returns active
			// people (§4), so this states that guarantee instead of leaving a
			// zero behind, which would make User.Visible() lie about a person
			// the pool has already decided is visible.
			Status: entity.StatusActive,
		},
		Search: r.searchRow.toEntity(),
	}
}
