package pointsrepo

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func TestPoolMemberIDs(t *testing.T) {
	pool, repo := newFeedRepo(t)

	entrant := seedPerson(t, pool, 1, "male", "any")
	author := seedPerson(t, pool, 2, "male", "any")
	mate := seedPerson(t, pool, 3, "male", "any")
	matched := seedPerson(t, pool, 4, "male", "any")

	hidden := seedPerson(t, pool, 5, "female", "same")

	point, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/fanout", entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	for _, u := range []uuid.UUID{entrant, mate, matched, hidden} {
		if _, err := repo.Enter(context.Background(), u, point.ID); err != nil {
			t.Fatalf("Enter() failed: %v", err)
		}
	}
	seedMatch(t, pool, entrant, matched)

	got, err := repo.PoolMemberIDs(t.Context(), entrant, point.ID)
	if err != nil {
		t.Fatalf("PoolMemberIDs() failed: %v", err)
	}

	for _, want := range []uuid.UUID{author, mate} {
		if !slices.Contains(got, want) {
			t.Errorf("PoolMemberIDs() = %v, lost the member %s", got, want)
		}
	}
	for _, unwanted := range []uuid.UUID{entrant, matched, hidden} {
		if slices.Contains(got, unwanted) {
			t.Errorf("PoolMemberIDs() = %v, must not contain %s", got, unwanted)
		}
	}
}
