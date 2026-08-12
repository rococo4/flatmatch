package searchesrepo

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
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[searchRow])
	if err != nil {
		return nil, fmt.Errorf("list searches of user %s: %w", userID, err)
	}
	out := make([]entity.Search, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) PoolCandidates(ctx context.Context, viewerID uuid.UUID, zoneIDs []string) ([]entity.PoolCandidate, error) {
	// Смэтченные не discovery: вы уже нашли друг друга, в пулах и счётчиках
	// такая карточка — шум. Фильтр именно здесь, а не в visible_people: пара
	// остаётся видимой друг другу (детали мэтча, контакты).
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

// HasActiveIn отвечает половину отношения «мы увиделись» (§2.3): есть ли у
// человека активный поиск в одной из этих зон. Пересечение зон считает
// справочник у вызывающего — БД про станции не знает.
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
				pool_seen_at = now(), updated_at = now()
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
		// Someone edited the row between the two statements; nothing to report
		// beyond «the update you asked for did not happen».
		return entity.Search{}, entity.ErrNotFound
	}
}

// lockAndCheckSlots serialises slot accounting: concurrent creations queue on
// the user row, so two of them cannot both see the same free slot and take it.
// The lock has to sit on users rather than on the user's searches — locking
// the searches would lock nothing at all for a user who has none yet, which is
// exactly the case where a fourth search slips through.
//
// It also rests on READ COMMITTED, the default level these transactions run at,
// and on the count being a SEPARATE statement from the lock. Under READ
// COMMITTED every statement takes its own snapshot, so the count issued AFTER
// the lock wait ends sees the rows the transaction that held the lock has just
// committed. Merge the two into one statement, or raise the level to
// REPEATABLE READ, and the count would read the snapshot taken before the wait
// — the concurrent search would be invisible, both transactions would see the
// same free slot, and the ≤3 limit (§2.1) would break silently, without an
// error anywhere.
func lockAndCheckSlots(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	// FOR NO KEY UPDATE, not FOR UPDATE: it excludes other slot checks but
	// still lets unrelated rows reference this user by foreign key meanwhile.
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
