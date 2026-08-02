package usersrepo

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres"
	"flatmatch/api/migrations"
)

// testTelegramID is outside the range Telegram assigns to real accounts, so a
// leftover row can never collide with a real user.
const testTelegramID int64 = -999000001

// newTestPool connects to the database named by TEST_DATABASE_URL, applies the
// migrations, and hands back a pool.
//
// Without that variable the test skips rather than fails: `go test ./...` has
// to stay runnable with nothing but a Go toolchain, and this is the only test
// in the module that needs a server.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set — see README, «Тесты, которым нужна база»")
	}

	ctx := t.Context()
	log := slog.New(slog.DiscardHandler)
	if err := postgres.Migrate(ctx, dsn, migrations.FS, log); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)

	// Leave no trace either side of the test, so reruns are identical.
	clean := func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM users WHERE tg_id = $1`, testTelegramID); err != nil {
			t.Errorf("clean up test user: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)

	return pool
}

func TestEnsureByTelegramIDCreatesAccountOnFirstLaunch(t *testing.T) {
	repo := New(newTestPool(t))

	user, err := repo.EnsureByTelegramID(t.Context(), entity.TelegramIdentity{
		ID:       testTelegramID,
		Name:     "Арсений",
		Username: "arsenii",
		PhotoURL: "https://t.me/i/userpic/320/arsenii.jpg",
	})
	if err != nil {
		t.Fatalf("EnsureByTelegramID() failed: %v", err)
	}

	if user.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Errorf("ID was not generated, got %q", user.ID)
	}
	if got, want := user.TelegramID, testTelegramID; got != want {
		t.Errorf("TelegramID = %d, want %d", got, want)
	}
	if got, want := user.Name, "Арсений"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := user.Username, "arsenii"; got != want {
		t.Errorf("Username = %q, want %q", got, want)
	}
	if got, want := user.PhotoURL, "https://t.me/i/userpic/320/arsenii.jpg"; got != want {
		t.Errorf("PhotoURL = %q, want %q", got, want)
	}
	// A fresh account has not answered a single question yet — it is a draft,
	// not something waiting for a moderator (§4, §5).
	if got, want := user.Status, entity.StatusDraft; got != want {
		t.Errorf("Status = %q, want %q", got, want)
	}
	if got, want := user.PhotoStatus, entity.PhotoPending; got != want {
		t.Errorf("PhotoStatus = %q, want %q", got, want)
	}
	if user.Visible() {
		t.Error("a pending account must not be visible")
	}
}

func TestEnsureByTelegramIDIsIdempotent(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	ctx := t.Context()

	first, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{
		ID: testTelegramID, Name: "Арсений", Username: "arsenii",
	})
	if err != nil {
		t.Fatalf("first EnsureByTelegramID() failed: %v", err)
	}

	// last_seen has second-ish resolution in practice; make sure the clock has
	// visibly moved before asserting it advanced.
	time.Sleep(10 * time.Millisecond)

	// A later launch reports whatever Telegram currently says. The name may by
	// then differ from what the user set inside the app.
	second, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{
		ID: testTelegramID, Name: "Что-то из Telegram", Username: "arsenii_new",
	})
	if err != nil {
		t.Fatalf("second EnsureByTelegramID() failed: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("a second launch created a different account: %s then %s", first.ID, second.ID)
	}
	// The name is the user's to change, not Telegram's to overwrite.
	if got, want := second.Name, "Арсений"; got != want {
		t.Errorf("Name = %q, want it left at %q", got, want)
	}
	// The username is Telegram's fact and must stay current, or the
	// «Написать @username» button after a match points at nobody (§6).
	if got, want := second.Username, "arsenii_new"; got != want {
		t.Errorf("Username = %q, want %q", got, want)
	}
	if !second.LastSeen.After(first.LastSeen) {
		t.Errorf("LastSeen did not advance: %s then %s", first.LastSeen, second.LastSeen)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed: %s then %s", first.CreatedAt, second.CreatedAt)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE tg_id = $1`, testTelegramID).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("found %d rows for one Telegram account, want 1", count)
	}
}

func TestEnsureByTelegramIDStoresAbsentFieldsAsNull(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	ctx := t.Context()

	// Not every Telegram account has a @username or a photo.
	user, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{ID: testTelegramID, Name: "Без ника"})
	if err != nil {
		t.Fatalf("EnsureByTelegramID() failed: %v", err)
	}
	if user.Username != "" || user.PhotoURL != "" {
		t.Errorf("NULL did not come back as \"\": username=%q photo=%q", user.Username, user.PhotoURL)
	}

	// "" must reach the column as NULL, not as an empty string: a UNIQUE index
	// on tg_username in a later step would otherwise treat every account
	// without a username as a duplicate.
	var usernameIsNull, photoIsNull bool
	err = pool.QueryRow(ctx,
		`SELECT tg_username IS NULL, photo_url IS NULL FROM users WHERE tg_id = $1`,
		testTelegramID,
	).Scan(&usernameIsNull, &photoIsNull)
	if err != nil {
		t.Fatalf("read back nullability: %v", err)
	}
	if !usernameIsNull || !photoIsNull {
		t.Errorf("empty values were stored as '' instead of NULL: username null=%v photo null=%v", usernameIsNull, photoIsNull)
	}
}
