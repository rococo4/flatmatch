package users

import (
	"context"
	"errors"
	"fmt"

	"flatmatch/api/internal/entity"
)

var ErrBanned = errors.New("user is banned")

type Repository interface {
	EnsureByTelegramID(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error)
}

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureFromTelegram(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error) {
	if tg.ID == 0 {
		return entity.User{}, errors.New("telegram identity has no id")
	}
	if tg.Name == "" {
		tg.Name = fmt.Sprintf("user%d", tg.ID)
	}

	user, err := s.repo.EnsureByTelegramID(ctx, tg)
	if err != nil {
		return entity.User{}, err
	}
	if user.Status == entity.StatusBanned {
		return entity.User{}, ErrBanned
	}
	return user, nil
}
