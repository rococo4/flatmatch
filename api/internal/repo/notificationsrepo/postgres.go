package notificationsrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"flatmatch/api/internal/entity"
)

func (r *Repo) Insert(ctx context.Context, n entity.Notification) (bool, error) {
	tag, err := r.pool.Exec(ctx, insertQuery, insertArgs(n)...)
	if err != nil {
		return false, fmt.Errorf("insert %s notification for user %s: %w", n.Type, n.UserID, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repo) InsertMany(ctx context.Context, ns []entity.Notification) (int, error) {
	if len(ns) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, n := range ns {
		batch.Queue(insertQuery, insertArgs(n)...)
	}
	res := r.pool.SendBatch(ctx, batch)

	var inserted int
	var failed error
	for range ns {
		tag, err := res.Exec()
		if err != nil {
			failed = err
			break
		}
		inserted += int(tag.RowsAffected())
	}
	if err := res.Close(); err != nil && failed == nil {
		failed = err
	}
	if failed != nil {
		return 0, fmt.Errorf("insert %d notifications: %w", len(ns), failed)
	}
	return inserted, nil
}

func (r *Repo) Pending(ctx context.Context, limit int) ([]entity.Notification, error) {
	return r.pending(ctx, limit, false)
}

func (r *Repo) PendingPersonal(ctx context.Context, limit int) ([]entity.Notification, error) {
	return r.pending(ctx, limit, true)
}

func (r *Repo) pending(ctx context.Context, limit int, personalOnly bool) ([]entity.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+notificationColumns+`
		FROM notifications n
		JOIN users u ON u.id = n.user_id
		WHERE n.sent_at IS NULL AND n.failed_at IS NULL
		  AND u.push_blocked_at IS NULL
		  AND u.status <> 'banned'
		  AND (NOT $2 OR n.type <> ALL($3))
		ORDER BY n.created_at, n.id
		LIMIT $1`, limit, personalOnly, aggregatedTypes())
	if err != nil {
		return nil, fmt.Errorf("read the push queue: %w", err)
	}
	raw, err := pgx.CollectRows(rows, pgx.RowToStructByName[notificationRow])
	if err != nil {
		return nil, fmt.Errorf("read the push queue: %w", err)
	}
	out := make([]entity.Notification, 0, len(raw))
	for _, row := range raw {
		out = append(out, row.toEntity())
	}
	return out, nil
}

func (r *Repo) MarkSent(ctx context.Context, ids []uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET sent_at = $2
		WHERE id = ANY($1) AND sent_at IS NULL`, ids, at)
	if err != nil {
		return fmt.Errorf("mark %d notifications sent: %w", len(ids), err)
	}
	return nil
}

func (r *Repo) MarkFailed(ctx context.Context, ids []uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET failed_at = now()
		WHERE id = ANY($1) AND sent_at IS NULL AND failed_at IS NULL`, ids)
	if err != nil {
		return fmt.Errorf("mark %d notifications failed: %w", len(ids), err)
	}
	return nil
}

func (r *Repo) Bump(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET attempts = attempts + 1 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("bump attempts of notification %s: %w", id, err)
	}
	return nil
}

func (r *Repo) LastSentAt(ctx context.Context, userID uuid.UUID, typ entity.NotificationType, contextKey string) (*time.Time, error) {
	var at *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT max(sent_at) FROM notifications
		WHERE user_id = $1 AND type = $2 AND context_key IS NOT DISTINCT FROM $3`,
		userID, string(typ), pointer.ToOrNil(contextKey)).Scan(&at)
	if err != nil {
		return nil, fmt.Errorf("last %s push to user %s: %w", typ, userID, err)
	}
	return at, nil
}

func (r *Repo) SentPersonalCount(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications
		WHERE user_id = $1 AND sent_at >= $2 AND type = ANY($3)`,
		userID, since, personalTypes()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count personal pushes to user %s: %w", userID, err)
	}
	return n, nil
}

func (r *Repo) FailAllPending(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications SET failed_at = now()
		WHERE user_id = $1 AND sent_at IS NULL AND failed_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("fail the queue of user %s: %w", userID, err)
	}
	return nil
}

func (r *Repo) LastLikedTail(ctx context.Context, userID uuid.UUID, since time.Time) (*TailMessage, error) {
	var tail TailMessage
	err := r.pool.QueryRow(ctx, `
		SELECT id, (payload->>'message_id')::int, coalesce((payload->>'tail_count')::int, 1)
		FROM notifications
		WHERE user_id = $1 AND type = $2 AND sent_at >= $3
		  AND payload->>'message_id' IS NOT NULL
		ORDER BY sent_at DESC
		LIMIT 1`, userID, string(entity.NotifLiked), since).Scan(&tail.ID, &tail.MessageID, &tail.Count)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("last liked tail of user %s: %w", userID, err)
	}
	return &tail, nil
}

func (r *Repo) SetTail(ctx context.Context, id uuid.UUID, messageID, count int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notifications
		SET payload = payload || jsonb_build_object('message_id', $2::int, 'tail_count', $3::int)
		WHERE id = $1`, id, messageID, count)
	if err != nil {
		return fmt.Errorf("set the tail of notification %s: %w", id, err)
	}
	return nil
}
