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

type Pushes interface {
	Liked(ctx context.Context, toID, fromID uuid.UUID, fromName string, fromGender entity.Gender)
	Matched(ctx context.Context, match entity.Match)
}

type Service struct {
	repo   Repository
	dir    *zones.Directory
	pushes Pushes
}

var ErrInvalidParams = errors.New("like params are invalid")

func New(repo Repository, dir *zones.Directory, pushes Pushes) *Service {
	return &Service{repo: repo, dir: dir, pushes: pushes}
}

func (s *Service) Like(ctx context.Context, from entity.User, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error) {
	if zoneID != "" && pointID != nil {
		return entity.LikeResult{}, &entity.FieldsError{
			Err: ErrInvalidParams, Fields: []string{"zone_id", "point_id"}}
	}
	if _, ok := s.dir.Get(zoneID); zoneID != "" && !ok {
		return entity.LikeResult{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"zone_id"}}
	}

	var contextZones []string
	if zoneID != "" {
		contextZones = s.dir.Intersecting(zoneID)
	}

	res, err := s.repo.Create(ctx, from.ID, toID,
		likesrepo.LikeContext{ZoneID: zoneID, ContextZones: contextZones, PointID: pointID})
	if err != nil {
		return entity.LikeResult{}, err
	}
	s.announce(ctx, from, toID, res)
	return res, nil
}

func (s *Service) announce(ctx context.Context, from entity.User, toID uuid.UUID, res entity.LikeResult) {
	if s.pushes == nil {
		return
	}

	if !res.Already && res.Like.Spent {
		s.pushes.Liked(ctx, toID, from.ID, from.Name, from.Profile.Gender)
	}

	if res.Match != nil {
		s.pushes.Matched(ctx, *res.Match)
	}
}

func (s *Service) Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error) {
	return s.repo.Incoming(ctx, userID)
}

func (s *Service) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error) {
	return s.repo.Budget(ctx, userID)
}
