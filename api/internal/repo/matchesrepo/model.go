package matchesrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const matchColumns = `m.id, m.a_id, m.b_id, m.context_zone_id,
	m.context_point_id, m.created_at`

const peerColumns = `u.id AS u_id, u.name AS u_name, u.age AS u_age,
	u.gender AS u_gender, u.schedule AS u_schedule, u.smoking AS u_smoking,
	u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
	u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
	u.expenses AS u_expenses, u.photo_key AS u_photo_key,
	u.photo_status AS u_photo_status, u.status AS u_status,
	u.last_seen AS u_last_seen`

const contactColumns = `u.tg_username AS u_tg_username, u.tg_id AS u_tg_id`

const blockedColumn = `EXISTS (
	SELECT 1 FROM blocks b
	WHERE (b.blocker_id, b.blocked_id) IN ((m.a_id, m.b_id), (m.b_id, m.a_id))) AS blocked`

type MatchWithPeer struct {
	Match entity.Match
	Peer  entity.User

	Blocked bool
}

type PollTarget struct {
	Match    entity.Match
	UserID   uuid.UUID
	PeerName string
}

type matchRow struct {
	ID           uuid.UUID  `db:"id"`
	AID          uuid.UUID  `db:"a_id"`
	BID          uuid.UUID  `db:"b_id"`
	ContextZone  *string    `db:"context_zone_id"`
	ContextPoint *uuid.UUID `db:"context_point_id"`
	CreatedAt    time.Time  `db:"created_at"`
}

type peerRow struct {
	matchRow

	Blocked bool `db:"blocked"`

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
	UStatus      string    `db:"u_status"`
	ULastSeen    time.Time `db:"u_last_seen"`
}

type pollRow struct {
	matchRow

	UserID   uuid.UUID `db:"user_id"`
	PeerName string    `db:"peer_name"`
}

type contactRow struct {
	peerRow

	UUsername   *string `db:"u_tg_username"`
	UTelegramID int64   `db:"u_tg_id"`
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

func (r peerRow) toEntity() MatchWithPeer {
	return MatchWithPeer{
		Match:   r.matchRow.toEntity(),
		Blocked: r.Blocked,
		Peer: entity.User{
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
			Status:      entity.ParseStatus(r.UStatus),
			LastSeen:    r.ULastSeen,
		},
	}
}

func (r contactRow) toEntity() MatchWithPeer {
	out := r.peerRow.toEntity()
	out.Peer.Username = pointer.Get(r.UUsername)
	out.Peer.TelegramID = r.UTelegramID
	return out
}

func (r pollRow) toEntity() PollTarget {
	return PollTarget{
		Match:    r.matchRow.toEntity(),
		UserID:   r.UserID,
		PeerName: r.PeerName,
	}
}
