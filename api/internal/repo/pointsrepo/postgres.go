package pointsrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres"
)

func (r *Repo) Create(ctx context.Context, p entity.Point) (entity.Point, error) {
	var out entity.Point
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if p.CreatedBy != nil {
			if err := lockAndCheckLimits(ctx, tx, p); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO points (type, stage, status, source, created_by, zone_id,
				price_total, price_share, rooms, isolated, bathrooms, utilities,
				restrictions, photo_key, url_norm, move_in, deposit)
			VALUES ($1, $2, COALESCE(NULLIF($3, ''), 'pending'), $4, $5, $6,
				$7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16, $17)
			RETURNING `+pointColumns,
			string(p.Type), enumArg(p.Stage), string(p.Status), string(p.Source),
			p.CreatedBy, p.ZoneID, p.PriceTotal, p.PriceShare, p.Rooms, p.Isolated,
			enumArg(p.Bathrooms), enumArg(p.Utilities), restrictionsArg(p.Restrictions),
			p.PhotoKey, p.URLNorm, p.MoveIn, p.Deposit)
		if err != nil {
			return err
		}
		out, err = collectPoint(rows)
		return err
	})
	if err != nil {
		return entity.Point{}, fmt.Errorf("create %s point: %w", p.Type, err)
	}
	return out, nil
}

func (r *Repo) ByID(ctx context.Context, id uuid.UUID) (entity.Point, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+pointColumns+` FROM points WHERE id = $1`, id)
	if err != nil {
		return entity.Point{}, fmt.Errorf("get point %s: %w", id, err)
	}
	point, err := collectPoint(rows)
	if err != nil {
		return entity.Point{}, fmt.Errorf("get point %s: %w", id, err)
	}
	return point, nil
}

func (r *Repo) ByURLNorm(ctx context.Context, urlNorm string) (*entity.Point, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+pointColumns+` FROM points WHERE url_norm = $1 AND status <> 'closed'`, urlNorm)
	if err != nil {
		return nil, fmt.Errorf("get point by url %s: %w", urlNorm, err)
	}
	point, err := collectPoint(rows)
	switch {
	case errors.Is(err, entity.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("get point by url %s: %w", urlNorm, err)
	default:
		return pointer.To(point), nil
	}
}

func (r *Repo) Enter(ctx context.Context, userID, pointID uuid.UUID) error {
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		open, err := exists(ctx, tx, `
			SELECT 1 FROM points
			WHERE id = $1 AND type = 'flat' AND status = 'active'`, pointID)
		if err != nil {
			return err
		}
		if !open {
			return entity.ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO likes_points (user_id, point_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, userID, pointID)
		return err
	})
	if err != nil {
		return fmt.Errorf("enter pool of point %s: %w", pointID, err)
	}
	return nil
}

func (r *Repo) Leave(ctx context.Context, userID, pointID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM likes_points WHERE user_id = $1 AND point_id = $2`, userID, pointID)
	if err != nil {
		return fmt.Errorf("leave pool of point %s: %w", pointID, err)
	}
	return nil
}

func (r *Repo) InPool(ctx context.Context, userID, pointID uuid.UUID) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM likes_points WHERE user_id = $1 AND point_id = $2`, userID, pointID).Scan(&one)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("pool membership of %s in point %s: %w", userID, pointID, err)
	default:
		return true, nil
	}
}

func lockAndCheckLimits(ctx context.Context, tx pgx.Tx, p entity.Point) error {
	var locked uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE id = $1 FOR NO KEY UPDATE`, *p.CreatedBy).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ErrNotFound
		}
		return err
	}

	if p.Type == entity.PointIntentHome {
		var total, live int
		if err := tx.QueryRow(ctx, `
			SELECT count(*), count(*) FILTER (WHERE stage = 'live')
			FROM points
			WHERE created_by = $1 AND type = 'intent_home' AND `+livePoint,
			*p.CreatedBy).Scan(&total, &live); err != nil {
			return err
		}
		if total >= entity.MaxIntentHome {
			return entity.ErrPointLimit
		}
		if pointer.Get(p.Stage) == entity.StageLive && live >= entity.MaxIntentHomeLive {
			return entity.ErrPointLimit
		}
		return nil
	}

	if p.Source != entity.SourceUserLink {
		return nil
	}
	var flats int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM points
		WHERE created_by = $1 AND type = 'flat' AND source = 'user_link' AND `+livePoint,
		*p.CreatedBy).Scan(&flats); err != nil {
		return err
	}
	if flats >= entity.MaxUserLinkPoints {
		return entity.ErrPointLimit
	}
	return nil
}

func collectPoint(rows pgx.Rows) (entity.Point, error) {
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[pointRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Point{}, entity.ErrNotFound
		}
		return entity.Point{}, err
	}
	return row.toEntity(), nil
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
