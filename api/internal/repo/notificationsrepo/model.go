package notificationsrepo

import (
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const notificationColumns = `n.id, n.user_id, n.type, n.actor_key, n.context_key,
	n.payload, n.created_at, n.sent_at, n.failed_at, n.attempts`

// Заблокировавший бота не получает даже строки (§3 спеки): проверка стоит
// внутри вставки — отдельным чтением адресата фан-аут ошибался бы на гонке.
const insertQuery = `
	INSERT INTO notifications (user_id, type, actor_key, context_key, payload)
	SELECT $1::uuid, $2::text, $3::text, $4::text, $5::jsonb
	WHERE EXISTS (
	    SELECT 1 FROM users WHERE id = $1::uuid AND push_blocked_at IS NULL)
	ON CONFLICT DO NOTHING`

type TailMessage struct {
	ID        uuid.UUID // строка-носитель хвоста
	MessageID int
	Count     int
}

type notificationRow struct {
	ID         uuid.UUID      `db:"id"`
	UserID     uuid.UUID      `db:"user_id"`
	Type       string         `db:"type"`
	ActorKey   *string        `db:"actor_key"`
	ContextKey *string        `db:"context_key"`
	Payload    map[string]any `db:"payload"`
	CreatedAt  time.Time      `db:"created_at"`
	SentAt     *time.Time     `db:"sent_at"`
	FailedAt   *time.Time     `db:"failed_at"`
	Attempts   int            `db:"attempts"`
}

func (r notificationRow) toEntity() entity.Notification {
	return entity.Notification{
		ID:         r.ID,
		UserID:     r.UserID,
		Type:       entity.ParseNotificationType(r.Type),
		ActorKey:   pointer.Get(r.ActorKey),
		ContextKey: pointer.Get(r.ContextKey),
		Payload:    r.Payload,
		CreatedAt:  r.CreatedAt,
		SentAt:     r.SentAt,
		FailedAt:   r.FailedAt,
		Attempts:   r.Attempts,
	}
}

func insertArgs(n entity.Notification) []any {
	// Пустая карта вместо nil: nil уехал бы в jsonb как json null, а по нему
	// payload->>'…' уже не спросишь.
	payload := n.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return []any{
		n.UserID, string(n.Type),
		pointer.ToOrNil(n.ActorKey), pointer.ToOrNil(n.ContextKey),
		payload,
	}
}

func personalTypes() []string {
	types := entity.PersonalPushTypes()
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, string(t))
	}
	return out
}
