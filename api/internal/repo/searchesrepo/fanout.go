package searchesrepo

import (
	"context"
	"fmt"

	"flatmatch/api/internal/entity"
)

func (r *Repo) ActiveIn(ctx context.Context, zoneIDs []string) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+`
		FROM searches
		WHERE status = 'active' AND zone_id = ANY($1)
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
