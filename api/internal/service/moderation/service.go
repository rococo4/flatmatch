package moderation

import (
	"context"
	"fmt"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/logctx"
	"flatmatch/api/internal/service/zones"
)

// Sender is the moderation chat's transport. Фото уходит файлом, а не ссылкой:
// на стенде картинки недоступны из интернета, а Telegram ходит за URL сам.
// Кнопка — пара (текст, callback_data).
type Sender interface {
	SendModerationPhoto(ctx context.Context, chatID int64, photoPath, caption string, buttons [][2]string) (int, error)
}

type UsersMod interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	SetModeration(ctx context.Context, id uuid.UUID, status *entity.Status, photo, about *entity.ModerationStatus) (entity.User, error)
	PendingModeration(ctx context.Context) ([]entity.User, error)
}

type PointsMod interface {
	ByID(ctx context.Context, id uuid.UUID) (entity.Point, error)
	SetStatus(ctx context.Context, id uuid.UUID, status entity.PointStatus, closeReason *string) (bool, error)
	PendingModeration(ctx context.Context) ([]entity.Point, error)
}

// Pushes — лог уведомлений (§7): вердикт человек узнаёт из бота, и пуш «загрузи
// другое» — единственная дорога назад из реджекта фото (§5). nil молча ничего
// не делает.
type Pushes interface {
	ProfileVerdict(ctx context.Context, userID uuid.UUID, typ entity.NotificationType)
	PointVerdict(ctx context.Context, point entity.Point, ok bool)
}

// PhotoURLs resolves a stored key to a local file: в базе лежит ключ, адрес
// собирает хранилище.
type PhotoURLs interface {
	Path(key string) (string, error)
}

type Service struct {
	bot    Sender
	chatID int64
	users  UsersMod
	points PointsMod
	photos PhotoURLs
	dir    *zones.Directory
	pushes Pushes
}

func New(bot Sender, chatID int64, users UsersMod, points PointsMod, photos PhotoURLs,
	dir *zones.Directory, pushes Pushes) *Service {
	return &Service{bot: bot, chatID: chatID, users: users, points: points,
		photos: photos, dir: dir, pushes: pushes}
}

// UserPending shows a questionnaire to the moderator: фото и «о себе» — ровно
// то, что он смотрит (§5).
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

// SendQueue re-sends everything still waiting — догон после рестарта, когда
// сообщения с кнопками потерялись или бот стоял (§3 дизайна).
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
	return nil
}

// send is best-effort by design: очередь живёт в БД, а не в чате, поэтому
// неотправленное сообщение — повод для warn и команды /queue, но не ошибка
// наверх, которая отменила бы создание точки или отправку анкеты.
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

func userCaption(u entity.User) string {
	return fmt.Sprintf("%s, %d\n%s\nстатус: %s", u.Name, u.Profile.Age, u.Profile.About, statusLine(u))
}

// statusLine tells the moderator, что именно он смотрит: у одобренной анкеты в
// очереди на перепроверку уходят только фото и «о себе», остальное меняется
// молча (§5).
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

// coverPhoto — кадр для модератора: обложка точки. Остальные фото он при
// сомнении откроет в приложении, слать альбом в чат — шум.
func coverPhoto(p entity.Point) string {
	if len(p.PhotoKeys) == 0 {
		return ""
	}
	return p.PhotoKeys[0]
}
