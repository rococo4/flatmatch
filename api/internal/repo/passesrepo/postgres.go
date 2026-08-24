package passesrepo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repo) Upsert(ctx context.Context, userID, targetID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO passes (user_id, target_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, target_id) DO UPDATE SET created_at = now()`, userID, targetID)
	if err != nil {
		return fmt.Errorf("pass %s by %s: %w", targetID, userID, err)
	}
	return nil
}

func (r *Repo) ActiveTargets(ctx context.Context, userID uuid.UUID, within time.Duration) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT target_id
		FROM passes
		WHERE user_id = $1 AND created_at >= now() - make_interval(secs => $2)
		ORDER BY created_at DESC, target_id`, userID, within.Seconds())
	if err != nil {
		return nil, fmt.Errorf("list passes of %s: %w", userID, err)
	}

	targets, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("list passes of %s: %w", userID, err)
	}
	return targets, nil
}

func (r *Repo) Clear(ctx context.Context, userID uuid.UUID) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM passes WHERE user_id = $1`, userID)
	if err != nil {
		return 0, fmt.Errorf("clear the passes of %s: %w", userID, err)
	}
	return int(tag.RowsAffected()), nil
}
