package usersrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

// Visibility needs several accounts at once, so these tests get their own
// tg_id range instead of the single fixed account the rest of the package uses.
const visibilityTGBase int64 = -994000000

func cleanVisibilityUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	clean := func() {
		if _, err := pool.Exec(context.Background(),
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`, visibilityTGBase); err != nil {
			t.Errorf("clean up test users: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)
}

// seedVisibilityUser inserts a user directly: repo methods cannot produce
// arbitrary status/gender combinations, and those are the whole subject here.
func seedVisibilityUser(t *testing.T, pool *pgxpool.Pool, n int64, status, gender, pref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', $2, $3, $4) RETURNING id`,
		visibilityTGBase-n, status, gender, pref).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestVisibleByID(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanVisibilityUsers(t, pool)
	repo := New(pool)
	viewer := seedVisibilityUser(t, pool, 1, "active", "male", "any")

	visible := seedVisibilityUser(t, pool, 2, "active", "female", "any")
	got, err := repo.VisibleByID(t.Context(), viewer, visible)
	if err != nil {
		t.Fatalf("VisibleByID() failed: %v", err)
	}
	if got.ID != visible || got.Name != "test" || got.Status != entity.StatusActive {
		t.Errorf("VisibleByID() returned %+v, want the visible person", got)
	}

	tests := []struct {
		name string
		id   uuid.UUID
	}{
		// Пол обоюден: она никого, кроме женщин, не хочет — значит её нет (§4).
		{"gender mismatch", seedVisibilityUser(t, pool, 3, "active", "female", "same")},
		{"not approved yet", seedVisibilityUser(t, pool, 4, "draft", "female", "any")},
		{"banned", seedVisibilityUser(t, pool, 5, "banned", "female", "any")},
		// Свой профиль читают через ByID: видимость — про других.
		{"myself", viewer},
		{"nobody", uuid.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := repo.VisibleByID(t.Context(), viewer, tt.id); !errors.Is(err, entity.ErrNotFound) {
				t.Errorf("VisibleByID() = %v, want ErrNotFound", err)
			}
		})
	}
}
