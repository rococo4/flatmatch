package pools

import (
	"context"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

const (
	avatarCap      = 4
	noneSectionCap = 20
)

type Repository interface {
	PoolCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]entity.PoolCandidate, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	MarkPoolSeen(ctx context.Context, userID uuid.UUID, zoneID string) error
}

type PhotoURLs interface {
	URL(key string) string
}

type Person struct {
	User          entity.User
	PhotoURL      string
	SharedAnswers int
	Chips         []Chip
	Params        entity.SearchParams
	Zone          zones.Zone
	SearchCreated time.Time
}

type Section struct {
	ID     SectionID
	People []Person
}

type Pool struct {
	Zone     zones.Zone
	MySearch *entity.Search
	Ranked   bool
	Sections []Section // ranked: always 3, in the order all / partial / none
	People   []Person  // unranked: one plain list
}

type Counts struct {
	People    int
	FullMatch int
	NewCount  int
	Avatars   []string
}

type Service struct {
	repo   Repository
	dir    *zones.Directory
	photos PhotoURLs
}

type scored struct {
	cand    entity.PoolCandidate
	chips   []Chip
	section SectionID
}

type bestByUser struct {
	byUser map[uuid.UUID]scored
	order  []uuid.UUID
}
