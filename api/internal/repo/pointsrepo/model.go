package pointsrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const pointColumns = `id, type, stage, status, source, created_by, zone_id,
	price_total, price_share, rooms, isolated, bathrooms, utilities,
	restrictions, photo_keys, description, url_norm, move_in, deposit, close_reason,
	last_seen_alive, gone_count, ping_asked_at,
	created_at, edited_at`

const joinedPointColumns = `p.id, p.type, p.stage, p.status, p.source, p.created_by,
	p.zone_id, p.price_total, p.price_share, p.rooms, p.isolated, p.bathrooms,
	p.utilities, p.restrictions, p.photo_keys, p.description, p.url_norm, p.move_in, p.deposit,
	p.close_reason, p.last_seen_alive, p.gone_count, p.ping_asked_at,
	p.created_at, p.edited_at`

const personColumns = `u.id AS u_id, u.name AS u_name, u.age AS u_age,
	u.gender AS u_gender, u.occupation AS u_occupation,
	u.schedule AS u_schedule, u.smoking AS u_smoking,
	u.pets_own AS u_pets_own, u.pets_ok AS u_pets_ok, u.noise AS u_noise,
	u.guests AS u_guests, u.cleaning AS u_cleaning, u.alcohol AS u_alcohol,
	u.expenses AS u_expenses, u.photo_keys AS u_photo_keys,
	u.photo_status AS u_photo_status, u.last_seen AS u_last_seen`

const memberColumns = `lp.created_at AS entered_at,
	EXISTS (SELECT 1 FROM matches m
	        WHERE m.a_id = least($1::uuid, lp.user_id)
	          AND m.b_id = greatest($1::uuid, lp.user_id)) AS matched, ` + personColumns + `,
	s.id AS s_id, s.zone_id AS s_zone_id, s.price_min AS s_price_min,
	s.price_max AS s_price_max, s.rooms AS s_rooms, s.terms AS s_terms,
	s.move_in_months AS s_move_in_months,
	s.has_stuff AS s_has_stuff, s.status AS s_status,
	s.pool_seen_at AS s_pool_seen_at, s.created_at AS s_created_at,
	s.updated_at AS s_updated_at`

const livePoint = `status IN ('pending', 'active', 'paused')`

const uniqueViolation = "23505"

const urlNormIndex = "points_url_norm_active"

type FeedPoint struct {
	Point     entity.Point
	PoolCount int
	Responses int
	InPool    bool
	Author    *entity.User
	GoneVotes int
	MyVote    *bool
}

type Member struct {
	User      entity.User
	Search    *entity.Search
	EnteredAt time.Time
	Matched   bool
}

type VoteOutcome struct {
	GoneVotes int
	Hidden    bool
	Closed    bool
}

type pointRow struct {
	ID           uuid.UUID  `db:"id"`
	Type         string     `db:"type"`
	Stage        *string    `db:"stage"`
	Status       string     `db:"status"`
	Source       string     `db:"source"`
	CreatedBy    *uuid.UUID `db:"created_by"`
	ZoneID       string     `db:"zone_id"`
	PriceTotal   *int       `db:"price_total"`
	PriceShare   *int       `db:"price_share"`
	Rooms        *int       `db:"rooms"`
	Isolated     *bool      `db:"isolated"`
	Bathrooms    *string    `db:"bathrooms"`
	Utilities    *string    `db:"utilities"`
	Restrictions []string   `db:"restrictions"`
	PhotoKeys    []string   `db:"photo_keys"`
	Description  *string    `db:"description"`
	URLNorm      *string    `db:"url_norm"`
	MoveIn       *time.Time `db:"move_in"`
	Deposit      *int       `db:"deposit"`
	CloseReason  *string    `db:"close_reason"`

	LastSeenAlive time.Time  `db:"last_seen_alive"`
	GoneCount     int        `db:"gone_count"`
	PingAskedAt   *time.Time `db:"ping_asked_at"`

	CreatedAt time.Time `db:"created_at"`
	EditedAt  time.Time `db:"edited_at"`
}

type personRow struct {
	UID          *uuid.UUID `db:"u_id"`
	UName        *string    `db:"u_name"`
	UAge         *int       `db:"u_age"`
	UGender      *string    `db:"u_gender"`
	UOccupation  *string    `db:"u_occupation"`
	USchedule    *string    `db:"u_schedule"`
	USmoking     *string    `db:"u_smoking"`
	UPetsOwn     *string    `db:"u_pets_own"`
	UPetsOK      *string    `db:"u_pets_ok"`
	UNoise       *string    `db:"u_noise"`
	UGuests      *string    `db:"u_guests"`
	UCleaning    *string    `db:"u_cleaning"`
	UAlcohol     *string    `db:"u_alcohol"`
	UExpenses    *string    `db:"u_expenses"`
	UPhotoKeys   []string   `db:"u_photo_keys"`
	UPhotoStatus *string    `db:"u_photo_status"`
	ULastSeen    *time.Time `db:"u_last_seen"`
}

type feedRow struct {
	pointRow
	personRow

	PoolCount int   `db:"pool_count"`
	Responses int   `db:"responses"`
	InPool    bool  `db:"in_pool"`
	GoneVotes int   `db:"gone_votes"`
	MyVote    *bool `db:"my_vote"`
}

type memberRow struct {
	personRow

	EnteredAt time.Time `db:"entered_at"`
	Matched   bool      `db:"matched"`

	SID           *uuid.UUID  `db:"s_id"`
	SZoneID       *string     `db:"s_zone_id"`
	SPriceMin     *int        `db:"s_price_min"`
	SPriceMax     *int        `db:"s_price_max"`
	SRooms        []int32     `db:"s_rooms"`
	STerms        []string    `db:"s_terms"`
	SMoveInMonths []time.Time `db:"s_move_in_months"`
	SHasStuff     []string    `db:"s_has_stuff"`
	SStatus       *string     `db:"s_status"`
	SPoolSeenAt   *time.Time  `db:"s_pool_seen_at"`
	SCreatedAt    *time.Time  `db:"s_created_at"`
	SUpdatedAt    *time.Time  `db:"s_updated_at"`
}

func (r pointRow) toEntity() entity.Point {
	p := entity.Point{
		ID:          r.ID,
		Type:        entity.ParsePointType(r.Type),
		Status:      entity.ParsePointStatus(r.Status),
		Source:      entity.ParsePointSource(r.Source),
		CreatedBy:   r.CreatedBy,
		ZoneID:      r.ZoneID,
		PriceTotal:  r.PriceTotal,
		PriceShare:  r.PriceShare,
		Rooms:       r.Rooms,
		Isolated:    r.Isolated,
		PhotoKeys:   r.PhotoKeys,
		Description: r.Description,
		URLNorm:     r.URLNorm,
		MoveIn:      r.MoveIn,
		Deposit:     r.Deposit,
		CloseReason: r.CloseReason,

		LastSeenAlive: r.LastSeenAlive,
		GoneCount:     r.GoneCount,
		PingAskedAt:   r.PingAskedAt,

		CreatedAt: r.CreatedAt,
		EditedAt:  r.EditedAt,
	}
	if r.Stage != nil {
		p.Stage = pointer.To(entity.ParsePointStage(*r.Stage))
	}
	if r.Bathrooms != nil {
		p.Bathrooms = pointer.To(entity.ParseBathrooms(*r.Bathrooms))
	}
	if r.Utilities != nil {
		p.Utilities = pointer.To(entity.ParseUtilities(*r.Utilities))
	}
	for _, code := range r.Restrictions {
		p.Restrictions = append(p.Restrictions, entity.ParseRestriction(code))
	}
	return p
}

func (r personRow) toEntity() *entity.User {
	if r.UID == nil {
		return nil
	}
	return &entity.User{
		ID:   *r.UID,
		Name: pointer.Get(r.UName),
		Profile: entity.Profile{
			Age:    pointer.Get(r.UAge),
			Gender: entity.ParseGender(pointer.Get(r.UGender)),

			Occupation: entity.ParseOccupation(pointer.Get(r.UOccupation)),
			Schedule:   entity.ParseSchedule(pointer.Get(r.USchedule)),
			Smoking:    entity.ParseSmoking(pointer.Get(r.USmoking)),
			PetsOwn:    entity.ParsePetsOwn(pointer.Get(r.UPetsOwn)),
			PetsOK:     entity.ParsePetsOK(pointer.Get(r.UPetsOK)),
			Noise:      entity.ParseNoise(pointer.Get(r.UNoise)),
			Guests:     entity.ParseGuests(pointer.Get(r.UGuests)),
			Cleaning:   entity.ParseCleaning(pointer.Get(r.UCleaning)),
			Alcohol:    entity.ParseAlcohol(pointer.Get(r.UAlcohol)),
			Expenses:   entity.ParseExpenses(pointer.Get(r.UExpenses)),
		},
		PhotoKeys:   r.UPhotoKeys,
		PhotoStatus: entity.ParseModerationStatus(pointer.Get(r.UPhotoStatus)),
		LastSeen:    pointer.Get(r.ULastSeen),

		Status: entity.StatusActive,
	}
}

func (r feedRow) toEntity() FeedPoint {
	return FeedPoint{
		Point:     r.pointRow.toEntity(),
		PoolCount: r.PoolCount,
		Responses: r.Responses,
		InPool:    r.InPool,
		Author:    r.personRow.toEntity(),
		GoneVotes: r.GoneVotes,
		MyVote:    r.MyVote,
	}
}

func (r memberRow) toEntity() Member {
	user := r.personRow.toEntity()
	return Member{User: *user, Search: r.toSearch(user.ID), EnteredAt: r.EnteredAt, Matched: r.Matched}
}

func (r memberRow) toSearch(userID uuid.UUID) *entity.Search {
	if r.SID == nil {
		return nil
	}
	s := entity.Search{
		ID:     *r.SID,
		UserID: userID,
		SearchParams: entity.SearchParams{
			ZoneID:       pointer.Get(r.SZoneID),
			PriceMin:     r.SPriceMin,
			PriceMax:     pointer.Get(r.SPriceMax),
			Rooms:        roomsEntity(r.SRooms),
			MoveInMonths: r.SMoveInMonths,
		},
		Status:     entity.ParseSearchStatus(pointer.Get(r.SStatus)),
		PoolSeenAt: pointer.Get(r.SPoolSeenAt),
		CreatedAt:  pointer.Get(r.SCreatedAt),
		UpdatedAt:  pointer.Get(r.SUpdatedAt),
	}
	for _, t := range r.STerms {
		s.Terms = append(s.Terms, entity.ParseTerm(t))
	}
	for _, c := range r.SHasStuff {
		s.HasStuff = append(s.HasStuff, entity.ParseStuffChip(c))
	}
	return &s
}

func enumArg[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	return pointer.To(string(*v))
}

func restrictionsArg(codes []entity.Restriction) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		out = append(out, string(c))
	}
	return out
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
