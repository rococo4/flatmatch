package usersrepo

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

func (r *Repo) EnsureByTelegramID(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error) {
	const query = `
		INSERT INTO users (tg_id, name, tg_username)
		VALUES ($1, $2, $3)
		ON CONFLICT (tg_id) DO UPDATE
			SET last_seen   = now(),
			    tg_username = EXCLUDED.tg_username
		RETURNING ` + columns

	user, err := r.one(ctx, query, tg.ID, tg.Name, pointer.ToOrNil(tg.Username))
	if err != nil {
		return entity.User{}, fmt.Errorf("ensure user by tg_id %d: %w", tg.ID, err)
	}
	return user, nil
}

func (r *Repo) ByID(ctx context.Context, id uuid.UUID) (entity.User, error) {
	const query = `SELECT ` + columns + ` FROM users WHERE id = $1`

	user, err := r.one(ctx, query, id)
	if err != nil {
		return entity.User{}, fmt.Errorf("get user %s: %w", id, err)
	}
	return user, nil
}

func (r *Repo) UpdateProfile(ctx context.Context, id uuid.UUID, p entity.ProfilePatch) (entity.User, error) {
	const query = `
		UPDATE users SET
			age             = COALESCE($2, age),
			gender          = COALESCE($3, gender),
			gender_pref     = COALESCE($4, gender_pref),
			occupation      = COALESCE($5, occupation),
			schedule        = COALESCE($6, schedule),
			smoking         = COALESCE($7, smoking),
			pets_own        = COALESCE($8, pets_own),
			pets_ok         = COALESCE($9, pets_ok),
			noise           = COALESCE($10, noise),
			guests          = COALESCE($11, guests),
			cleaning        = COALESCE($12, cleaning),
			alcohol         = COALESCE($13, alcohol),
			expenses        = COALESCE($14, expenses),
			about           = COALESCE($15, about),
			onboarding_step = COALESCE($16, onboarding_step),
			about_status    = CASE WHEN $15::text IS NULL THEN about_status ELSE 'pending' END
		WHERE id = $1
		RETURNING ` + columns

	user, err := r.one(ctx, query, id,
		p.Age,
		enumArg(p.Gender), enumArg(p.GenderPref), enumArg(p.Occupation),
		enumArg(p.Schedule), enumArg(p.Smoking), enumArg(p.PetsOwn),
		enumArg(p.PetsOK), enumArg(p.Noise), enumArg(p.Guests),
		enumArg(p.Cleaning), enumArg(p.Alcohol), enumArg(p.Expenses),
		p.About,
		p.OnboardingStep,
	)
	if err != nil {
		return entity.User{}, fmt.Errorf("update profile of user %s: %w", id, err)
	}
	return user, nil
}

func (r *Repo) SetPhoto(ctx context.Context, id uuid.UUID, key string) (entity.User, error) {
	const query = `
		UPDATE users
		SET photo_key = $2, photo_status = 'pending'
		WHERE id = $1
		RETURNING ` + columns

	user, err := r.one(ctx, query, id, key)
	if err != nil {
		return entity.User{}, fmt.Errorf("set photo of user %s: %w", id, err)
	}
	return user, nil
}

func (r *Repo) SetStatus(ctx context.Context, id uuid.UUID, status entity.Status) (entity.User, error) {
	const query = `UPDATE users SET status = $2 WHERE id = $1 RETURNING ` + columns

	user, err := r.one(ctx, query, id, string(status))
	if err != nil {
		return entity.User{}, fmt.Errorf("set status of user %s to %s: %w", id, status, err)
	}
	return user, nil
}

func (r *Repo) one(ctx context.Context, query string, args ...any) (entity.User, error) {
	var user entity.User

	err := postgres.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}

		row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[userRow])
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return entity.ErrNotFound
			}
			return err
		}

		user = row.toEntity(ctx)
		return nil
	})
	if err != nil {
		return entity.User{}, err
	}
	return user, nil
}

func enumArg[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	return pointer.To(string(*v))
}
