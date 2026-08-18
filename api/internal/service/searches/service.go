package searches

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error)
	ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
	UpdateParams(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error)
	Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
	Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
}

type Pushes interface {
	ZoneNew(ctx context.Context, search entity.Search)
}

type Service struct {
	repo   Repository
	dir    *zones.Directory
	pushes Pushes
}

var ErrInvalidParams = errors.New("search params are invalid")

func New(repo Repository, dir *zones.Directory, pushes Pushes) *Service {
	return &Service{repo: repo, dir: dir, pushes: pushes}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	p, err := s.validated(p)
	if err != nil {
		return entity.Search{}, err
	}
	created, err := s.repo.Create(ctx, userID, p)
	if err != nil {
		return entity.Search{}, err
	}
	s.announce(ctx, created)
	return created, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]entity.Search, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	current, err := s.repo.ByID(ctx, userID, id)
	if err != nil {
		return entity.Search{}, err
	}

	if p.ZoneID != current.ZoneID {
		return entity.Search{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"zone_id"}}
	}
	p, err = s.validated(p)
	if err != nil {
		return entity.Search{}, err
	}
	return s.repo.UpdateParams(ctx, userID, id, p)
}

func (s *Service) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	if reason == "" || reason == entity.CloseReasonUnknown ||
		entity.ParseCloseReason(string(reason)) != reason {
		return entity.Search{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"reason"}}
	}
	return s.repo.Close(ctx, userID, id, reason)
}

func (s *Service) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	search, err := s.repo.Reactivate(ctx, userID, id)
	if err != nil {
		return entity.Search{}, err
	}
	s.announce(ctx, search)
	return search, nil
}

func (s *Service) announce(ctx context.Context, search entity.Search) {
	if s.pushes == nil {
		return
	}
	s.pushes.ZoneNew(ctx, search)
}

func (s *Service) validated(p entity.SearchParams) (entity.SearchParams, error) {
	if p.MoveIn != nil {
		monthStart := time.Date(p.MoveIn.Year(), p.MoveIn.Month(), 1, 0, 0, 0, 0, time.UTC)
		p.MoveIn = &monthStart
	}

	bad := p.InvalidFields()
	if _, ok := s.dir.Get(p.ZoneID); p.ZoneID != "" && !ok {
		bad = append(bad, "zone_id")
	}
	if len(bad) > 0 {
		return entity.SearchParams{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: bad}
	}
	return p, nil
}
