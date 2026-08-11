package matchesrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
)

const peerJoin = `JOIN users u ON u.id = CASE WHEN m.a_id = $1::uuid THEN m.b_id ELSE m.a_id END`

func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]MatchWithPeer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+matchColumns+`, `+blockedColumn+`, `+peerColumns+`
		FROM matches m
		`+peerJoin+`
		WHERE $1::uuid IN (m.a_id, m.b_id)
		ORDER BY m.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list matches of user %s: %w", userID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[peerRow])
	if err != nil {
		return nil, fmt.Errorf("list matches of user %s: %w", userID, err)
	}
	out := make([]MatchWithPeer, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) ByID(ctx context.Context, userID, id uuid.UUID) (MatchWithPeer, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+matchColumns+`, `+blockedColumn+`, `+peerColumns+`, `+contactColumns+`
		FROM matches m
		`+peerJoin+`
		WHERE m.id = $2 AND $1::uuid IN (m.a_id, m.b_id)`, userID, id)
	if err != nil {
		return MatchWithPeer{}, fmt.Errorf("get match %s of user %s: %w", id, userID, err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[contactRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = entity.ErrNotFound
		}
		return MatchWithPeer{}, fmt.Errorf("get match %s of user %s: %w", id, userID, err)
	}
	return row.toEntity(), nil
}
