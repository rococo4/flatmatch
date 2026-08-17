package likes

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/zones"
)

type Repository interface {
	Create(ctx context.Context, fromID, toID uuid.UUID, lc likesrepo.LikeContext) (entity.LikeResult, error)
	Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error)
	Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error)
}

// Pushes — лог уведомлений (§7): «тебя лайкнул N» адресату и «у тебя мэтч»
// обоим. nil молча ничего не делает — пуш не часть лайка.
type Pushes interface {
	Liked(ctx context.Context, toID, fromID uuid.UUID, fromName string, fromGender entity.Gender)
	Matched(ctx context.Context, match entity.Match)
}

type Service struct {
	repo   Repository
	dir    *zones.Directory
	pushes Pushes
}

var ErrInvalidParams = errors.New("like params are invalid")

func New(repo Repository, dir *zones.Directory, pushes Pushes) *Service {
	return &Service{repo: repo, dir: dir, pushes: pushes}
}

// Like принимает лайкающего целиком: имя и род нужны пушу адресату (§7), и
// анкету, только что прочитанную аутентификацией, незачем читать второй раз.
func (s *Service) Like(ctx context.Context, from entity.User, toID uuid.UUID, zoneID string, pointID *uuid.UUID) (entity.LikeResult, error) {
	// Один лайк — один контекст: мэтч потом рассказывает, ГДЕ пересеклись, и на
	// два места сразу этот рассказ не делится (§6).
	if zoneID != "" && pointID != nil {
		return entity.LikeResult{}, &entity.FieldsError{
			Err: ErrInvalidParams, Fields: []string{"zone_id", "point_id"}}
	}
	if _, ok := s.dir.Get(zoneID); zoneID != "" && !ok {
		return entity.LikeResult{}, &entity.FieldsError{Err: ErrInvalidParams, Fields: []string{"zone_id"}}
	}
	// Лайк без зоны контекста тратящим быть не может: пустой список зон валит
	// гейт поиска в ErrNeedSearch, ответным лайкам гейт и так не мешает.
	var contextZones []string
	if zoneID != "" {
		contextZones = s.dir.Intersecting(zoneID)
	}
	// Точку валидирует транзакция: активность, тип и отношение к ней читаются
	// там же, где решается трата слота.
	res, err := s.repo.Create(ctx, from.ID, toID,
		likesrepo.LikeContext{ZoneID: zoneID, ContextZones: contextZones, PointID: pointID})
	if err != nil {
		return entity.LikeResult{}, err
	}
	s.announce(ctx, from, toID, res)
	return res, nil
}

func (s *Service) announce(ctx context.Context, from entity.User, toID uuid.UUID, res entity.LikeResult) {
	if s.pushes == nil {
		return
	}
	// Повторный тап ничего не создал, а ответный лайк адресату не новость: он
	// сам его и вызвал, и следом идёт «у тебя мэтч».
	if !res.Already && res.Like.Spent {
		s.pushes.Liked(ctx, toID, from.ID, from.Name, from.Profile.Gender)
	}
	// Гонка встречных лайков приводит сюда обе стороны — строка всё равно одна:
	// её актор сам мэтч, и вторую отбивает уникальный индекс.
	if res.Match != nil {
		s.pushes.Matched(ctx, *res.Match)
	}
}

func (s *Service) Incoming(ctx context.Context, userID uuid.UUID) ([]likesrepo.IncomingLike, error) {
	return s.repo.Incoming(ctx, userID)
}

func (s *Service) Budget(ctx context.Context, userID uuid.UUID) (entity.LikeBudget, error) {
	return s.repo.Budget(ctx, userID)
}
