package usersrepo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
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

const testTelegramID int64 = -999000001

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

	if len(user.PhotoKeys) != 0 {
		t.Errorf("PhotoKeys = %v, want them empty on a fresh account", user.PhotoKeys)
	}

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

	time.Sleep(10 * time.Millisecond)

	second, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{
		ID: testTelegramID, Name: "Что-то из Telegram", Username: "arsenii_new",
	})
	if err != nil {
		t.Fatalf("second EnsureByTelegramID() failed: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("a second launch created a different account: %s then %s", first.ID, second.ID)
	}

	if got, want := second.Name, "Арсений"; got != want {
		t.Errorf("Name = %q, want it left at %q", got, want)
	}

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

	user, err := repo.EnsureByTelegramID(ctx, entity.TelegramIdentity{ID: testTelegramID, Name: "Без ника"})
	if err != nil {
		t.Fatalf("EnsureByTelegramID() failed: %v", err)
	}
	if user.Username != "" {
		t.Errorf("NULL did not come back as \"\": username=%q", user.Username)
	}

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

	if _, err := pool.Exec(ctx,
		`UPDATE users SET about = 'Старый текст', about_status = 'ok' WHERE id = $1`, user.ID,
	); err != nil {
		t.Fatalf("approve about: %v", err)
	}

	got, err := repo.UpdateProfile(ctx, user.ID, entity.ProfilePatch{
		About: pointer.To("Новый текст"),
	})
	if err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if got.AboutStatus != entity.ModerationPending {
		t.Errorf("AboutStatus = %q, want %q", got.AboutStatus, entity.ModerationPending)
	}

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

func TestAppendPhotoAddsToTheEndAndQueuesTheSet(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`UPDATE users SET photo_keys = ARRAY['photos/old.jpg'], photo_status = 'ok' WHERE id = $1`, user.ID,
	); err != nil {
		t.Fatalf("approve photo: %v", err)
	}

	first, err := repo.AppendPhoto(ctx, user.ID, "photos/new.jpg")
	if err != nil {
		t.Fatalf("AppendPhoto() failed: %v", err)
	}
	if !slices.Equal(first.PhotoKeys, []string{"photos/old.jpg", "photos/new.jpg"}) {
		t.Errorf("PhotoKeys = %v, want the new key at the end", first.PhotoKeys)
	}
	if first.PhotoStatus != entity.ModerationPending {
		t.Errorf("PhotoStatus = %q, want %q", first.PhotoStatus, entity.ModerationPending)
	}
	if first.PhotoApproved() {
		t.Error("a freshly uploaded photo must not count as approved")
	}

	second, err := repo.AppendPhoto(ctx, user.ID, "photos/third.jpg")
	if err != nil {
		t.Fatalf("second AppendPhoto() failed: %v", err)
	}
	want := []string{"photos/old.jpg", "photos/new.jpg", "photos/third.jpg"}
	if !slices.Equal(second.PhotoKeys, want) {
		t.Errorf("PhotoKeys = %v, want %v — neither upload may overwrite the other", second.PhotoKeys, want)
	}
}

func TestAppendPhotoStopsAtTheCapWithoutTouchingTheRow(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)
	ctx := t.Context()

	full := make([]string, 0, entity.MaxUserPhotos)
	for i := range entity.MaxUserPhotos {
		full = append(full, fmt.Sprintf("photos/%d.jpg", i))
	}
	if _, err := pool.Exec(ctx,
		`UPDATE users SET photo_keys = $2, photo_status = 'ok' WHERE id = $1`, user.ID, full,
	); err != nil {
		t.Fatalf("fill the photo set: %v", err)
	}

	if _, err := repo.AppendPhoto(ctx, user.ID, "photos/sixth.jpg"); !errors.Is(err, entity.ErrPhotoLimit) {
		t.Fatalf("AppendPhoto() error = %v, want ErrPhotoLimit", err)
	}

	got, err := repo.ByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if !slices.Equal(got.PhotoKeys, full) {
		t.Errorf("PhotoKeys = %v, want the row left alone", got.PhotoKeys)
	}
	if got.PhotoStatus != entity.ModerationOK {
		t.Errorf("PhotoStatus = %q, want the refused upload to leave the verdict alone", got.PhotoStatus)
	}

	if _, err := repo.AppendPhoto(ctx, uuid.New(), "photos/x.jpg"); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("AppendPhoto() of nobody = %v, want ErrNotFound", err)
	}
}

func TestSetPhotosLeavesTheVerdictAlone(t *testing.T) {
	pool := newTestPool(t)
	repo := New(pool)
	user := seedUser(t, repo)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		UPDATE users SET photo_keys = ARRAY['photos/a.jpg', 'photos/b.jpg'], photo_status = 'ok'
		WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("approve photos: %v", err)
	}

	got, err := repo.SetPhotos(ctx, user.ID, []string{"photos/b.jpg", "photos/a.jpg"})
	if err != nil {
		t.Fatalf("SetPhotos() failed: %v", err)
	}
	if !slices.Equal(got.PhotoKeys, []string{"photos/b.jpg", "photos/a.jpg"}) {
		t.Errorf("PhotoKeys = %v, want the new order", got.PhotoKeys)
	}
	if got.PhotoStatus != entity.ModerationOK {
		t.Errorf("PhotoStatus = %q, want %q — reordering approved photos shows nothing new",
			got.PhotoStatus, entity.ModerationOK)
	}
}

func TestApprovedPhotoCannotExistWithoutAFile(t *testing.T) {
	pool := newTestPool(t)
	user := seedUser(t, New(pool))

	_, err := pool.Exec(t.Context(),
		`UPDATE users SET photo_status = 'ok', photo_keys = '{}' WHERE id = $1`, user.ID)
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
