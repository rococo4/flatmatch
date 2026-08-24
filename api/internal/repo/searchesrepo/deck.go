package searchesrepo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

func (r *Repo) DeckCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string,
	f entity.PeopleFilter, within time.Duration,
) ([]DeckCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+candidateColumns+`
		FROM searches s
		JOIN visible_people($1) vp ON vp = s.user_id
		JOIN users u ON u.id = s.user_id
		WHERE s.status = 'active'
		  AND s.zone_id = ANY($2)
		  AND NOT EXISTS (
		      SELECT 1 FROM matches m
		      WHERE m.a_id = least($1::uuid, s.user_id)
		        AND m.b_id = greatest($1::uuid, s.user_id))
		  AND NOT EXISTS (
		      SELECT 1 FROM likes_users l
		      WHERE l.from_id = $1 AND l.to_id = s.user_id)
		  AND NOT EXISTS (
		      SELECT 1 FROM passes p
		      WHERE p.user_id = $1 AND p.target_id = s.user_id
		        AND p.created_at >= now() - make_interval(secs => $3))
		  AND `+peopleFilter+`
		ORDER BY s.user_id, s.created_at DESC, s.id`,
		viewerID, zoneIDs, within.Seconds(),
		f.AgeMin, f.AgeMax, f.Occupation, f.Schedule, f.Smoking, f.PetsOwnNone, f.PetsOKFine)
	if err != nil {
		return nil, fmt.Errorf("deck candidates for viewer %s: %w", viewerID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[candidateRow])
	if err != nil {
		return nil, fmt.Errorf("deck candidates for viewer %s: %w", viewerID, err)
	}

	out := make([]DeckCandidate, 0, len(raw))
	at := make(map[uuid.UUID]int, len(raw))
	for _, row := range raw {
		i, seen := at[row.UID]
		if !seen {
			i = len(out)
			at[row.UID] = i
			out = append(out, DeckCandidate{User: row.user()})
		}
		out[i].Searches = append(out[i].Searches, row.searchRow.toEntity())
	}
	return out, nil
}

func (r *Repo) ActiveByUsers(ctx context.Context, userIDs []uuid.UUID) ([]entity.Search, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+` FROM searches
		WHERE user_id = ANY($1) AND status = 'active'
		ORDER BY user_id, created_at DESC, id`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("active searches of %d users: %w", len(userIDs), err)
	}
	out, err := collectSearches(rows)
	if err != nil {
		return nil, fmt.Errorf("active searches of %d users: %w", len(userIDs), err)
	}
	return out, nil
}
