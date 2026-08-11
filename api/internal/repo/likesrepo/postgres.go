package likesrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres"
)

// Create runs the whole §2.3 mechanics in one transaction.
func (r *Repo) Create(ctx context.Context, fromID, toID uuid.UUID, zoneID string) (entity.LikeResult, error) {
	var out entity.LikeResult
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		status, err := lockPair(ctx, tx, fromID, toID)
		if err != nil {
			return err
		}
		// Draft and pending may browse and save searches, but not spend a dear
		// like (§5). The gate stands before everything else, including the reply
		// path: nobody can have liked a draft first — visible_people hides them
		// — so an unapproved reply is not a case, it is a bug elsewhere.
		if status != entity.StatusActive {
			return entity.ErrNotApproved
		}

		visible, err := exists(ctx, tx,
			`SELECT 1 FROM visible_people($1::uuid) vp WHERE vp = $2::uuid`, fromID, toID)
		if err != nil {
			return err
		}
		if !visible {
			return entity.ErrNotFound
		}

		reply, err := exists(ctx, tx,
			`SELECT 1 FROM likes_users WHERE from_id = $1 AND to_id = $2`, toID, fromID)
		if err != nil {
			return err
		}
		// An existing like of ours makes this tap a repeat, and a repeat changes
		// nothing — so the gates below, which guard the SPENDING of a slot, must
		// not run. Otherwise a second tap on the fifth person liked today would
		// answer «limit reached» instead of «already liked», and so would a tap
		// made after the search that paid for the like was closed.
		already, err := exists(ctx, tx,
			`SELECT 1 FROM likes_users WHERE from_id = $1 AND to_id = $2`, fromID, toID)
		if err != nil {
			return err
		}

		if !reply && !already {
			hasSearch, err := exists(ctx, tx,
				`SELECT 1 FROM searches WHERE user_id = $1 AND status = 'active'`, fromID)
			if err != nil {
				return err
			}
			if !hasSearch {
				return entity.ErrNeedSearch
			}
			spent, err := spentInWindow(ctx, tx, fromID)
			if err != nil {
				return err
			}
			if spent >= entity.MaxDailyLikes {
				return entity.ErrLikeLimit
			}
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO likes_users (from_id, to_id, zone_id, point_id, spent)
			VALUES ($1, $2, NULLIF($3, ''), NULL, $4)
			ON CONFLICT (from_id, to_id) DO NOTHING`, fromID, toID, zoneID, !reply)
		if err != nil {
			return err
		}
		// A repeat tap: the row is already there, no slot was spent, and the
		// stored context stays the one the first tap called from.
		out.Already = tag.RowsAffected() == 0

		if reply {
			if err := ensureMatch(ctx, tx, fromID, toID); err != nil {
				return err
			}
			if out.Match, err = readMatch(ctx, tx, fromID, toID); err != nil {
				return err
			}
		}
		out.Like, err = readLike(ctx, tx, fromID, toID)
		return err
	})
	if err != nil {
		return entity.LikeResult{}, fmt.Errorf("like %s -> %s: %w", fromID, toID, err)
	}

	if out.Match == nil {
		// The mirrored like may be running right now: the transaction above
		// could not see it, but it holds the pair lock until it commits. Asking
		// for that lock once more queues behind it, so both sides return the
		// same match instead of only the one that happened to be second.
		match, err := r.settledMatch(ctx, fromID, toID)
		if err != nil {
			return entity.LikeResult{}, fmt.Errorf("like %s -> %s: %w", fromID, toID, err)
		}
		out.Match = match
	}

	budget, err := r.Budget(ctx, fromID)
	if err != nil {
		return entity.LikeResult{}, err
	}
	out.Budget = budget
	return out, nil
}

func (r *Repo) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error) {
	var spent int
	var oldest *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT count(*), min(created_at) FROM likes_users
		WHERE `+spentInWindowFilter, userID, windowSeconds).Scan(&spent, &oldest)
	if err != nil {
		return entity.LikeBudget{}, fmt.Errorf("like budget of %s: %w", userID, err)
	}

	budget := entity.LikeBudget{Remaining: max(entity.MaxDailyLikes-spent, 0)}
	if oldest != nil {
		budget.NextFreeAt = pointer.To(oldest.Add(entity.LikeWindow))
	}
	return budget, nil
}

func (r *Repo) Incoming(ctx context.Context, userID uuid.UUID) ([]IncomingLike, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+incomingColumns+`
		FROM likes_users l
		JOIN visible_people($1) vp ON vp = l.from_id
		JOIN users u ON u.id = l.from_id
		WHERE l.to_id = $1
		  AND NOT EXISTS (
		      SELECT 1 FROM matches m
		      WHERE m.a_id = least(l.from_id, l.to_id)
		        AND m.b_id = greatest(l.from_id, l.to_id))
		ORDER BY l.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("incoming likes of %s: %w", userID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[incomingRow])
	if err != nil {
		return nil, fmt.Errorf("incoming likes of %s: %w", userID, err)
	}
	out := make([]IncomingLike, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) Between(ctx context.Context, a, b uuid.UUID) (aLikedB, bLikedA bool, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM likes_users WHERE from_id = $1 AND to_id = $2),
		       EXISTS (SELECT 1 FROM likes_users WHERE from_id = $2 AND to_id = $1)`,
		a, b).Scan(&aLikedB, &bLikedA)
	if err != nil {
		return false, false, fmt.Errorf("likes between %s and %s: %w", a, b, err)
	}
	return aLikedB, bLikedA, nil
}

func (r *Repo) settledMatch(ctx context.Context, fromID, toID uuid.UUID) (*entity.Match, error) {
	var out *entity.Match
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := lockPair(ctx, tx, fromID, toID); err != nil {
			return err
		}
		var err error
		out, err = readMatch(ctx, tx, fromID, toID)
		return err
	})
	return out, err
}

func lockPair(ctx context.Context, tx pgx.Tx, fromID, toID uuid.UUID) (entity.Status, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, status FROM users WHERE id IN ($1::uuid, $2::uuid)
		ORDER BY id FOR NO KEY UPDATE`, fromID, toID)
	if err != nil {
		return entity.StatusUnknown, err
	}
	locked, err := pgx.CollectRows(rows, pgx.RowToStructByPos[lockedUser])
	if err != nil {
		return entity.StatusUnknown, err
	}
	for _, user := range locked {
		if user.ID == fromID {
			return entity.ParseStatus(user.Status), nil
		}
	}
	return entity.StatusUnknown, entity.ErrNotFound
}

func ensureMatch(ctx context.Context, tx pgx.Tx, fromID, toID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO matches (a_id, b_id, context_zone_id, context_point_id)
		SELECT least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid), l.zone_id, l.point_id
		FROM likes_users l
		WHERE (l.from_id, l.to_id) IN (($1::uuid, $2::uuid), ($2::uuid, $1::uuid))
		ORDER BY l.created_at
		LIMIT 1
		ON CONFLICT (a_id, b_id) DO NOTHING`, fromID, toID)
	return err
}

func readLike(ctx context.Context, tx pgx.Tx, fromID, toID uuid.UUID) (entity.Like, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+likeColumns+` FROM likes_users WHERE from_id = $1 AND to_id = $2`, fromID, toID)
	if err != nil {
		return entity.Like{}, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[likeRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Like{}, entity.ErrNotFound
		}
		return entity.Like{}, err
	}
	return row.toEntity(), nil
}

func readMatch(ctx context.Context, tx pgx.Tx, fromID, toID uuid.UUID) (*entity.Match, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+matchColumns+` FROM matches
		WHERE a_id = least($1::uuid, $2::uuid) AND b_id = greatest($1::uuid, $2::uuid)`, fromID, toID)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[matchRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return pointer.To(row.toEntity()), nil
}

func spentInWindow(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (int, error) {
	var spent int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM likes_users WHERE `+spentInWindowFilter, userID, windowSeconds).Scan(&spent)
	return spent, err
}

func exists(ctx context.Context, tx pgx.Tx, query string, args ...any) (bool, error) {
	var one int
	switch err := tx.QueryRow(ctx, query, args...).Scan(&one); {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}
