package deck

import (
	"context"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

const passWindow = 7 * 24 * time.Hour

const cardCap = 50

type Kind string

const (
	KindPerson Kind = "person"
	KindRoom   Kind = "room"
)

type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	DeckCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
		f entity.PeopleFilter, within time.Duration) ([]searchesrepo.DeckCandidate, error)
	ActiveByUsers(ctx context.Context, userIDs []uuid.UUID) ([]entity.Search, error)
}

type Points interface {
	DeckRooms(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
		f entity.PeopleFilter, within time.Duration) ([]pointsrepo.FeedPoint, error)
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
}

type Passes interface {
	Upsert(ctx context.Context, userID, targetID uuid.UUID) error
	ActiveTargets(ctx context.Context, userID uuid.UUID, within time.Duration) ([]uuid.UUID, error)
	Clear(ctx context.Context, userID uuid.UUID) (int, error)
}

type People interface {
	VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error)
}

type Matches interface {
	IsMatched(ctx context.Context, a, b uuid.UUID) (bool, error)
}

type Likes interface {
	Between(ctx context.Context, a, b uuid.UUID) (aLikedB, bLikedA bool, err error)
}

type Query struct {
	People bool
	Rooms  bool
	Filter entity.PeopleFilter
}

type Pair struct {
	Zone      zones.Zone
	ZoneMatch bool
	Section   pools.SectionID
	Chips     []pools.Chip
	Params    *entity.SearchParams
}

type Card struct {
	Kind   Kind
	Person entity.User
	Room   pointsrepo.FeedPoint
	Pair   Pair

	activity time.Time
}

type Deck struct {
	Cards    []Card
	Total    int
	NoSearch bool
}

type RankedLike struct {
	Like likesrepo.IncomingLike
	Pair *Pair
}

type Service struct {
	searches Searches
	points   Points
	passes   Passes
	people   People
	matches  Matches
	likes    Likes
	dir      *zones.Directory
}

func (c Card) key() uuid.UUID {
	if c.Kind == KindRoom {
		return c.Room.Point.ID
	}
	return c.Person.ID
}
