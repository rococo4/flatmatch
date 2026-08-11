package entity

import (
	"time"

	"github.com/google/uuid"
)

const MaxDailyLikes = 5

const LikeWindow = 24 * time.Hour

type Like struct {
	FromID, ToID uuid.UUID
	ZoneID       string // "" — no context (a reply)
	PointID      *uuid.UUID
	// Spent is decided at insert time and never recomputed: after a match both
	// sides hold a like, so «was this one a reply?» cannot be answered later.
	Spent     bool
	CreatedAt time.Time
}

// Match keeps the pair ordered — AID < BID — so that a unique index on it
// rejects the second insert when both sides like at the same moment.
type Match struct {
	ID           uuid.UUID
	AID, BID     uuid.UUID
	ContextZone  string
	ContextPoint *uuid.UUID
	CreatedAt    time.Time
}

type LikeBudget struct {
	Remaining  int
	NextFreeAt *time.Time // nil while nothing is spent in the window
}

type LikeResult struct {
	Like    Like
	Match   *Match // non-nil when this like closed the mutual pair
	Already bool   // repeat tap: the like already existed, nothing was spent
	Budget  LikeBudget
}

func (m Match) Peer(me uuid.UUID) uuid.UUID {
	switch me {
	case m.AID:
		return m.BID
	case m.BID:
		return m.AID
	default:
		return uuid.Nil
	}
}
