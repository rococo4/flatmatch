package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

const callbackPrefix = "mod"

const (
	subjectUser  = "u"
	subjectPoint = "p"
)

const (
	opOK    = "ok"
	opPhoto = "photo"
	opAbout = "about"
	opNo    = "no"
)

// Тап по кнопке из старого сообщения — обычное дело в чате, где очередь
// пролистывают сверху вниз.
const (
	ackOK   = "✓"
	ackDone = "уже обработано"
)

// rejectedReason marks a point closed by the moderator, so «Присмотренные»
// могут отличить отклонённую квартиру от закрытой самим человеком (§6).
const rejectedReason = "rejected"

// HandleCallback applies one inline-button tap. Ack — это текст всплывающего
// ответа Telegram; ошибка означает, что вердикт НЕ применён.
func (s *Service) HandleCallback(ctx context.Context, data string) (string, error) {
	parts := strings.Split(data, ":")
	if len(parts) != 4 || parts[0] != callbackPrefix {
		return "", fmt.Errorf("moderation callback %q: unexpected shape", data)
	}
	id, err := uuid.Parse(parts[3])
	if err != nil {
		return "", fmt.Errorf("moderation callback %q: %w", data, err)
	}

	switch parts[1] {
	case subjectUser:
		return s.userVerdict(ctx, parts[2], id)
	case subjectPoint:
		return s.pointVerdict(ctx, parts[2], id)
	default:
		return "", fmt.Errorf("moderation callback %q: unknown subject", data)
	}
}

func (s *Service) userVerdict(ctx context.Context, op string, id uuid.UUID) (string, error) {
	u, err := s.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	// Бан — исход разбора жалобы, а не инструмент модерации контента (§5):
	// кнопка из очереди его не отменяет.
	if u.Status == entity.StatusBanned {
		return ackDone, nil
	}

	switch op {
	case opOK:
		if u.Status != entity.StatusPending &&
			u.PhotoStatus != entity.ModerationPending && u.AboutStatus != entity.ModerationPending {
			return ackDone, nil
		}
		// После аппрува человек проявляется везде разом (§5).
		_, err = s.users.SetModeration(ctx, id, pointer.To(entity.StatusActive),
			pointer.To(entity.ModerationOK), pointer.To(entity.ModerationOK))
	case opPhoto:
		if u.PhotoStatus != entity.ModerationPending {
			return ackDone, nil
		}
		_, err = s.users.SetModeration(ctx, id, backToDraft(u), pointer.To(entity.ModerationRejected), nil)
	case opAbout:
		if u.AboutStatus != entity.ModerationPending {
			return ackDone, nil
		}
		_, err = s.users.SetModeration(ctx, id, backToDraft(u), nil, pointer.To(entity.ModerationRejected))
	default:
		return "", fmt.Errorf("moderation callback for user %s: unknown verdict %q", id, op)
	}
	if err != nil {
		return "", err
	}
	return ackOK, nil
}

func (s *Service) pointVerdict(ctx context.Context, op string, id uuid.UUID) (string, error) {
	var (
		status entity.PointStatus
		reason *string
	)
	switch op {
	case opOK:
		status = entity.PointActive
	case opNo:
		// CHECK в схеме держит пару: закрытая точка всегда с причиной.
		status, reason = entity.PointClosed, pointer.To(rejectedReason)
	default:
		return "", fmt.Errorf("moderation callback for point %s: unknown verdict %q", id, op)
	}

	changed, err := s.points.SetStatus(ctx, id, status, reason)
	if err != nil {
		return "", err
	}
	if !changed {
		return ackDone, nil
	}
	return ackOK, nil
}

// backToDraft: реджект возвращает анкету в draft, потому что фото обязательно и
// без него человека не показать; у одобренной статус не трогаем — правка не
// должна выкидывать её из пулов (§5, две ветки реджекта).
func backToDraft(u entity.User) *entity.Status {
	if u.Status == entity.StatusActive {
		return nil
	}
	return pointer.To(entity.StatusDraft)
}

func callback(subject, op string, id uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s:%s", callbackPrefix, subject, op, id)
}
