package users

import (
	"context"
	"errors"
	"testing"

	"flatmatch/api/internal/entity"
)

// fakeRepo records what the service asked for and returns what the test wants.
// Hand-written rather than generated: the interface has one method, and a fake
// that can be read in ten seconds beats a mock that cannot.
type fakeRepo struct {
	got  entity.TelegramIdentity
	user entity.User
	err  error
}

func (f *fakeRepo) EnsureByTelegramID(_ context.Context, tg entity.TelegramIdentity) (entity.User, error) {
	f.got = tg
	return f.user, f.err
}

func TestEnsureFromTelegramPassesIdentityThrough(t *testing.T) {
	repo := &fakeRepo{user: entity.User{TelegramID: 42, Status: entity.StatusDraft}}

	identity := entity.TelegramIdentity{ID: 42, Name: "Арсений", Username: "arsenii"}
	user, err := New(repo).EnsureFromTelegram(t.Context(), identity)
	if err != nil {
		t.Fatalf("EnsureFromTelegram() failed: %v", err)
	}

	if repo.got != identity {
		t.Errorf("repository received %+v, want %+v", repo.got, identity)
	}
	if user.TelegramID != 42 {
		t.Errorf("TelegramID = %d, want 42", user.TelegramID)
	}
}

func TestEnsureFromTelegramSubstitutesAnEmptyName(t *testing.T) {
	// Telegram guarantees a first name, but an empty one would leave the
	// greeting blank and violate the NOT NULL on name.
	repo := &fakeRepo{user: entity.User{Status: entity.StatusDraft}}

	if _, err := New(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42}); err != nil {
		t.Fatalf("EnsureFromTelegram() failed: %v", err)
	}
	if got, want := repo.got.Name, "user42"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
}

func TestEnsureFromTelegramRejectsBannedAccounts(t *testing.T) {
	repo := &fakeRepo{user: entity.User{TelegramID: 42, Status: entity.StatusBanned}}

	_, err := New(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42, Name: "Mallory"})
	if !errors.Is(err, ErrBanned) {
		t.Fatalf("EnsureFromTelegram() error = %v, want ErrBanned", err)
	}
}

func TestEnsureFromTelegramRejectsIdentityWithoutID(t *testing.T) {
	repo := &fakeRepo{}

	if _, err := New(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{Name: "Никто"}); err == nil {
		t.Fatal("EnsureFromTelegram() accepted an identity with no id")
	}
	if repo.got.Name != "" {
		t.Error("the repository was called despite an invalid identity")
	}
}

func TestEnsureFromTelegramPropagatesRepositoryFailure(t *testing.T) {
	wantErr := errors.New("connection refused")
	repo := &fakeRepo{err: wantErr}

	_, err := New(repo).EnsureFromTelegram(t.Context(), entity.TelegramIdentity{ID: 42, Name: "Арсений"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnsureFromTelegram() error = %v, want it to wrap %v", err, wantErr)
	}
}
