package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	Insert(ctx context.Context, n entity.Notification) (bool, error)
	InsertMany(ctx context.Context, ns []entity.Notification) (int, error)
}

// Searches отдаёт подписчиков зоны: активный поиск — это и есть подписка на
// «появился новый совпавший» (§3).
type Searches interface {
	ActiveIn(ctx context.Context, zoneIDs []string) ([]entity.Search, error)
}

type Points interface {
	PoolMemberIDs(ctx context.Context, viewerID, pointID uuid.UUID) ([]uuid.UUID, error)
}

type People interface {
	VisibleBetween(ctx context.Context, a, b uuid.UUID) (bool, error)
	IsMatched(ctx context.Context, a, b uuid.UUID) (bool, error)
}

type Service struct {
	repo     Repository
	searches Searches
	points   Points
	people   People
	dir      *zones.Directory
	log      *slog.Logger
}

func New(repo Repository, searches Searches, points Points, people People,
	dir *zones.Directory, log *slog.Logger) *Service {
	return &Service{repo: repo, searches: searches, points: points, people: people, dir: dir, log: log}
}

func (s *Service) Liked(ctx context.Context, toID, fromID uuid.UUID, fromName string, fromGender entity.Gender) {
	s.one(ctx, entity.Notification{
		UserID:   toID,
		Type:     entity.NotifLiked,
		ActorKey: fromID.String(),
		Payload:  map[string]any{"name": fromName, "gender": string(fromGender)},
	})
}

func (s *Service) Matched(ctx context.Context, match entity.Match) {
	for _, userID := range []uuid.UUID{match.AID, match.BID} {
		s.one(ctx, entity.Notification{
			UserID:   userID,
			Type:     entity.NotifMatched,
			ActorKey: match.ID.String(),
		})
	}
}

func (s *Service) ProfileVerdict(ctx context.Context, userID uuid.UUID, typ entity.NotificationType) {
	switch typ {
	case entity.NotifProfileOK, entity.NotifPhotoRejected, entity.NotifAboutRejected:
	default:
		s.log.Warn("not a profile verdict, nothing to push", "type", typ, "user", userID)
		return
	}
	s.one(ctx, entity.Notification{
		UserID:   userID,
		Type:     typ,
		ActorKey: verdictActor(userID, typ, time.Now()),
	})
}

func (s *Service) PointVerdict(ctx context.Context, point entity.Point, ok bool) {
	if point.CreatedBy == nil {
		return
	}
	typ := entity.NotifPointRejected
	if ok {
		typ = entity.NotifPointOK
	}
	s.one(ctx, entity.Notification{
		UserID:     *point.CreatedBy,
		Type:       typ,
		ActorKey:   verdictActor(point.ID, typ, time.Now()),
		ContextKey: point.ID.String(),
	})
}

func (s *Service) one(ctx context.Context, n entity.Notification) {
	if _, err := s.repo.Insert(detached(ctx), n); err != nil {
		s.log.Warn("write a notification", "type", n.Type, "user", n.UserID, "error", err)
	}
}

func (s *Service) many(ctx context.Context, typ entity.NotificationType, rows []entity.Notification) {
	if len(rows) == 0 {
		return
	}
	if _, err := s.repo.InsertMany(detached(ctx), rows); err != nil {
		s.log.Warn("write a notification fan-out", "type", typ, "rows", len(rows), "error", err)
	}
}

func detached(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

func verdictActor(subject uuid.UUID, typ entity.NotificationType, at time.Time) string {
	return fmt.Sprintf("%s:%s:%d", subject, typ, at.Unix())
}
