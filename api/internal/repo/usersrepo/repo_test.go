package usersrepo

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
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
	// Nothing has been uploaded yet: §4 has people choose a photo themselves,
	// and a Telegram avatar is never substituted for one.
	if user.PhotoKey != "" {
		t.Errorf("PhotoKey = %q, want it empty on a fresh account", user.PhotoKey)
	}
	// A fresh account has not answered a single question yet — it is a draft,
	// not something waiting for a moderator (§4, §5).
	if got, want := user.Status, entity.StatusDraft; got != want {
		t.Errorf("Status = %q, want %q", got, want)
	}
	if got, want := user.PhotoStatus, entity.ModerationPending; got != want {
		t.Errorf("PhotoStatus = %q, want %q", got, want)
	}
	if got, want := user.OnboardingStep, 0; got != want {
		t.Errorf("OnboardingStep = %d, want %d", got, want)
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

	// Not every Telegram account has a @username.
	user, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{ID: testTelegramID, Name: "Без ника"})
	if err != nil {
		t.Fatalf("EnsureByTelegramID() failed: %v", err)
	}
	if user.Username != "" {
		t.Errorf("NULL did not come back as \"\": username=%q", user.Username)
	}

	// "" must reach the column as NULL, not as an empty string: a UNIQUE index
	// on tg_username in a later step would otherwise treat every account
	// without a username as a duplicate.
	var usernameIsNull bool
	err = pool.QueryRow(ctx,
		`SELECT tg_username IS NULL FROM users WHERE tg_id = $1`,
		testTelegramID,
	).Scan(&usernameIsNull)
	if err != nil {
		t.Fatalf("read back nullability: %v", err)
	}
	if !usernameIsNull {
		t.Error("an empty username was stored as '' instead of NULL")
	}
}

// seedUser creates the account the profile tests then modify.
func seedUser(t *testing.T, repo *Repo) entity.User {
	t.Helper()

	user, err := repo.EnsureByTelegramID(t.Context(), entity.TelegramIdentity{
		ID: testTelegramID, Name: "Арсений",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return user
}

func TestUpdateProfileLeavesUnmentionedAnswersAlone(t *testing.T) {
	// The point of COALESCE: a request carrying one screen's answer must not
	// blank out the eleven screens already filled in.
	repo := New(newTestPool(t))
	user := seedUser(t, repo)
	ctx := t.Context()

	if _, err := repo.UpdateProfile(ctx, user.ID, entity.ProfilePatch{
		Schedule:       pointer.To(entity.ScheduleOwl),
		OnboardingStep: pointer.To(2),
	}); err != nil {
		t.Fatalf("first UpdateProfile() failed: %v", err)
	}

	got, err := repo.UpdateProfile(ctx, user.ID, entity.ProfilePatch{
		Smoking:        pointer.To(entity.SmokingNo),
		OnboardingStep: pointer.To(3),
	})
	if err != nil {
		t.Fatalf("second UpdateProfile() failed: %v", err)
	}

	if got.Profile.Schedule != entity.ScheduleOwl {
		t.Errorf("Schedule = %q, want it left at %q", got.Profile.Schedule, entity.ScheduleOwl)
	}
	if got.Profile.Smoking != entity.SmokingNo {
		t.Errorf("Smoking = %q, want %q", got.Profile.Smoking, entity.SmokingNo)
	}
	if got.OnboardingStep != 3 {
		t.Errorf("OnboardingStep = %d, want 3", got.OnboardingStep)
	}
}

func TestUpdateProfileSendsRewrittenAboutBackToModeration(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)
	ctx := t.Context()

	// Approved once…
	if _, err := pool.Exec(ctx,
		`UPDATE users SET about = 'Старый текст', about_status = 'ok' WHERE id = $1`, user.ID,
	); err != nil {
		t.Fatalf("approve about: %v", err)
	}

	// …then rewritten, which has to put it back in the queue (§5).
	got, err := repo.UpdateProfile(ctx, user.ID, entity.ProfilePatch{
		About: pointer.To("Новый текст"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if got.AboutStatus != entity.ModerationPending {
		t.Errorf("AboutStatus = %q, want %q", got.AboutStatus, entity.ModerationPending)
	}

	// A patch that does not touch «о себе» must leave the verdict where it is,
	// or every answered question would drag the text back through moderation.
	if _, err := pool.Exec(ctx, `UPDATE users SET about_status = 'ok' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("re-approve about: %v", err)
	}
	got, err = repo.UpdateProfile(ctx, user.ID, entity.ProfilePatch{
		Noise: pointer.To(entity.NoiseNormal),
	})
	if err != nil {
		t.Fatalf("second UpdateProfile() failed: %v", err)
	}
	if got.AboutStatus != entity.ModerationOK {
		t.Errorf("AboutStatus = %q, want it left at %q", got.AboutStatus, entity.ModerationOK)
	}
}

func TestSetPhotoQueuesTheNewFile(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`UPDATE users SET photo_key = 'photos/old.jpg', photo_status = 'ok' WHERE id = $1`, user.ID,
	); err != nil {
		t.Fatalf("approve photo: %v", err)
	}

	got, err := repo.SetPhoto(ctx, user.ID, "photos/new.jpg")
	if err != nil {
		t.Fatalf("SetPhoto() failed: %v", err)
	}
	if got.PhotoKey != "photos/new.jpg" {
		t.Errorf("PhotoKey = %q, want the new one", got.PhotoKey)
	}
	// Replacing an approved photo has to un-approve it, or an unreviewed face
	// would go straight into the pools.
	if got.PhotoStatus != entity.ModerationPending {
		t.Errorf("PhotoStatus = %q, want %q", got.PhotoStatus, entity.ModerationPending)
	}
	if got.PhotoApproved() {
		t.Error("a freshly uploaded photo must not count as approved")
	}
}

func TestApprovedPhotoCannotExistWithoutAFile(t *testing.T) {
	// The photo_ok_needs_a_file CHECK. Without it a row could claim a moderator
	// cleared a photo that is not there, and the app would render a placeholder
	// as if it had been checked.
	pool := newTestPool(t)
	user := seedUser(t, New(pool))

	_, err := pool.Exec(t.Context(),
		`UPDATE users SET photo_status = 'ok', photo_key = NULL WHERE id = $1`, user.ID)
	if err == nil {
		t.Fatal("the database accepted an approved photo with no file")
	}
	if !strings.Contains(err.Error(), "photo_ok_needs_a_file") {
		t.Errorf("rejected by something else: %v", err)
	}
}

func TestByIDReportsAMissingUser(t *testing.T) {
	repo := New(newTestPool(t))

	if _, err := repo.ByID(t.Context(), uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Fatalf("ByID() error = %v, want ErrNotFound", err)
	}
}

func TestSetStatusMovesADraftToModeration(t *testing.T) {
	repo := New(newTestPool(t))
	user := seedUser(t, repo)

	got, err := repo.SetStatus(t.Context(), user.ID, entity.StatusPending)
	if err != nil {
		t.Fatalf("SetStatus() failed: %v", err)
	}
	if got.Status != entity.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, entity.StatusPending)
	}
	if got.Visible() {
		t.Error("an account waiting for a moderator must not be visible")
	}
}
