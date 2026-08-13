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

func (r *Repo) Feed(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]FeedPoint, error) {
	points, err := r.feed(ctx, `p.status = 'active' AND p.zone_id = ANY($2)
		  AND (p.type = 'flat'
		       OR (EXISTS (SELECT 1 FROM visible_people($1) vp WHERE vp = p.created_by)
		           AND `+notMatchedWith("p.created_by")+`))`, viewerID, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("feed for viewer %s: %w", viewerID, err)
	}
	return points, nil
}

func (r *Repo) Mine(ctx context.Context, viewerID uuid.UUID) (created, entered []FeedPoint, err error) {

	created, err = r.feed(ctx, `p.created_by = $1 AND p.status <> 'closed'`, viewerID)
	if err != nil {
		return nil, nil, fmt.Errorf("points created by %s: %w", viewerID, err)
	}
	entered, err = r.feed(ctx, `p.status = 'active' AND p.created_by IS DISTINCT FROM $1
		  AND EXISTS (SELECT 1 FROM likes_points lp
		              WHERE lp.point_id = p.id AND lp.user_id = $1)`, viewerID)
	if err != nil {
		return nil, nil, fmt.Errorf("pools entered by %s: %w", viewerID, err)
	}
	return created, entered, nil
}

func (r *Repo) PoolMembers(ctx context.Context, viewerID, pointID uuid.UUID) ([]Member, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+memberColumns+`
		FROM likes_points lp
		JOIN visible_people($1) vp ON vp = lp.user_id
		JOIN users u ON u.id = lp.user_id
		LEFT JOIN LATERAL (
			SELECT * FROM searches s
			WHERE s.user_id = lp.user_id AND s.status = 'active'
			ORDER BY s.created_at DESC, s.id DESC
			LIMIT 1
		) s ON true
		WHERE lp.point_id = $2
		  AND `+notMatchedWith("lp.user_id")+`
		ORDER BY lp.created_at DESC, lp.user_id`, viewerID, pointID)
	if err != nil {
		return nil, fmt.Errorf("pool members of point %s: %w", pointID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[memberRow])
	if err != nil {
		return nil, fmt.Errorf("pool members of point %s: %w", pointID, err)
	}
	out := make([]Member, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) CountActiveIn(ctx context.Context, zoneIDs []string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM points WHERE status = 'active' AND zone_id = ANY($1)`, zoneIDs).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active points in zones: %w", err)
	}
	return n, nil
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

func (r *Repo) feed(ctx context.Context, where string, args ...any) ([]FeedPoint, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+joinedPointColumns+`, `+personColumns+`,
			coalesce(pool.n, 0) AS pool_count,
			coalesce(resp.n, 0) AS responses,
			EXISTS (SELECT 1 FROM likes_points lp
			        WHERE lp.point_id = p.id AND lp.user_id = $1) AS in_pool
		FROM points p
		LEFT JOIN users u ON u.id = p.created_by AND p.type = 'intent_home'
		LEFT JOIN LATERAL (
			SELECT count(*) AS n
			FROM likes_points lp
			JOIN visible_people($1) vp ON vp = lp.user_id
			WHERE lp.point_id = p.id
			  AND `+notMatchedWith("lp.user_id")+`
		) pool ON p.type = 'flat'
		LEFT JOIN LATERAL (
			SELECT count(*) AS n FROM likes_users lu WHERE lu.point_id = p.id
		) resp ON p.type = 'intent_home'
		WHERE `+where+`
		ORDER BY (coalesce(pool.n, resp.n, 0) + 1)
		         / power(extract(epoch FROM now() - p.created_at) / 86400 + 2, 1.5) DESC,
		         p.created_at DESC, p.id`, args...)
	if err != nil {
		return nil, err
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[feedRow])
	if err != nil {
		return nil, err
	}
	out := make([]FeedPoint, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

// notMatchedWith is the filter searchesrepo.PoolCandidates carries too: the
// person you already matched with is out of discovery (§2.3), and a pool of a
// point is discovery as much as a zone pool is. The viewer is always $1.
func notMatchedWith(userColumn string) string {
	return `NOT EXISTS (SELECT 1 FROM matches m
			      WHERE m.a_id = least($1::uuid, ` + userColumn + `)
			        AND m.b_id = greatest($1::uuid, ` + userColumn + `))`
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
