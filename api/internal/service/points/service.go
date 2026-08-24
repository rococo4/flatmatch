package points

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/imagetype"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

const (
	MinPrice = 20_000
	MaxPrice = 300_000
)

var ErrInvalidForm = errors.New("point form has invalid fields")

var ErrFlatsOff = errors.New("market flats are switched off")

type ErrDuplicateURL struct {
	Existing entity.Point

	PoolCount int
}

type Repository interface {
	Create(ctx context.Context, p entity.Point) (entity.Point, error)
	Close(ctx context.Context, userID, pointID uuid.UUID) error
	ByURLNorm(ctx context.Context, urlNorm string) (*entity.Point, error)
	CardByID(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Feed(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]pointsrepo.FeedPoint, error)
	Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error)
	PoolMembers(ctx context.Context, viewerID, pointID uuid.UUID) ([]pointsrepo.Member, error)
	CountActiveIn(ctx context.Context, zoneIDs []string) (int, error)
	Enter(ctx context.Context, userID, pointID uuid.UUID) (entered bool, err error)
	Leave(ctx context.Context, userID, pointID uuid.UUID) error
	InPool(ctx context.Context, userID, pointID uuid.UUID) (bool, error)
	Vote(ctx context.Context, userID, pointID uuid.UUID, alive bool) (pointsrepo.VoteOutcome, error)
	Restore(ctx context.Context, userID, pointID uuid.UUID) error
}

type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
}

type Notifier interface {
	PointPending(ctx context.Context, p entity.Point)
}

type Pushes interface {
	PoolNew(ctx context.Context, pointID, enteredUserID uuid.UUID)
	PointGone(ctx context.Context, point entity.Point, lastVoter uuid.UUID)
}

type PhotoStore interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string) error
}

type CreateFlat struct {
	URL          string
	ZoneID       string
	PriceTotal   int
	Rooms        *int
	Isolated     *bool
	Bathrooms    *entity.Bathrooms
	Utilities    *entity.Utilities
	Restrictions []entity.Restriction
	PhotoKeys    []string
	Deposit      *int
}

type CreateIntentHome struct {
	Stage        entity.PointStage
	ZoneID       string
	PriceShare   int
	Rooms        *int
	Isolated     *bool
	Restrictions []entity.Restriction
	PhotoKeys    []string
	Description  *string
	MoveIn       *time.Time
	Deposit      *int
}

type MemberView struct {
	pointsrepo.Member

	Chips         []pools.Chip
	SharedAnswers int
}

type PoolView struct {
	Members []MemberView
}

type Service struct {
	repo     Repository
	searches Searches
	photos   PhotoStore
	dir      *zones.Directory
	notify   Notifier
	pushes   Pushes
}

func New(repo Repository, searches Searches, photos PhotoStore, dir *zones.Directory,
	notify Notifier, pushes Pushes) *Service {
	return &Service{repo: repo, searches: searches, photos: photos, dir: dir, notify: notify, pushes: pushes}
}

func (e *ErrDuplicateURL) Error() string {
	return fmt.Sprintf("url already belongs to point %s", e.Existing.ID)
}

func (s *Service) CreateFlat(context.Context, uuid.UUID, CreateFlat) (entity.Point, error) {
	return entity.Point{}, ErrFlatsOff
}

func (s *Service) CreateIntentHome(ctx context.Context, userID uuid.UUID, in CreateIntentHome) (entity.Point, error) {
	p, err := s.validIntentHome(in)
	if err != nil {
		return entity.Point{}, err
	}
	p.CreatedBy = &userID
	return s.create(ctx, p)
}

func (s *Service) Feed(ctx context.Context, viewerID uuid.UUID, zoneID string) ([]pointsrepo.FeedPoint, error) {
	if _, ok := s.dir.Get(zoneID); !ok {
		return nil, entity.ErrNotFound
	}
	return s.repo.Feed(ctx, viewerID, s.dir.Intersecting(zoneID))
}

func (s *Service) Point(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error) {
	card, err := s.repo.CardByID(ctx, viewerID, pointID)
	if err != nil {
		return pointsrepo.FeedPoint{}, err
	}

	if card.Point.Status != entity.PointActive && card.Point.Status != entity.PointPaused &&
		(card.Point.CreatedBy == nil || *card.Point.CreatedBy != viewerID) {
		return pointsrepo.FeedPoint{}, entity.ErrNotFound
	}
	return card, nil
}

func (s *Service) SavePhoto(ctx context.Context, file io.ReadSeeker) (string, error) {
	contentType, ext, err := imagetype.Sniff(file)
	if err != nil {
		return "", fmt.Errorf("point photo: %w", err)
	}

	key := "points/" + uuid.NewString() + ext
	if err := s.photos.Put(ctx, key, file, contentType); err != nil {
		return "", fmt.Errorf("store point photo: %w", err)
	}
	return key, nil
}

func (s *Service) Enter(ctx context.Context, userID, pointID uuid.UUID) error {
	entered, err := s.repo.Enter(ctx, userID, pointID)
	if err != nil {
		return err
	}

	if entered && s.pushes != nil {
		s.pushes.PoolNew(ctx, pointID, userID)
	}
	return nil
}

func (s *Service) Close(ctx context.Context, userID, pointID uuid.UUID) error {
	return s.repo.Close(ctx, userID, pointID)
}

func (s *Service) Leave(ctx context.Context, userID, pointID uuid.UUID) error {
	return s.repo.Leave(ctx, userID, pointID)
}

func (s *Service) Vote(ctx context.Context, viewerID, pointID uuid.UUID, alive bool) (pointsrepo.FeedPoint, error) {
	card, err := s.Point(ctx, viewerID, pointID)
	if err != nil {
		return pointsrepo.FeedPoint{}, err
	}
	if !card.Point.Votable() {
		return pointsrepo.FeedPoint{}, entity.ErrNotVotable
	}
	if card.Point.Status != entity.PointActive {
		return pointsrepo.FeedPoint{}, entity.ErrPointNotActive
	}

	out, err := s.repo.Vote(ctx, viewerID, pointID, alive)
	if err != nil {
		return pointsrepo.FeedPoint{}, err
	}

	card, err = s.repo.CardByID(ctx, viewerID, pointID)
	if err != nil {
		return pointsrepo.FeedPoint{}, err
	}

	if out.Hidden && s.pushes != nil {
		s.pushes.PointGone(ctx, card.Point, viewerID)
	}
	return card, nil
}

func (s *Service) Restore(ctx context.Context, userID, pointID uuid.UUID) (pointsrepo.FeedPoint, error) {
	if err := s.repo.Restore(ctx, userID, pointID); err != nil {
		return pointsrepo.FeedPoint{}, err
	}
	return s.repo.CardByID(ctx, userID, pointID)
}

func (s *Service) Pool(ctx context.Context, viewer entity.User, pointID uuid.UUID) (PoolView, error) {
	card, err := s.Point(ctx, viewer.ID, pointID)
	if err != nil {
		return PoolView{}, err
	}
	in, err := s.repo.InPool(ctx, viewer.ID, pointID)
	if err != nil {
		return PoolView{}, err
	}
	if !in {
		return PoolView{}, entity.ErrNeedEnter
	}

	members, err := s.repo.PoolMembers(ctx, viewer.ID, pointID)
	if err != nil {
		return PoolView{}, err
	}
	mine, err := s.searchNear(ctx, viewer.ID, card.Point.ZoneID)
	if err != nil {
		return PoolView{}, err
	}

	view := PoolView{Members: make([]MemberView, 0, len(members))}
	for _, m := range members {
		mv := MemberView{Member: m, SharedAnswers: viewer.Profile.SharedAnswers(m.User.Profile)}
		if mine != nil && m.Search != nil {
			mv.Chips, _ = pools.Compare(mine.SearchParams, m.Search.SearchParams)
		}
		view.Members = append(view.Members, mv)
	}
	return view, nil
}

func (s *Service) Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error) {
	return s.repo.Mine(ctx, viewerID)
}

func (s *Service) CountIn(ctx context.Context, zoneID string) (int, error) {
	zoneIDs := s.dir.Intersecting(zoneID)
	if len(zoneIDs) == 0 {
		return 0, nil
	}
	return s.repo.CountActiveIn(ctx, zoneIDs)
}

func (s *Service) create(ctx context.Context, p entity.Point) (entity.Point, error) {
	if p.URLNorm != nil {
		existing, err := s.repo.ByURLNorm(ctx, *p.URLNorm)
		if err != nil {
			return entity.Point{}, err
		}
		if existing != nil {
			return entity.Point{}, s.duplicate(ctx, p, *existing)
		}
	}

	created, err := s.repo.Create(ctx, p)

	if errors.Is(err, pointsrepo.ErrURLTaken) && p.URLNorm != nil {
		if existing, e := s.repo.ByURLNorm(ctx, *p.URLNorm); e == nil && existing != nil {
			return entity.Point{}, s.duplicate(ctx, p, *existing)
		}
	}
	if err != nil {
		return entity.Point{}, err
	}

	if s.notify != nil {
		s.notify.PointPending(context.WithoutCancel(ctx), created)
	}
	return created, nil
}

func (s *Service) duplicate(ctx context.Context, mine entity.Point, existing entity.Point) error {
	dup := &ErrDuplicateURL{Existing: existing}
	if existing.Status != entity.PointActive {
		return dup
	}
	if card, err := s.repo.CardByID(ctx, pointer.Get(mine.CreatedBy), existing.ID); err == nil {
		dup.PoolCount = card.PoolCount
	}
	return dup
}

func (s *Service) searchNear(ctx context.Context, viewerID uuid.UUID, zoneID string) (*entity.Search, error) {
	mine, err := s.searches.ListByUser(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	intersecting := s.dir.Intersecting(zoneID)
	var best *entity.Search
	for i, m := range mine {
		if m.Status != entity.SearchActive || !slices.Contains(intersecting, m.ZoneID) {
			continue
		}
		if best == nil || m.CreatedAt.After(best.CreatedAt) {
			best = &mine[i]
		}
	}
	return best, nil
}
