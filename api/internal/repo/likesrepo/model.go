package likesrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const likeColumns = `from_id, to_id, zone_id, point_id, spent, created_at`

const matchColumns = `id, a_id, b_id, context_zone_id, context_point_id, created_at`

const spentInWindowFilter = `from_id = $1 AND spent AND created_at > now() - make_interval(secs => $2)`

var windowSeconds = entity.LikeWindow.Seconds()

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
