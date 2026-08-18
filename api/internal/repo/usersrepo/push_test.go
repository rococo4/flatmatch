package usersrepo

import (
	"errors"
	"testing"
	"time"

	"flatmatch/api/internal/entity"
)

func TestMarkPushBlockedKeepsTheFirstMark(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)

	blockedAt := func() *time.Time {
		t.Helper()
		var at *time.Time
		if err := pool.QueryRow(t.Context(),
			`SELECT push_blocked_at FROM users WHERE id = $1`, user.ID).Scan(&at); err != nil {
			t.Fatalf("read push_blocked_at: %v", err)
		}
		return at
	}

	if at := blockedAt(); at != nil {
		t.Fatalf("a fresh account has push_blocked_at = %v, want nil", at)
	}

	if err := repo.MarkPushBlocked(t.Context(), user.ID); err != nil {
		t.Fatalf("MarkPushBlocked() failed: %v", err)
	}
	first := blockedAt()
	if first == nil {
		t.Fatal("MarkPushBlocked() left push_blocked_at empty")
	}

	if err := repo.MarkPushBlocked(t.Context(), user.ID); err != nil {
		t.Fatalf("second MarkPushBlocked() failed: %v", err)
	}
	if got := blockedAt(); got == nil || !got.Equal(*first) {
		t.Errorf("push_blocked_at moved to %v, want it kept at %v", got, first)
	}

	if err := repo.ClearPushBlocked(t.Context(), user.ID); err != nil {
		t.Fatalf("ClearPushBlocked() failed: %v", err)
	}
	if got := blockedAt(); got != nil {
		t.Errorf("push_blocked_at = %v after unblocking, want nil", got)
	}

	if err := repo.ClearPushBlocked(t.Context(), user.ID); err != nil {
		t.Errorf("second ClearPushBlocked() = %v, want nil", err)
	}
}

func TestByTelegramIDFindsWithoutCreating(t *testing.T) {
	repo := New(newTestPool(t))
	user := seedUser(t, repo)

	got, err := repo.ByTelegramID(t.Context(), testTelegramID)
	if err != nil {
		t.Fatalf("ByTelegramID() failed: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ByTelegramID() = %s, want %s", got.ID, user.ID)
	}

	_, err = repo.ByTelegramID(t.Context(), testTelegramID-1)
	if !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ByTelegramID() of a stranger = %v, want ErrNotFound", err)
	}
}
