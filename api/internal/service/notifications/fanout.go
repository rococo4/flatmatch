package notifications

import (
	"context"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func (s *Service) ZoneNew(ctx context.Context, search entity.Search) {
	ctx = detached(ctx)

	subs, err := s.searches.ActiveIn(ctx, s.dir.Intersecting(search.ZoneID))
	if err != nil {
		s.log.Warn("find the subscribers of a zone", "zone", search.ZoneID, "error", err)
		return
	}

	rows := make([]entity.Notification, 0, len(subs))
	answered := make(map[uuid.UUID]bool, len(subs))
	for _, sub := range subs {
		if sub.UserID == search.UserID {
			continue
		}
		reachable, asked := answered[sub.UserID]
		if !asked {
			reachable, err = s.reaches(ctx, sub.UserID, search.UserID)
			if err != nil {
				s.log.Warn("check whether a subscriber sees the newcomer",
					"subscriber", sub.UserID, "newcomer", search.UserID, "error", err)
				continue
			}
			answered[sub.UserID] = reachable
		}
		if !reachable {
			continue
		}
		rows = append(rows, entity.Notification{
			UserID:     sub.UserID,
			Type:       entity.NotifZoneNew,
			ActorKey:   search.UserID.String(),
			ContextKey: sub.ZoneID,
		})
	}
	s.many(ctx, entity.NotifZoneNew, rows)
}

func (s *Service) PoolNew(ctx context.Context, pointID, enteredUserID uuid.UUID) {
	ctx = detached(ctx)

	members, err := s.points.PoolMemberIDs(ctx, enteredUserID, pointID)
	if err != nil {
		s.log.Warn("find the members of a pool", "point", pointID, "error", err)
		return
	}

	rows := make([]entity.Notification, 0, len(members))
	for _, member := range members {
		if member == enteredUserID {
			continue
		}
		reachable, err := s.reaches(ctx, member, enteredUserID)
		if err != nil {
			s.log.Warn("check whether a pool member sees the newcomer",
				"member", member, "newcomer", enteredUserID, "error", err)
			continue
		}
		if !reachable {
			continue
		}
		rows = append(rows, entity.Notification{
			UserID:     member,
			Type:       entity.NotifPoolNew,
			ActorKey:   enteredUserID.String(),
			ContextKey: pointID.String(),
		})
	}
	s.many(ctx, entity.NotifPoolNew, rows)
}

func (s *Service) reaches(ctx context.Context, viewerID, actorID uuid.UUID) (bool, error) {
	visible, err := s.people.VisibleBetween(ctx, viewerID, actorID)
	if err != nil || !visible {
		return false, err
	}
	matched, err := s.people.IsMatched(ctx, viewerID, actorID)
	if err != nil {
		return false, err
	}
	return !matched, nil
}
