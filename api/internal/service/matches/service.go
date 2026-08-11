package matches

import (
	"context"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]matchesrepo.MatchWithPeer, error)
	ByID(ctx context.Context, userID, id uuid.UUID) (matchesrepo.MatchWithPeer, error)
}

type Searches interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error)
}

type Users interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
}

type Service struct {
	repo     Repository
	searches Searches
	users    Users
	dir      *zones.Directory
}

func New(repo Repository, searches Searches, users Users, dir *zones.Directory) *Service {
	return &Service{repo: repo, searches: searches, users: users, dir: dir}
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]matchesrepo.MatchWithPeer, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *Service) Detail(ctx context.Context, userID, id uuid.UUID) (matchesrepo.MatchWithPeer, Summary, error) {
	match, err := s.repo.ByID(ctx, userID, id)
	if err != nil {
		return matchesrepo.MatchWithPeer{}, Summary{}, err
	}
	match = withoutHiddenContacts(match)

	viewer, err := s.users.ByID(ctx, userID)
	if err != nil {
		return matchesrepo.MatchWithPeer{}, Summary{}, err
	}
	summary, err := s.summary(ctx, viewer, match)
	if err != nil {
		return matchesrepo.MatchWithPeer{}, Summary{}, err
	}
	return match, summary, nil
}

func (s *Service) summary(ctx context.Context, viewer entity.User, match matchesrepo.MatchWithPeer) (Summary, error) {
	out := Summary{SharedAnswers: viewer.Profile.Shared(match.Peer.Profile)}
	if zone, ok := s.dir.Get(match.Match.ContextZone); ok {
		out.Zone = &zone
	}

	mine, err := s.activeSearch(ctx, viewer.ID)
	if err != nil {
		return Summary{}, err
	}
	theirs, err := s.activeSearch(ctx, match.Peer.ID)
	if err != nil {
		return Summary{}, err
	}
	if mine != nil && theirs != nil {
		out.ParamsOverlap = paramsOverlap(*mine, *theirs)
	}
	return out, nil
}

func (s *Service) activeSearch(ctx context.Context, userID uuid.UUID) (*entity.SearchParams, error) {
	list, err := s.searches.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	for _, search := range list {
		if search.Status == entity.SearchActive {
			return &search.SearchParams, nil
		}
	}
	return nil, nil
}

func withoutHiddenContacts(m matchesrepo.MatchWithPeer) matchesrepo.MatchWithPeer {
	if m.Peer.Visible() && !m.Blocked {
		return m
	}
	m.Peer.Username = ""
	m.Peer.TelegramID = 0
	return m
}
