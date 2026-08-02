package usersrepo

import (
	"context"
	"fmt"

	"github.com/AlekSi/pointer"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres"
)

func (r *Repo) EnsureByTelegramID(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error) {
	const query = `
		INSERT INTO users (tg_id, name, tg_username, photo_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tg_id) DO UPDATE
			SET last_seen   = now(),
			    tg_username = EXCLUDED.tg_username
		RETURNING ` + columns

	var user entity.User

	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, tg.ID, tg.Name, pointer.ToOrNil(tg.Username), pointer.ToOrNil(tg.PhotoURL))
		if err != nil {
			return err
		}

		row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[userRow])
		if err != nil {
			return err
		}

		user = row.toEntity(ctx)
		return nil
	})
	if err != nil {
		return entity.User{}, fmt.Errorf("ensure user by tg_id %d: %w", tg.ID, err)
	}
	return user, nil
}
