package usersrepo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (r *Repo) VisibleBetween(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var visible bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM visible_people($1) vp WHERE vp = $2)`, a, b).Scan(&visible)
	if err != nil {
		return false, fmt.Errorf("visibility of %s to %s: %w", b, a, err)
	}
	return visible, nil
}
