package pointsrepo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const peopleFilter = `($4::int IS NULL OR u.age >= $4)
	  AND ($5::int IS NULL OR u.age <= $5)
	  AND (coalesce(cardinality($6::text[]), 0) = 0 OR u.occupation = ANY($6))
	  AND (coalesce(cardinality($7::text[]), 0) = 0 OR u.schedule = ANY($7))
	  AND (coalesce(cardinality($8::text[]), 0) = 0 OR u.smoking = ANY($8))
	  AND (NOT ($9::bool OR $10::bool)
	       OR ($9 AND u.pets_own = 'none')
	       OR ($10 AND u.pets_ok = 'ok'))`

func (r *Repo) DeckRooms(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
	f entity.PeopleFilter, within time.Duration,
) ([]FeedPoint, error) {
	rooms, err := r.feed(ctx, `p.type = 'intent_home' AND p.status = 'active'
		  AND p.zone_id = ANY($2)
		  AND p.created_by IS DISTINCT FROM $1
		  AND EXISTS (SELECT 1 FROM visible_people($1) vp WHERE vp = p.created_by)
		  AND `+notMatchedWith("p.created_by")+`
		  AND NOT EXISTS (
		      SELECT 1 FROM likes_users l
		      WHERE l.from_id = $1 AND l.to_id = p.created_by)
		  AND NOT EXISTS (
		      SELECT 1 FROM passes ps
		      WHERE ps.user_id = $1 AND ps.target_id = p.id
		        AND ps.created_at >= now() - make_interval(secs => $3))
		  AND NOT EXISTS (
		      SELECT 1 FROM passes ps
		      WHERE ps.user_id = $1 AND ps.target_id = p.created_by
		        AND ps.created_at >= now() - make_interval(secs => $3))
		  AND `+peopleFilter,
		viewerID, zoneIDs, within.Seconds(),
		f.AgeMin, f.AgeMax, f.Occupation, f.Schedule, f.Smoking, f.PetsOwnNone, f.PetsOKFine)
	if err != nil {
		return nil, fmt.Errorf("deck rooms for viewer %s: %w", viewerID, err)
	}
	return rooms, nil
}
