package usersrepo

import (
	"context"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
)

type userRow struct {
	ID          uuid.UUID `db:"id"`
	TelegramID  int64     `db:"tg_id"`
	Name        string    `db:"name"`
	Username    *string   `db:"tg_username"`
	PhotoURL    *string   `db:"photo_url"`
	Status      string    `db:"status"`
	PhotoStatus string    `db:"photo_status"`
	LastSeen    time.Time `db:"last_seen"`
	CreatedAt   time.Time `db:"created_at"`
}

const columns = `id, tg_id, name, tg_username, photo_url, status, photo_status, last_seen, created_at`

func (r userRow) toEntity(ctx context.Context) entity.User {
	status := entity.ParseStatus(r.Status)
	if status == entity.StatusUnknown {
		logctx.From(ctx).Warn("unreadable user status, hiding the account",
			"user_id", r.ID, "stored_status", r.Status)
	}

	photoStatus := entity.ParsePhotoStatus(r.PhotoStatus)
	if photoStatus == entity.PhotoUnknown {
		logctx.From(ctx).Warn("unreadable photo status",
			"user_id", r.ID, "stored_photo_status", r.PhotoStatus)
	}

	return entity.User{
		ID:          r.ID,
		TelegramID:  r.TelegramID,
		Name:        r.Name,
		Username:    pointer.Get(r.Username),
		PhotoURL:    pointer.Get(r.PhotoURL),
		Status:      status,
		PhotoStatus: photoStatus,
		LastSeen:    r.LastSeen,
		CreatedAt:   r.CreatedAt,
	}
}
