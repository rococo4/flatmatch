package searchesrepo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func (r *Repo) ActiveIn(ctx context.Context, zoneIDs []string) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+`
		FROM searches
		WHERE status = 'active' AND zone_id = ANY($1)
		  AND EXISTS (SELECT 1 FROM users u WHERE u.id = searches.user_id AND u.status = 'active')
		ORDER BY user_id, created_at, id`, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("active searches in zones: %w", err)
	}
	out, err := collectSearches(rows)
	if err != nil {
		return nil, fmt.Errorf("active searches in zones: %w", err)
	}
	return out, nil
}

func (r *Repo) ActiveByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+`
		FROM searches
		WHERE user_id = $1 AND status = 'active'
		ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, fmt.Errorf("active searches of user %s: %w", userID, err)
	}
	out, err := collectSearches(rows)
	if err != nil {
		return nil, fmt.Errorf("active searches of user %s: %w", userID, err)
	}
	return out, nil
}
