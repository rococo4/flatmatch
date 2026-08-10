package searchesrepo

import (
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const searchColumns = `id, user_id, zone_id, price_min, price_max, rooms_min,
	rooms_max, move_in, term, has_stuff, status, close_reason, pool_seen_at,
	created_at, updated_at`

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
