package matchesrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
)

func (r *Repo) SavePollAnswer(ctx context.Context, matchID, userID uuid.UUID, answer entity.PollAnswer) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `
		SELECT 1 FROM matches WHERE id = $1 AND $2::uuid IN (a_id, b_id)`,
		matchID, userID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("save poll answer for match %s: %w", matchID, entity.ErrNotFound)
	}
	if err != nil {
		return false, fmt.Errorf("save poll answer for match %s: %w", matchID, err)
	}

	tag, err := r.pool.Exec(ctx, `
		INSERT INTO match_polls (match_id, user_id, answer)
		VALUES ($1, $2, $3)
		ON CONFLICT (match_id, user_id) DO NOTHING`, matchID, userID, string(answer))
	if err != nil {
		return false, fmt.Errorf("save poll answer for match %s: %w", matchID, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repo) MatchesForPostPoll(ctx context.Context, olderThan time.Duration) ([]PollTarget, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+matchColumns+`, side.user_id, u.name AS peer_name
		FROM matches m
		CROSS JOIN LATERAL (VALUES (m.a_id, m.b_id), (m.b_id, m.a_id)) AS side(user_id, peer_id)
		JOIN users u ON u.id = side.peer_id
		WHERE m.created_at < now() - make_interval(secs => $1)
		  AND NOT EXISTS (
		      SELECT 1 FROM match_polls p
		      WHERE p.match_id = m.id AND p.user_id = side.user_id)
		ORDER BY m.created_at, side.user_id`, olderThan.Seconds())
	if err != nil {
		return nil, fmt.Errorf("matches for the post-match poll: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[pollRow])
	if err != nil {
		return nil, fmt.Errorf("matches for the post-match poll: %w", err)
	}
	out := make([]PollTarget, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}
