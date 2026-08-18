package users

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/imagetype"
)

type Repository interface {
	EnsureByTelegramID(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error)
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, p entity.ProfilePatch) (entity.User, error)
	SetPhoto(ctx context.Context, id uuid.UUID, key string) (entity.User, error)
	SetStatus(ctx context.Context, id uuid.UUID, status entity.Status) (entity.User, error)
}

type PhotoStore interface {
	Put(ctx context.Context, key string, r io.Reader, contentType string) error
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

type Notifier interface {
	UserPending(ctx context.Context, u entity.User)
}

type Service struct {
	repo   Repository
	photos PhotoStore
	notify Notifier
}

var (
	ErrBanned = errors.New("user is banned")

	ErrInvalidPatch = errors.New("profile patch has invalid fields")
	ErrIncomplete   = errors.New("profile is not filled in")

	ErrUnsupportedImage = imagetype.ErrUnsupported
)

func New(repo Repository, photos PhotoStore, notify Notifier) *Service {
	return &Service{repo: repo, photos: photos, notify: notify}
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

func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, p entity.ProfilePatch) (entity.User, error) {
	if bad := p.InvalidFields(); len(bad) > 0 {
		return entity.User{}, &entity.FieldsError{Err: ErrInvalidPatch, Fields: bad}
	}
	user, err := s.repo.UpdateProfile(ctx, id, p)
	if err != nil {
		return entity.User{}, err
	}

	if p.About != nil && user.Status == entity.StatusActive {
		s.enqueue(ctx, user)
	}
	return user, nil
}

func (s *Service) SetPhoto(ctx context.Context, id uuid.UUID, file io.ReadSeeker) (entity.User, error) {
	contentType, ext, err := imagetype.Sniff(file)
	if err != nil {
		return entity.User{}, fmt.Errorf("photo of user %s: %w", id, err)
	}

	key := fmt.Sprintf("photos/%s%s", id, ext)

	if err := s.photos.Put(ctx, key, file, contentType); err != nil {
		return entity.User{}, fmt.Errorf("store photo of user %s: %w", id, err)
	}
	user, err := s.repo.SetPhoto(ctx, id, key)
	if err != nil {
		return entity.User{}, err
	}

	if user.Status == entity.StatusActive {
		s.enqueue(ctx, user)
	}
	return user, nil
}

func (s *Service) Submit(ctx context.Context, id uuid.UUID) (entity.User, error) {
	user, err := s.repo.ByID(ctx, id)
	if err != nil {
		return entity.User{}, err
	}

	missing := user.MissingFields()
	if len(missing) > 0 {
		return entity.User{}, &entity.FieldsError{Err: ErrIncomplete, Fields: missing}
	}

	if user.Status == entity.StatusActive {
		return user, nil
	}
	submitted, err := s.repo.SetStatus(ctx, id, entity.StatusPending)
	if err != nil {
		return entity.User{}, err
	}

	if user.Status != entity.StatusPending {
		s.enqueue(ctx, submitted)
	}
	return submitted, nil
}

func (s *Service) PhotoURL(u entity.User) string {
	if !u.PhotoApproved() {
		return ""
	}
	return s.photos.URL(u.PhotoKey)
}

func (s *Service) OwnPhotoURL(u entity.User) string {
	return s.photos.URL(u.PhotoKey)
}

func (s *Service) enqueue(ctx context.Context, u entity.User) {
	if s.notify == nil {
		return
	}

	s.notify.UserPending(context.WithoutCancel(ctx), u)
}
