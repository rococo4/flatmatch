package likes

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	Create(ctx context.Context, fromID, toID uuid.UUID, lc likesrepo.LikeContext) (entity.LikeResult, error)
	Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

type Service struct {
	repo Repository
	dir  *zones.Directory
}

var ErrInvalidParams = errors.New("like params are invalid")

func New(repo Repository, dir *zones.Directory) *Service {
	return &Service{repo: repo, dir: dir}
}

func (s *Service) Like(ctx context.Context, fromID, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error) {
	// Один лайк — один контекст: мэтч потом рассказывает, ГДЕ пересеклись, и на
	// два места сразу этот рассказ не делится (§6).
	if zoneID != "" && pointID != nil {
		return entity.LikeResult{}, &entity.FieldsError{
			Err: ErrInvalidParams, Fields: []string{"zone_id", "point_id"}}
	}
	if _, ok := s.dir.Get(zoneID); zoneID != "" && !ok {
		return entity.LikeResult{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"zone_id"}}
	}
	// Лайк без зоны контекста тратящим быть не может: пустой список зон валит
	// гейт поиска в ErrNeedSearch, ответным лайкам гейт и так не мешает.
	var contextZones []string
	if zoneID != "" {
		contextZones = s.dir.Intersecting(zoneID)
	}
	// Точку валидирует транзакция: активность, тип и отношение к ней читаются
	// там же, где решается трата слота.
	return s.repo.Create(ctx, fromID, toID,
		likesrepo.LikeContext{ZoneID: zoneID, ContextZones: contextZones, PointID: pointID})
}

func (s *Service) Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error) {
	return s.repo.Incoming(ctx, userID)
}

func (s *Service) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error) {
	return s.repo.Budget(ctx, userID)
}
