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

func TestVisibleBetween(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanVisibilityUsers(t, pool)
	repo := New(pool)

	viewer := seedVisibilityUser(t, pool, 11, "active", "male", "any")

	tests := []struct {
		name string
		id   uuid.UUID
		want bool
	}{
		{"visible", seedVisibilityUser(t, pool, 12, "active", "female", "any"), true},
		// Пол обоюден: она хочет только женщин — её нет ни в пулах, ни в пушах (§4).
		{"gender mismatch", seedVisibilityUser(t, pool, 13, "active", "female", "same"), false},
		{"not approved yet", seedVisibilityUser(t, pool, 14, "pending", "male", "any"), false},
		{"banned", seedVisibilityUser(t, pool, 15, "banned", "male", "any"), false},
		{"myself", viewer, false},
		{"nobody", uuid.New(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.VisibleBetween(t.Context(), viewer, tt.id)
			if err != nil {
				t.Fatalf("VisibleBetween() failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("VisibleBetween() = %v, want %v", got, tt.want)
			}
		})
	}

	// Блок обоюден, и вопрос «видит ли a b» отвечает одинаково с любой стороны.
	blocked := seedVisibilityUser(t, pool, 16, "active", "male", "any")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, blocked, viewer); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			`DELETE FROM blocks WHERE blocker_id = $1`, blocked); err != nil {
			t.Errorf("clean up the block: %v", err)
		}
	})

	for _, pair := range [][2]uuid.UUID{{viewer, blocked}, {blocked, viewer}} {
		got, err := repo.VisibleBetween(t.Context(), pair[0], pair[1])
		if err != nil {
			t.Fatalf("VisibleBetween() failed: %v", err)
		}
		if got {
			t.Errorf("VisibleBetween(%s, %s) = true, want false after a block", pair[0], pair[1])
		}
	}
}
