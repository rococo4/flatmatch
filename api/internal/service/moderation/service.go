package moderation

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
	"flatmatch/api/internal/service/zones"
)

type Sender interface {
	SendModerationPhoto(ctx context.Context, chatID int64, photoPath, caption string, buttons [][2]string) (int, error)
}

type UsersMod interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	SetStatus(ctx context.Context, id uuid.UUID, status entity.Status) (entity.User, error)
	SetModeration(ctx context.Context, id uuid.UUID, status *entity.Status, photo, about *entity.ModerationStatus) (entity.User, error)
	PendingModeration(ctx context.Context) ([]entity.User, error)
}

type PointsMod interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
	SetStatus(ctx context.Context, id uuid.UUID, status entity.PointStatus, closeReason *string) (bool, error)
	CloseByCreator(ctx context.Context, userID uuid.UUID) (int, error)
	PendingModeration(ctx context.Context) ([]entity.Point, error)
}

type Reports interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Report, error)
	Resolve(ctx context.Context, id uuid.UUID, verdict entity.ReportVerdict) (bool, error)
	Unresolved(ctx context.Context) ([]entity.Report, error)
}

type Pushes interface {
	ProfileVerdict(ctx context.Context, userID uuid.UUID, typ entity.NotificationType)
	PointVerdict(ctx context.Context, point entity.Point, ok bool)
}

type Queue interface {
	FailAllPending(ctx context.Context, userID uuid.UUID) error
}

type PhotoURLs interface {
	Path(key string) (string, error)
}

type Service struct {
	bot     Sender
	chatID  int64
	users   UsersMod
	points  PointsMod
	reports Reports
	photos  PhotoURLs
	dir     *zones.Directory
	pushes  Pushes
	queue   Queue
}

func New(bot Sender, chatID int64, users UsersMod, points PointsMod, reports Reports,
	photos PhotoURLs, dir *zones.Directory, pushes Pushes, queue Queue) *Service {
	return &Service{bot: bot, chatID: chatID, users: users, points: points, reports: reports,
		photos: photos, dir: dir, pushes: pushes, queue: queue}
}

func (s *Service) UserPending(ctx context.Context, u entity.User) {
	s.send(ctx, subjectUser, u.ID, u.PhotoKey, userCaption(u), [][2]string{
		{"ок", callback(subjectUser, opOK, u.ID)},
		{"фото не ок", callback(subjectUser, opPhoto, u.ID)},
		{"текст не ок", callback(subjectUser, opAbout, u.ID)},
	})
}

func (s *Service) PointPending(ctx context.Context, p entity.Point) {
	s.send(ctx, subjectPoint, p.ID, coverPhoto(p), s.pointCaption(p), [][2]string{
		{"ок", callback(subjectPoint, opOK, p.ID)},
		{"отклонить", callback(subjectPoint, opNo, p.ID)},
	})
}

func (s *Service) ReportFiled(ctx context.Context, r entity.Report, reporter, target entity.User) {
	s.send(ctx, subjectReport, r.ID, target.PhotoKey, s.reportCaption(ctx, r, reporter, target), [][2]string{
		{"Забанить", callback(subjectReport, opBan, r.ID)},
		{"Пропустить", callback(subjectReport, opSkip, r.ID)},
	})
}

func (s *Service) SendQueue(ctx context.Context) error {
	users, err := s.users.PendingModeration(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		s.UserPending(ctx, u)
	}

	points, err := s.points.PendingModeration(ctx)
	if err != nil {
		return err
	}
	for _, p := range points {
		s.PointPending(ctx, p)
	}

	reports, err := s.reports.Unresolved(ctx)
	if err != nil {
		return err
	}
	for _, r := range reports {
		s.resendReport(ctx, r)
	}
	return nil
}

func (s *Service) resendReport(ctx context.Context, r entity.Report) {
	reporter, err := s.users.ByID(ctx, r.ReporterID)
	if err != nil {
		logctx.From(ctx).Warn("read the author of a complaint", "report", r.ID, "error", err)
		return
	}
	target, err := s.users.ByID(ctx, r.TargetID)
	if err != nil {
		logctx.From(ctx).Warn("read the target of a complaint", "report", r.ID, "error", err)
		return
	}
	s.ReportFiled(ctx, r, reporter, target)
}

func (s *Service) send(ctx context.Context, subject string, id uuid.UUID, photoKey, caption string, buttons [][2]string) {
	log := logctx.From(ctx).With("subject", subject, "id", id)
	if s.chatID == 0 {
		log.Warn("moderation chat is not configured, the queue only piles up in the database")
		return
	}
	if photoKey == "" {
		log.Warn("nothing to show the moderator: no photo")
		return
	}

	path, err := s.photos.Path(photoKey)
	if err != nil {
		log.Warn("resolve photo for moderation", "error", err)
		return
	}
	if _, err := s.bot.SendModerationPhoto(ctx, s.chatID, path, caption, buttons); err != nil {
		log.Warn("send to the moderation chat", "error", err)
	}
}

func (s *Service) pointCaption(p entity.Point) string {
	station := p.ZoneID
	if zone, ok := s.dir.Get(p.ZoneID); ok {
		station = zone.Name
	}

	if p.Type == entity.PointIntentHome {
		return fmt.Sprintf("%s\nдоля %d ₽ · м. %s",
			stageLabel(p.Stage), pointer.Get(p.PriceShare), station)
	}

	caption := fmt.Sprintf("%s\n%d ₽ · м. %s",
		pointer.Get(p.URLNorm), pointer.Get(p.PriceTotal), station)
	if p.Rooms != nil {
		caption += fmt.Sprintf(" · %d комн.", *p.Rooms)
	}
	return caption
}

func (s *Service) reportCaption(ctx context.Context, r entity.Report, reporter, target entity.User) string {
	caption := fmt.Sprintf("Жалоба: %s → %s (%d) · %s",
		reporter.Name, target.Name, target.Profile.Age, entity.ReportReasonLabel(r.Reason))

	if line := s.reportContext(ctx, r.ContextKey); line != "" {
		caption += "\nКонтекст: " + line
	}

	about := target.Profile.About
	if about == "" {
		about = "—"
	}
	return caption + "\nО себе цели: " + about
}

func (s *Service) reportContext(ctx context.Context, key string) string {
	kind, value, ok := strings.Cut(key, ":")
	if !ok {
		return ""
	}

	switch kind {
	case entity.ReportContextZone:
		zone, ok := s.dir.Get(value)
		if !ok {
			return ""
		}
		return "пул " + zone.NameGen
	case entity.ReportContextPoint:
		return s.pointContext(ctx, value)
	case entity.ReportContextMatch:
		return "мэтч"
	default:
		return ""
	}
}

func (s *Service) pointContext(ctx context.Context, raw string) string {
	id, err := uuid.Parse(raw)
	if err != nil {
		return ""
	}
	point, err := s.points.ByID(ctx, id)
	if err != nil {
		logctx.From(ctx).Warn("read the point a complaint points at", "point", id, "error", err)
		return ""
	}

	station := point.ZoneID
	if zone, ok := s.dir.Get(point.ZoneID); ok {
		station = zone.Name
	}
	if point.Type == entity.PointIntentHome {
		return "комната у метро " + station
	}
	return "квартира у метро " + station
}

func userCaption(u entity.User) string {
	return fmt.Sprintf("%s, %d\n%s\nстатус: %s", u.Name, u.Profile.Age, u.Profile.About, statusLine(u))
}

func statusLine(u entity.User) string {
	if u.Status != entity.StatusActive {
		return "первичная анкета"
	}
	photo := u.PhotoStatus == entity.ModerationPending
	about := u.AboutStatus == entity.ModerationPending
	switch {
	case photo && about:
		return "правка фото и «о себе»"
	case photo:
		return "правка фото"
	case about:
		return "правка «о себе»"
	default:
		return "правка"
	}
}

func stageLabel(stage *entity.PointStage) string {
	if pointer.Get(stage) == entity.StageMovingIn {
		return "заезжает · ищет со-съёмщика со старта"
	}
	return "живёт здесь"
}

func coverPhoto(p entity.Point) string {
	if len(p.PhotoKeys) == 0 {
		return ""
	}
	return p.PhotoKeys[0]
}
