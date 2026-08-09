package searchesrepo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/postgres/pgtest"
)

// Test users live in their own negative tg_id range so leftovers never
// collide with real accounts or with usersrepo's tests.
const testTGBase int64 = -998000000

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// pgx uses the extended protocol: one statement per Exec, no semicolons.
	clean := func() {
		for _, q := range []string{
			`DELETE FROM searches WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM blocks WHERE blocker_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM users WHERE tg_id <= $1 AND tg_id > $1-1000`,
		} {
			if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
				t.Errorf("clean up test users: %v", err)
			}
		}
	}
	clean()
	t.Cleanup(clean)
}

// seedUser inserts a user directly: repo methods can't set status/gender to
// arbitrary combinations, and this test is about the SQL function itself.
func seedUser(t *testing.T, pool *pgxpool.Pool, n int64, status, gender, pref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, status, gender, gender_pref)
		VALUES ($1, 'test', $2, $3, $4) RETURNING id`,
		testTGBase-n, status, gender, pref).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func visibleTo(t *testing.T, pool *pgxpool.Pool, viewer uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT visible_people($1)`, viewer)
	if err != nil {
		t.Fatalf("visible_people: %v", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	set := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

func TestVisiblePeopleGenderIsMutual(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	anyM := seedUser(t, pool, 1, "active", "male", "any")
	sameM := seedUser(t, pool, 2, "active", "male", "same")
	anyF := seedUser(t, pool, 3, "active", "female", "any")
	sameF := seedUser(t, pool, 4, "active", "female", "same")

	tests := []struct {
		name    string
		viewer  uuid.UUID
		wantYes []uuid.UUID
		wantNo  []uuid.UUID
	}{
		// same-M видит any-M (оба согласны), не видит женщин: он их отсёк.
		{"same male", sameM, []uuid.UUID{anyM}, []uuid.UUID{anyF, sameF}},
		// any-F готова на всех, но same-M отсёк её: правило обоюдно (§4).
		{"any female", anyF, []uuid.UUID{anyM, sameF}, []uuid.UUID{sameM}},
		// any-M не видит same-F по той же причине, себя — никогда.
		{"any male", anyM, []uuid.UUID{anyF, sameM}, []uuid.UUID{sameF, anyM}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis := visibleTo(t, pool, tt.viewer)
			for _, id := range tt.wantYes {
				if !vis[id] {
					t.Errorf("user %s must be visible", id)
				}
			}
			for _, id := range tt.wantNo {
				if vis[id] {
					t.Errorf("user %s must be invisible", id)
				}
			}
		})
	}
}

func TestVisiblePeopleOnlyActive(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	viewer := seedUser(t, pool, 1, "draft", "male", "any") // draft может смотреть (§5)
	for i, status := range []string{"draft", "pending", "banned"} {
		hidden := seedUser(t, pool, int64(10+i), status, "female", "any")
		if visibleTo(t, pool, viewer)[hidden] {
			t.Errorf("%s user must be invisible", status)
		}
	}
	active := seedUser(t, pool, 20, "active", "female", "any")
	if !visibleTo(t, pool, viewer)[active] {
		t.Error("active user must be visible")
	}
}

func TestVisiblePeopleBlockWorksBothWays(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)

	a := seedUser(t, pool, 1, "active", "male", "any")
	b := seedUser(t, pool, 2, "active", "female", "any")

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, a, b); err != nil {
		t.Fatalf("insert block: %v", err)
	}

	if visibleTo(t, pool, a)[b] {
		t.Error("blocker must not see the blocked")
	}
	if visibleTo(t, pool, b)[a] {
		t.Error("blocked must not see the blocker either")
	}
}
