package usersrepo

import (
	"testing"
	"time"
)

// Бот заблокирован — адресат выпадает из очереди пушей, и первая отметка не
// перезаписывается: важно, когда это случилось.
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
}
