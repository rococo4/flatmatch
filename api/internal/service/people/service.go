package people

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type Repository interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error)
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
}

type Reports interface {
	Insert(ctx context.Context, r entity.Report) (entity.Report, bool, error)
}

type Matches interface {
	IsMatched(ctx context.Context, a, b uuid.UUID) (bool, error)
}

type Moderation interface {
	ReportFiled(ctx context.Context, r entity.Report, reporter, target entity.User)
}

type Service struct {
	users      Repository
	reports    Reports
	matches    Matches
	moderation Moderation
}

func New(users Repository, reports Reports, matches Matches, moderation Moderation) *Service {
	return &Service{users: users, reports: reports, matches: matches, moderation: moderation}
}

func (s *Service) Block(ctx context.Context, viewerID, targetID uuid.UUID) error {
	if viewerID == targetID {
		return entity.ErrSelfTarget
	}
	if _, err := s.users.ByID(ctx, targetID); err != nil {
		return err
	}
	return s.users.Block(ctx, viewerID, targetID)
}

func (s *Service) Report(
	ctx context.Context, viewer entity.User, targetID uuid.UUID,
	reason entity.ReportReason, contextKey string,
) error {
	if viewer.ID == targetID {
		return entity.ErrSelfTarget
	}

	target, err := s.target(ctx, viewer.ID, targetID)
	if err != nil {
		return err
	}

	stored, inserted, err := s.reports.Insert(ctx, entity.Report{
		ReporterID: viewer.ID,
		TargetID:   targetID,
		Reason:     reason,
		ContextKey: contextKey,
	})
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	s.moderation.ReportFiled(context.WithoutCancel(ctx), stored, viewer, target)
	return nil
}

func (s *Service) target(ctx context.Context, viewerID, targetID uuid.UUID) (entity.User, error) {
	target, err := s.users.VisibleByID(ctx, viewerID, targetID)
	switch {
	case err == nil:
		return target, nil
	case !errors.Is(err, entity.ErrNotFound):
		return entity.User{}, err
	}

	matched, err := s.matches.IsMatched(ctx, viewerID, targetID)
	if err != nil {
		return entity.User{}, err
	}
	if !matched {
		return entity.User{}, entity.ErrNotFound
	}
	return s.users.ByID(ctx, targetID)
}
