package points

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/pointsrepo"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

// Автовалидация до модератора (§5): вилка отсекает опечатку в нуле раньше, чем
// её увидит человек.
const (
	MinPrice = 20_000
	MaxPrice = 300_000
)

var ErrInvalidForm = errors.New("point form has invalid fields")

// ErrDuplicateURL is the form's answer «эта квартира уже есть, в пуле N —
// войти?» (§6): the link is not free, but the point behind it is somewhere the
// author can go.
type ErrDuplicateURL struct {
	Existing entity.Point
}

type Repository interface {
	Create(ctx context.Context, p entity.Point) (entity.Point, error)
	ByURLNorm(ctx context.Context, urlNorm string) (*entity.Point, error)
	CardByID(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error)
	Feed(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]pointsrepo.FeedPoint, error)
	Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []pointsrepo.FeedPoint, err error)
	PoolMembers(ctx context.Context, viewerID, pointID uuid.UUID) ([]pointsrepo.Member, error)
	CountActiveIn(ctx context.Context, zoneIDs []string) (int, error)
	Enter(ctx context.Context, userID, pointID uuid.UUID) error
	Leave(ctx context.Context, userID, pointID uuid.UUID) error
	InPool(ctx context.Context, userID, pointID uuid.UUID) (bool, error)
}

// Searches is the viewer's own side of the verdict chips: пул квартиры несёт
// их, только если смотрящему есть чем сравнивать (§6).
type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
}

// Notifier is the moderation chat (§5). Ошибка отправки создание точки не
// отменяет: очередь всегда можно дослать командой /queue.
type Notifier interface {
	PointPending(ctx context.Context, p entity.Point)
}

// CreateFlat is «Найди, с кем снять эту квартиру» (§6) — ссылка с рынка.
type CreateFlat struct {
	URL          string
	ZoneID       string
	PriceTotal   int
	Rooms        *int
	Isolated     *bool
	Bathrooms    *entity.Bathrooms
	Utilities    *entity.Utilities
	Restrictions []entity.Restriction
	PhotoKey     string
	Deposit      *int
}

// CreateIntentHome is «Зову соседа к себе» (§2.2): цена здесь — доля
// въезжающего, а фото — из квартиры, а не скрин объявления.
type CreateIntentHome struct {
	Stage        entity.PointStage
	ZoneID       string
	PriceShare   int
	Rooms        *int
	Isolated     *bool
	Restrictions []entity.Restriction
	PhotoKey     string
	MoveIn       *time.Time
	Deposit      *int
}

type MemberView struct {
	pointsrepo.Member
	// Chips are empty when either side has no active search — сравнивать
	// нечего, карточка идёт без вердиктов (§6).
	Chips         []pools.Chip
	SharedAnswers int
}

type PoolView struct {
	Members []MemberView
}

type Service struct {
	repo     Repository
	searches Searches
	dir      *zones.Directory
	notify   Notifier
}

func New(repo Repository, searches Searches, dir *zones.Directory, notify Notifier) *Service {
	return &Service{repo: repo, searches: searches, dir: dir, notify: notify}
}

func (e *ErrDuplicateURL) Error() string {
	return fmt.Sprintf("url already belongs to point %s", e.Existing.ID)
}

func (s *Service) CreateFlat(ctx context.Context, userID uuid.UUID, in CreateFlat) (entity.Point, error) {
	p, err := s.validFlat(in)
	if err != nil {
		return entity.Point{}, err
	}
	p.CreatedBy = &userID
	return s.create(ctx, p)
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

// Point is one card. Всё, кроме одобренной точки, видит только создатель: она
// ему видна сразу, «на проверке», а в общую ленту уходит после аппрува (§5).
func (s *Service) Point(ctx context.Context, viewerID, pointID uuid.UUID) (pointsrepo.FeedPoint, error) {
	card, err := s.repo.CardByID(ctx, viewerID, pointID)
	if err != nil {
		return pointsrepo.FeedPoint{}, err
	}
	if card.Point.Status != entity.PointActive && pointer.Get(card.Point.CreatedBy) != viewerID {
		return pointsrepo.FeedPoint{}, entity.ErrNotFound
	}
	return card, nil
}

func (s *Service) Enter(ctx context.Context, userID, pointID uuid.UUID) error {
	return s.repo.Enter(ctx, userID, pointID)
}

func (s *Service) Leave(ctx context.Context, userID, pointID uuid.UUID) error {
	return s.repo.Leave(ctx, userID, pointID)
}

// Pool is the list of everyone who entered — «в пуле N — увидишь после входа»
// (§6): вход бесплатен и безлимитен, поэтому гейт стоит на нём, а не на поиске.
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

// CountIn answers «M квартир под эти параметры» (§6) — тизер выдачи на карточке
// поиска, поэтому считает то же, что показала бы лента этой зоны.
func (s *Service) CountIn(ctx context.Context, zoneID string) (int, error) {
	zoneIDs := s.dir.Intersecting(zoneID)
	if len(zoneIDs) == 0 {
		return 0, nil
	}
	return s.repo.CountActiveIn(ctx, zoneIDs)
}

// create runs the dedup §6 promises and hands the point to the moderator.
func (s *Service) create(ctx context.Context, p entity.Point) (entity.Point, error) {
	if p.URLNorm != nil {
		existing, err := s.repo.ByURLNorm(ctx, *p.URLNorm)
		if err != nil {
			return entity.Point{}, err
		}
		if existing != nil {
			return entity.Point{}, &ErrDuplicateURL{Existing: *existing}
		}
	}

	created, err := s.repo.Create(ctx, p)
	// Гонка двух подач одной ссылки: проигравший обязан услышать тот же дедуп,
	// что и опоздавший на секунду, а не ошибку базы.
	if errors.Is(err, pointsrepo.ErrURLTaken) && p.URLNorm != nil {
		if existing, e := s.repo.ByURLNorm(ctx, *p.URLNorm); e == nil && existing != nil {
			return entity.Point{}, &ErrDuplicateURL{Existing: *existing}
		}
	}
	if err != nil {
		return entity.Point{}, err
	}

	if s.notify != nil {
		s.notify.PointPending(ctx, created)
	}
	return created, nil
}

// searchNear picks the search the chips speak for: зона точки — станция, так
// что пересечение значит «его зона содержит эту станцию». Свежайший из
// подходящих, как и в зонном пуле.
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
