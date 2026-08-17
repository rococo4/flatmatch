package matchesrepo

import (
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/postgres/pgtest"
)

func TestIsMatched(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	a := seedUser(t, pool, 41, "Катя")
	b := seedUser(t, pool, 42, "Марк")
	stranger := seedUser(t, pool, 43, "Ира")

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO matches (a_id, b_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid))`, a, b); err != nil {
		t.Fatalf("seed match: %v", err)
	}

	// Пара канонична (a_id < b_id), а спрашивают о ней с любой стороны.
	for _, pair := range [][2]uuid.UUID{{a, b}, {b, a}} {
		got, err := repo.IsMatched(t.Context(), pair[0], pair[1])
		if err != nil {
			t.Fatalf("IsMatched() failed: %v", err)
		}
		if !got {
			t.Errorf("IsMatched(%s, %s) = false, want true", pair[0], pair[1])
		}
	}

	for _, pair := range [][2]uuid.UUID{{a, stranger}, {stranger, b}, {a, a}} {
		got, err := repo.IsMatched(t.Context(), pair[0], pair[1])
		if err != nil {
			t.Fatalf("IsMatched() failed: %v", err)
		}
		if got {
			t.Errorf("IsMatched(%s, %s) = true, want false", pair[0], pair[1])
		}
	}
}
