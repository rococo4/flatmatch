package searchesrepo

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

func (r *Repo) Create(ctx context.Context, userID uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	var out entity.Search
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockAndCheckSlots(ctx, tx, userID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO searches (user_id, zone_id, price_min, price_max,
				rooms_min, rooms_max, move_in, term, has_stuff)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING `+searchColumns,
			userID, p.ZoneID, p.PriceMin, p.PriceMax, p.RoomsMin, p.RoomsMax,
			p.MoveIn, termArg(p.Term), stuffArg(p.HasStuff))
		if err != nil {
			return err
		}
		out, err = collectSearch(rows)
		return err
	})
	if err != nil {
		return entity.Search{}, fmt.Errorf("create search for user %s: %w", userID, err)
	}
	return out, nil
}

func (r *Repo) ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	search, err := r.one(ctx, `SELECT `+searchColumns+` FROM searches WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return entity.Search{}, fmt.Errorf("get search %s: %w", id, err)
	}
	return search, nil
}

func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+` FROM searches
		WHERE user_id = $1
		ORDER BY (status = 'active') DESC, created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list searches of user %s: %w", userID, err)
	}
	out, err := collectSearches(rows)
	if err != nil {
		return nil, fmt.Errorf("list searches of user %s: %w", userID, err)
	}
	return out, nil
}

func (r *Repo) PoolCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]entity.PoolCandidate, error) {
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
		ORDER BY s.created_at DESC`, viewerID, zoneIDs)
	if err != nil {
		return nil, fmt.Errorf("pool candidates for viewer %s: %w", viewerID, err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[candidateRow])
	if err != nil {
		return nil, fmt.Errorf("pool candidates for viewer %s: %w", viewerID, err)
	}
	out := make([]entity.PoolCandidate, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) HasActiveIn(ctx context.Context, userID uuid.UUID, zoneIDs []string) (bool, error) {
	var one int
	err := r.pool.QueryRow(ctx, `
		SELECT 1 FROM searches
		WHERE user_id = $1 AND status = 'active' AND zone_id = ANY($2)
		LIMIT 1`, userID, zoneIDs).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("active searches of %s in zones: %w", userID, err)
	}
	return true, nil
}

func (r *Repo) UpdateParams(ctx context.Context, userID, id uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	search, err := r.oneEditable(ctx, userID, id, `
		UPDATE searches SET zone_id = $3, price_min = $4, price_max = $5,
			rooms_min = $6, rooms_max = $7, move_in = $8, term = $9,
			has_stuff = $10, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'closed'
		RETURNING `+searchColumns,
		id, userID, p.ZoneID, p.PriceMin, p.PriceMax, p.RoomsMin, p.RoomsMax,
		p.MoveIn, termArg(p.Term), stuffArg(p.HasStuff))
	if err != nil {
		return entity.Search{}, fmt.Errorf("update params of search %s: %w", id, err)
	}
	return search, nil
}

func (r *Repo) Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	search, err := r.oneEditable(ctx, userID, id, `
		UPDATE searches SET status = 'closed', close_reason = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'closed'
		RETURNING `+searchColumns, id, userID, string(reason))
	if err != nil {
		return entity.Search{}, fmt.Errorf("close search %s: %w", id, err)
	}
	return search, nil
}

func (r *Repo) Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error) {
	var out entity.Search
	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := lockAndCheckSlots(ctx, tx, userID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			UPDATE searches SET status = 'active', close_reason = NULL,
				pool_seen_at = now(), last_active_at = now(), ttl_asked_at = NULL,
				updated_at = now()
			WHERE id = $1 AND user_id = $2 AND status <> 'active'
			RETURNING `+searchColumns, id, userID)
		if err != nil {
			return err
		}
		out, err = collectSearch(rows)
		return err
	})
	if err != nil {
		return entity.Search{}, fmt.Errorf("reactivate search %s: %w", id, err)
	}
	return out, nil
}

func (r *Repo) MarkPoolSeen(ctx context.Context, userID uuid.UUID, zoneID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE searches SET pool_seen_at = now()
		WHERE user_id = $1 AND zone_id = $2 AND status = 'active'`, userID, zoneID)
	if err != nil {
		return fmt.Errorf("mark pool seen for user %s zone %s: %w", userID, zoneID, err)
	}
	return nil
}

func (r *Repo) Touch(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE searches SET last_active_at = now()
		WHERE user_id = $1 AND status = 'active'
		  AND last_active_at < now() - interval '1 hour'`, userID)
	if err != nil {
		return fmt.Errorf("touch searches of user %s: %w", userID, err)
	}
	return nil
}

func (r *Repo) StaleForTTL(ctx context.Context, quiet, afterMatch time.Duration) ([]entity.Search, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+searchColumns+`
		FROM searches
		WHERE status = 'active'
		  AND (ttl_asked_at IS NULL OR ttl_asked_at < now() - make_interval(secs => $1))
		  AND (
		      last_active_at < now() - make_interval(secs => $1)
		      OR EXISTS (
		          SELECT 1 FROM matches m
		          WHERE searches.user_id IN (m.a_id, m.b_id)
		            AND m.created_at > searches.last_active_at
		            AND m.created_at < now() - make_interval(secs => $2)))
		ORDER BY last_active_at, id`, quiet.Seconds(), afterMatch.Seconds())
	if err != nil {
		return nil, fmt.Errorf("stale searches: %w", err)
	}
	out, err := collectSearches(rows)
	if err != nil {
		return nil, fmt.Errorf("stale searches: %w", err)
	}
	return out, nil
}

func (r *Repo) ClearTTLAsked(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE searches SET last_active_at = now(), ttl_asked_at = NULL
		WHERE id = $1 AND status = 'active'`, id)
	if err != nil {
		return fmt.Errorf("clear the ttl question of search %s: %w", id, err)
	}
	return nil
}

func (r *Repo) MarkTTLAsked(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE searches SET ttl_asked_at = now() WHERE id = $1 AND status = 'active'`, id)
	if err != nil {
		return fmt.Errorf("mark search %s asked: %w", id, err)
	}
	return nil
}

func (r *Repo) PauseUnanswered(ctx context.Context, olderThan time.Duration) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE searches s SET status = 'paused', updated_at = now()
		WHERE s.status = 'active'
		  AND s.ttl_asked_at IS NOT NULL
		  AND s.ttl_asked_at < now() - make_interval(secs => $1)
		  AND s.last_active_at < s.ttl_asked_at
		  AND EXISTS (
		      SELECT 1 FROM notifications n
		      WHERE n.type = 'still_searching'
		        AND n.user_id = s.user_id
		        AND n.payload->>'search_id' = s.id::text
		        AND n.sent_at IS NOT NULL
		        AND n.sent_at >= s.ttl_asked_at - interval '1 minute')
		RETURNING s.id`, olderThan.Seconds())
	if err != nil {
		return nil, fmt.Errorf("pause unanswered searches: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("pause unanswered searches: %w", err)
	}
	return ids, nil
}

func (r *Repo) one(ctx context.Context, query string, args ...any) (entity.Search, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return entity.Search{}, err
	}
	return collectSearch(rows)
}

func (r *Repo) oneEditable(ctx context.Context, userID, id uuid.UUID, query string, args ...any) (entity.Search, error) {
	search, err := r.one(ctx, query, args...)
	if !errors.Is(err, entity.ErrNotFound) {
		return search, err
	}

	var status string
	err = r.pool.QueryRow(ctx,
		`SELECT status FROM searches WHERE id = $1 AND user_id = $2`, id, userID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return entity.Search{}, entity.ErrNotFound
	case err != nil:
		return entity.Search{}, err
	case status == string(entity.SearchClosed):
		return entity.Search{}, entity.ErrSearchClosed
	default:

		return entity.Search{}, entity.ErrNotFound
	}
}

func lockAndCheckSlots(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	var locked uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ErrNotFound
		}
		return err
	}

	var active int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM searches
		WHERE user_id = $1 AND status = 'active'`, userID).Scan(&active); err != nil {
		return err
	}
	if active >= entity.MaxActiveSearches {
		return entity.ErrSearchLimit
	}
	return nil
}

func collectSearch(rows pgx.Rows) (entity.Search, error) {
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[searchRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Search{}, entity.ErrNotFound
		}
		return entity.Search{}, err
	}
	return row.toEntity(), nil
}

func collectSearches(rows pgx.Rows) ([]entity.Search, error) {
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[searchRow])
	if err != nil {
		return nil, err
	}
	out := make([]entity.Search, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func termArg(t *entity.Term) *string {
	if t == nil {
		return nil
	}
	return pointer.To(string(*t))
}

func stuffArg(chips []entity.StuffChip) []string {
	out := make([]string, 0, len(chips))
	for _, c := range chips {
		out = append(out, string(c))
	}
	return out
}
