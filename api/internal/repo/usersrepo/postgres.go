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

func (r *Repo) ByTelegramID(ctx context.Context, tgID int64) (entity.User, error) {
	const query = `SELECT ` + columns + ` FROM users WHERE tg_id = $1`

	user, err := r.one(ctx, query, tgID)
	if err != nil {
		return entity.User{}, fmt.Errorf("get user by tg_id %d: %w", tgID, err)
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

func (r *Repo) VisibleByID(ctx context.Context, viewerID, id uuid.UUID) (entity.User, error) {
	const query = `
		SELECT ` + columns + `
		FROM users u
		JOIN visible_people($1) vp ON vp = u.id
		WHERE u.id = $2`

	user, err := r.one(ctx, query, viewerID, id)
	if err != nil {
		return entity.User{}, fmt.Errorf("get user %s visible to %s: %w", id, viewerID, err)
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

func (r *Repo) SetModeration(ctx context.Context, id uuid.UUID, status *entity.Status, photo, about *entity.ModerationStatus) (entity.User, error) {
	const query = `
		UPDATE users SET
			status       = COALESCE($2, status),
			photo_status = COALESCE($3, photo_status),
			about_status = COALESCE($4, about_status)
		WHERE id = $1
		RETURNING ` + columns

	user, err := r.one(ctx, query, id, enumArg(status), enumArg(photo), enumArg(about))
	if err != nil {
		return entity.User{}, fmt.Errorf("set moderation of user %s: %w", id, err)
	}
	return user, nil
}

func (r *Repo) PendingModeration(ctx context.Context) ([]entity.User, error) {
	const query = `
		SELECT ` + columns + `
		FROM users
		WHERE status = 'pending'
		   OR (status = 'active' AND (photo_status = 'pending' OR about_status = 'pending'))
		ORDER BY created_at, id`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list users pending moderation: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[userRow])
	if err != nil {
		return nil, fmt.Errorf("list users pending moderation: %w", err)
	}
	out := make([]entity.User, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity(ctx))
	}
	return out, nil
}

func (r *Repo) MarkPushBlocked(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET push_blocked_at = now()
		WHERE id = $1 AND push_blocked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("mark pushes blocked for user %s: %w", id, err)
	}
	return nil
}

func (r *Repo) ClearPushBlocked(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET push_blocked_at = NULL WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("clear the push block of user %s: %w", id, err)
	}
	return nil
}

func (r *Repo) SetSourceIfEmpty(ctx context.Context, id uuid.UUID, source string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET source = $2 WHERE id = $1 AND source IS NULL`, id, source)
	if err != nil {
		return fmt.Errorf("set source of user %s: %w", id, err)
	}
	return nil
}

func (r *Repo) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO blocks (blocker_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("block %s for %s: %w", blockedID, blockerID, err)
	}
	return nil
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
