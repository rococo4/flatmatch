package entity

import (
	"time"

	"github.com/google/uuid"
)

const MaxDailyLikes = 5

const LikeWindow = 24 * time.Hour

type Like struct {
	FromID, ToID uuid.UUID
	ZoneID       string
	PointID      *uuid.UUID

	Spent     bool
	CreatedAt time.Time
}

type Match struct {
	ID           uuid.UUID
	AID, BID     uuid.UUID
	ContextZone  string
	ContextPoint *uuid.UUID
	CreatedAt    time.Time
}

type LikeBudget struct {
	Remaining  int
	NextFreeAt *time.Time
}

type LikeResult struct {
	Like    Like
	Match   *Match
	Already bool
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
