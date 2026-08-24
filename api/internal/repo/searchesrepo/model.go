package searchesrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const searchColumns = `id, user_id, zone_id, price_min, price_max, rooms, terms,
	move_in_months, has_stuff, status, close_reason, pool_seen_at,
	last_active_at, ttl_asked_at, created_at, updated_at`

const candidateColumns = `s.id, s.user_id, s.zone_id, s.price_min, s.price_max,
	s.rooms, s.terms, s.move_in_months, s.has_stuff, s.status,
	s.close_reason, s.pool_seen_at, s.last_active_at, s.ttl_asked_at,
	s.created_at, s.updated_at,
	u.id AS u_id, u.name AS u_name, u.age AS u_age, u.gender AS u_gender,
	u.schedule AS u_schedule, u.smoking AS u_smoking,
	u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
	u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
	u.expenses AS u_expenses, u.photo_keys AS u_photo_keys,
	u.photo_status AS u_photo_status, u.last_seen AS u_last_seen`

type ZoneUsers struct {
	ZoneID  string
	UserIDs []uuid.UUID
}

type searchRow struct {
	ID           uuid.UUID   `db:"id"`
	UserID       uuid.UUID   `db:"user_id"`
	ZoneID       string      `db:"zone_id"`
	PriceMin     *int        `db:"price_min"`
	PriceMax     int         `db:"price_max"`
	Rooms        []int32     `db:"rooms"`
	Terms        []string    `db:"terms"`
	MoveInMonths []time.Time `db:"move_in_months"`
	HasStuff     []string    `db:"has_stuff"`
	Status       string      `db:"status"`
	CloseReason  *string     `db:"close_reason"`
	PoolSeenAt   time.Time   `db:"pool_seen_at"`
	LastActiveAt time.Time   `db:"last_active_at"`
	TTLAskedAt   *time.Time  `db:"ttl_asked_at"`
	CreatedAt    time.Time   `db:"created_at"`
	UpdatedAt    time.Time   `db:"updated_at"`
}

type candidateRow struct {
	searchRow

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
	UPhotoKeys   []string  `db:"u_photo_keys"`
	UPhotoStatus string    `db:"u_photo_status"`
	ULastSeen    time.Time `db:"u_last_seen"`
}

type zoneUsersRow struct {
	ZoneID  string      `db:"zone_id"`
	UserIDs []uuid.UUID `db:"user_ids"`
}

func (r searchRow) toEntity() entity.Search {
	s := entity.Search{
		ID:     r.ID,
		UserID: r.UserID,
		SearchParams: entity.SearchParams{
			ZoneID:       r.ZoneID,
			PriceMin:     r.PriceMin,
			PriceMax:     r.PriceMax,
			Rooms:        roomsEntity(r.Rooms),
			MoveInMonths: r.MoveInMonths,
		},
		Status:       entity.ParseSearchStatus(r.Status),
		PoolSeenAt:   r.PoolSeenAt,
		LastActiveAt: r.LastActiveAt,
		TTLAskedAt:   r.TTLAskedAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
	for _, t := range r.Terms {
		s.Terms = append(s.Terms, entity.ParseTerm(t))
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
			PhotoKeys:   r.UPhotoKeys,
			PhotoStatus: entity.ParseModerationStatus(r.UPhotoStatus),
			LastSeen:    r.ULastSeen,

			Status: entity.StatusActive,
		},
		Search: r.searchRow.toEntity(),
	}
}

func roomsEntity(raw []int32) []int {
	if raw == nil {
		return nil
	}
	out := make([]int, 0, len(raw))
	for _, n := range raw {
		out = append(out, int(n))
	}
	return out
}
