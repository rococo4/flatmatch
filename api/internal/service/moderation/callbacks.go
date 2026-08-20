package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
)

const callbackPrefix = "mod"

const (
	subjectUser   = "u"
	subjectPoint  = "p"
	subjectReport = "r"
)

const (
	opOK    = "ok"
	opPhoto = "photo"
	opAbout = "about"
	opNo    = "no"
	opBan   = "ban"
	opSkip  = "skip"
)

const (
	ackApproved      = "✓ Одобрено"
	ackPhotoRejected = "✕ Фото отклонено"
	ackAboutRejected = "✕ Текст отклонён"
	ackRejected      = "✕ Отклонено"
	ackBanned        = "✓ Забанен"
	ackBannedAlready = "уже забанен"
	ackSkipped       = "✓ Пропущено"
	ackDone          = "уже обработано"
)

const rejectedReason = "rejected"

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
	case subjectReport:
		return s.reportVerdict(ctx, parts[2], id)
	default:
		return "", fmt.Errorf("moderation callback %q: unknown subject", data)
	}
}

func (s *Service) userVerdict(ctx context.Context, op string, id uuid.UUID) (string, error) {
	u, err := s.users.ByID(ctx, id)
	if err != nil {
		return "", err
	}

	if u.Status == entity.StatusBanned {
		return ackDone, nil
	}

	var (
		ack   string
		notif entity.NotificationType
	)
	switch op {
	case opOK:
		if !inQueue(u) {
			return ackDone, nil
		}

		_, err = s.users.SetModeration(ctx, id, pointer.To(entity.StatusActive),
			pointer.To(entity.ModerationOK), pointer.To(entity.ModerationOK))
		ack, notif = ackApproved, entity.NotifProfileOK
	case opPhoto:
		if u.PhotoStatus != entity.ModerationPending {
			return ackDone, nil
		}
		_, err = s.users.SetModeration(ctx, id, backToDraft(u), pointer.To(entity.ModerationRejected), nil)
		ack, notif = ackPhotoRejected, entity.NotifPhotoRejected
	case opAbout:
		if u.AboutStatus != entity.ModerationPending {
			return ackDone, nil
		}
		_, err = s.users.SetModeration(ctx, id, backToDraft(u), nil, pointer.To(entity.ModerationRejected))
		ack, notif = ackAboutRejected, entity.NotifAboutRejected
	default:
		return "", fmt.Errorf("moderation callback for user %s: unknown verdict %q", id, op)
	}
	if err != nil {
		return "", err
	}
	if s.pushes != nil {
		s.pushes.ProfileVerdict(ctx, id, notif)
	}
	return ack, nil
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
	s.pushVerdict(ctx, id, op == opOK)
	if op == opNo {
		return ackRejected, nil
	}
	return ackApproved, nil
}

func (s *Service) reportVerdict(ctx context.Context, op string, id uuid.UUID) (string, error) {
	report, err := s.reports.ByID(ctx, id)
	if err != nil {
		return "", err
	}
	if report.Resolved() {
		return ackDone, nil
	}

	switch op {
	case opBan:
		return s.ban(ctx, report)
	case opSkip:
		if _, err := s.reports.Resolve(ctx, report.ID, entity.ReportSkipped); err != nil {
			return "", err
		}
		return ackSkipped, nil
	default:
		return "", fmt.Errorf("moderation callback for report %s: unknown verdict %q", id, op)
	}
}

func (s *Service) ban(ctx context.Context, report entity.Report) (string, error) {
	target, err := s.users.ByID(ctx, report.TargetID)
	if err != nil {
		return "", err
	}

	ack := ackBannedAlready
	if target.Status != entity.StatusBanned {
		if _, err := s.users.SetStatus(ctx, target.ID, entity.StatusBanned); err != nil {
			return "", err
		}
		if err := s.queue.FailAllPending(ctx, target.ID); err != nil {
			return "", err
		}
		ack = ackBanned
	}

	if _, err := s.reports.Resolve(ctx, report.ID, entity.ReportBanned); err != nil {
		return "", err
	}
	return ack, nil
}

func (s *Service) pushVerdict(ctx context.Context, id uuid.UUID, ok bool) {
	if s.pushes == nil {
		return
	}
	point, err := s.points.ByID(ctx, id)
	if err != nil {
		logctx.From(ctx).Warn("read the point a verdict was given to", "point", id, "error", err)
		return
	}
	s.pushes.PointVerdict(ctx, point, ok)
}

func inQueue(u entity.User) bool {
	switch u.Status {
	case entity.StatusPending:
		return true
	case entity.StatusActive:
		return u.PhotoStatus == entity.ModerationPending || u.AboutStatus == entity.ModerationPending
	default:
		return false
	}
}

func backToDraft(u entity.User) *entity.Status {
	if u.Status == entity.StatusActive {
		return nil
	}
	return pointer.To(entity.StatusDraft)
}

func callback(subject, op string, id uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s:%s", callbackPrefix, subject, op, id)
}
