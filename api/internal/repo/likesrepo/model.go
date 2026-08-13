package likesrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const likeColumns = `from_id, to_id, zone_id, point_id, spent, created_at`

const matchColumns = `id, a_id, b_id, context_zone_id, context_point_id, created_at`

const incomingColumns = `l.zone_id, l.point_id, l.created_at,
	NOT EXISTS (SELECT 1 FROM searches s
	            WHERE s.user_id = l.from_id AND s.status = 'active') AS not_searching,
	u.id AS u_id, u.name AS u_name, u.age AS u_age, u.gender AS u_gender,
	u.schedule AS u_schedule, u.smoking AS u_smoking,
	u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
	u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
	u.expenses AS u_expenses, u.photo_key AS u_photo_key,
	u.photo_status AS u_photo_status, u.last_seen AS u_last_seen`
const spentInWindowFilter = `from_id = $1 AND spent AND created_at > now() - make_interval(secs => $2)`

var windowSeconds = entity.LikeWindow.Seconds()

// LikeContext is where the dear like was placed from (§2.3) — a zone pool or a
// point, never both: the service refuses a request that names two contexts.
type LikeContext struct {
	ZoneID string
	// ContextZones — зоны, пересекающие зону контекста: тратящий зонный лайк
	// оплачивается активным поиском в любой из них.
	ContextZones []string
	// PointID переносит отношение на сам контекст — пул квартиры или карточку
	// «к себе», — и поиска тогда не требуется вовсе.
	PointID *uuid.UUID
}

type IncomingLike struct {
	From   entity.User // partial, like entity.PoolCandidate.User
	ZoneID string
	// PointID is the other half of the context: у «к себе» это отклик на твою
	// комнату (§2.2), у квартиры с рынка — общий пул.
	PointID      *uuid.UUID
	NotSearching bool // no active searches — the grey «сейчас не в поиске» mark (§2.3)
	CreatedAt    time.Time
}

// lockedUser is what the pair lock brings back: the id to tell the two rows
// apart and the status the §5 gate reads without a second statement.
type lockedUser struct {
	ID     uuid.UUID
	Status string
}

type likeRow struct {
	FromID    uuid.UUID  `db:"from_id"`
	ToID      uuid.UUID  `db:"to_id"`
	ZoneID    *string    `db:"zone_id"`
	PointID   *uuid.UUID `db:"point_id"`
	Spent     bool       `db:"spent"`
	CreatedAt time.Time  `db:"created_at"`
}

type matchRow struct {
	ID           uuid.UUID  `db:"id"`
	AID          uuid.UUID  `db:"a_id"`
	BID          uuid.UUID  `db:"b_id"`
	ContextZone  *string    `db:"context_zone_id"`
	ContextPoint *uuid.UUID `db:"context_point_id"`
	CreatedAt    time.Time  `db:"created_at"`
}

type incomingRow struct {
	ZoneID       *string    `db:"zone_id"`
	PointID      *uuid.UUID `db:"point_id"`
	NotSearching bool       `db:"not_searching"`
	CreatedAt    time.Time  `db:"created_at"`

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

func (r likeRow) toEntity() entity.Like {
	return entity.Like{
		FromID:    r.FromID,
		ToID:      r.ToID,
		ZoneID:    pointer.Get(r.ZoneID),
		PointID:   r.PointID,
		Spent:     r.Spent,
		CreatedAt: r.CreatedAt,
	}
}

func (r incomingRow) toEntity() IncomingLike {
	return IncomingLike{
		From: entity.User{
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
			// Not read from the row: the query goes through visible_people, which
			// only ever returns active people (§4). Stating the guarantee beats
			// leaving a zero that would make User.Visible() lie.
			Status: entity.StatusActive,
		},
		ZoneID:       pointer.Get(r.ZoneID),
		PointID:      r.PointID,
		NotSearching: r.NotSearching,
		CreatedAt:    r.CreatedAt,
	}
}

func (r matchRow) toEntity() entity.Match {
	return entity.Match{
		ID:           r.ID,
		AID:          r.AID,
		BID:          r.BID,
		ContextZone:  pointer.Get(r.ContextZone),
		ContextPoint: r.ContextPoint,
		CreatedAt:    r.CreatedAt,
	}
}
