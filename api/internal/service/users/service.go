package users

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/imagetype"
	"flatmatch/api/internal/logctx"
)

type Repository interface {
	EnsureByTelegramID(ctx context.Context, tg entity.TelegramIdentity) (entity.User, error)
	ByID(ctx context.Context, id uuid.UUID) (entity.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, p entity.ProfilePatch) (entity.User, error)
	AppendPhoto(ctx context.Context, id uuid.UUID, key string) (entity.User, error)
	SetPhotos(ctx context.Context, id uuid.UUID, keys []string) (entity.User, error)
	SetStatus(ctx context.Context, id uuid.UUID, status entity.Status) (entity.User, error)
	SetSourceIfEmpty(ctx context.Context, id uuid.UUID, source string) error
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

	ErrInvalidPatch  = errors.New("profile patch has invalid fields")
	ErrInvalidSource = errors.New("source code is not a valid tracking code")
	ErrIncomplete    = errors.New("profile is not filled in")

	ErrPhotoLimit        = entity.ErrPhotoLimit
	ErrLastPhoto         = errors.New("the last photo of a submitted profile cannot be removed")
	ErrInvalidPhotoIndex = errors.New("no photo under that index")
	ErrInvalidPhotoOrder = errors.New("photo order is not a permutation of the set")

	ErrUnsupportedImage = imagetype.ErrUnsupported
)

var sourceCode = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

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
	return s.repo.UpdateProfile(ctx, id, p)
}

func (s *Service) AddPhoto(ctx context.Context, id uuid.UUID, file io.ReadSeeker) (entity.User, error) {
	contentType, ext, err := imagetype.Sniff(file)
	if err != nil {
		return entity.User{}, fmt.Errorf("photo of user %s: %w", id, err)
	}

	key := photoKey(id, ext)
	if err := s.photos.Put(ctx, key, file, contentType); err != nil {
		return entity.User{}, fmt.Errorf("store photo of user %s: %w", id, err)
	}

	updated, err := s.repo.AppendPhoto(ctx, id, key)
	if err != nil {
		s.drop(ctx, id, key)
		if errors.Is(err, ErrPhotoLimit) {
			return entity.User{}, &entity.FieldsError{Err: ErrPhotoLimit, Fields: []string{"photo"}}
		}
		return entity.User{}, err
	}
	return updated, nil
}

func (s *Service) RemovePhoto(ctx context.Context, id uuid.UUID, index int) (entity.User, error) {
	user, err := s.repo.ByID(ctx, id)
	if err != nil {
		return entity.User{}, err
	}

	keys, ok := entity.RemovePhoto(user.PhotoKeys, index)
	if !ok {
		return entity.User{}, &entity.FieldsError{Err: ErrInvalidPhotoIndex, Fields: []string{"index"}}
	}
	if len(keys) == 0 && everSubmitted(user) {
		return entity.User{}, &entity.FieldsError{Err: ErrLastPhoto, Fields: []string{"photo"}}
	}

	updated, err := s.repo.SetPhotos(ctx, id, keys)
	if err != nil {
		return entity.User{}, err
	}

	s.drop(ctx, id, user.PhotoKeys[index])
	return updated, nil
}

func (s *Service) ReorderPhotos(ctx context.Context, id uuid.UUID, order []int) (entity.User, error) {
	user, err := s.repo.ByID(ctx, id)
	if err != nil {
		return entity.User{}, err
	}

	keys, ok := entity.ReorderPhotos(user.PhotoKeys, order)
	if !ok {
		return entity.User{}, &entity.FieldsError{Err: ErrInvalidPhotoOrder, Fields: []string{"order"}}
	}
	return s.repo.SetPhotos(ctx, id, keys)
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
		if user.PhotoStatus == entity.ModerationPending || user.AboutStatus == entity.ModerationPending {
			s.enqueue(ctx, user)
		}
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

func (s *Service) SetSource(ctx context.Context, id uuid.UUID, raw string) error {
	if !sourceCode.MatchString(raw) {
		return &entity.FieldsError{Err: ErrInvalidSource, Fields: []string{"source"}}
	}
	return s.repo.SetSourceIfEmpty(ctx, id, raw)
}

func (s *Service) PhotoURLs(u entity.User) []string {
	if !u.PhotoApproved() {
		return []string{}
	}
	return s.OwnPhotoURLs(u)
}

func (s *Service) OwnPhotoURLs(u entity.User) []string {
	out := make([]string, 0, len(u.PhotoKeys))
	for _, key := range u.PhotoKeys {
		out = append(out, s.photos.URL(key))
	}
	return out
}

func (s *Service) drop(ctx context.Context, id uuid.UUID, key string) {
	if err := s.photos.Delete(ctx, key); err != nil {
		logctx.From(ctx).Warn("delete a photo the profile no longer holds",
			"user_id", id, "key", key, "error", err)
	}
}

func (s *Service) enqueue(ctx context.Context, u entity.User) {
	if s.notify == nil {
		return
	}

	s.notify.UserPending(context.WithoutCancel(ctx), u)
}

func photoKey(id uuid.UUID, ext string) string {
	return fmt.Sprintf("photos/%s-%s%s", id, uuid.NewString()[:8], ext)
}

func everSubmitted(u entity.User) bool {
	return u.Status != entity.StatusDraft ||
		u.PhotoStatus != entity.ModerationPending ||
		u.AboutStatus != entity.ModerationPending
}
