package searchesrepo

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

// Зоны фан-аута свои: ActiveIn смотрит на всю базу, и чужой поиск в
// «Хамовниках» — девсид, коллега, что угодно — попал бы в выдачу теста.
const (
	fanoutZoneA = "z-test-fanout-a"
	fanoutZoneB = "z-test-fanout-b"
	fanoutZoneC = "z-test-fanout-c"
)

func TestActiveIn(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	author := seedUser(t, pool, 1, "active", "male", "any")
	// Пол не совпадает и анкета не одобрена — ActiveIn всё равно отдаёт обоих:
	// видимость решает сервис одной точкой (§4), запрос знает только зоны.
	neighbour := seedUser(t, pool, 2, "active", "female", "same")
	pending := seedUser(t, pool, 3, "pending", "male", "any")

	create := func(userID uuid.UUID, zone string) entity.Search {
		t.Helper()
		s, err := repo.Create(t.Context(), userID, testParams(zone))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		return s
	}

	inA := create(author, fanoutZoneA)
	inB := create(author, fanoutZoneB)
	neighbourInA := create(neighbour, fanoutZoneA)
	pendingInA := create(pending, fanoutZoneA)

	elsewhere := create(neighbour, fanoutZoneC)
	closed := create(neighbour, fanoutZoneA)
	if _, err := repo.Close(t.Context(), neighbour, closed.ID, entity.CloseFoundHere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	paused := create(pending, fanoutZoneA)
	if _, err := pool.Exec(t.Context(),
		`UPDATE searches SET status = 'paused' WHERE id = $1`, paused.ID); err != nil {
		t.Fatalf("pause search: %v", err)
	}

	got, err := repo.ActiveIn(t.Context(), []string{fanoutZoneA, fanoutZoneB})
	if err != nil {
		t.Fatalf("ActiveIn() failed: %v", err)
	}

	ids := make([]uuid.UUID, 0, len(got))
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	for _, want := range []entity.Search{inA, inB, neighbourInA, pendingInA} {
		if !slices.Contains(ids, want.ID) {
			t.Errorf("ActiveIn() lost the active search %s in %s", want.ID, want.ZoneID)
		}
	}
	for _, unwanted := range []entity.Search{elsewhere, closed, paused} {
		if slices.Contains(ids, unwanted.ID) {
			t.Errorf("ActiveIn() returned %s — a search in another zone or not active", unwanted.ID)
		}
	}

	// Фан-аут строит адресата и контекст из этих двух полей, и они обязаны
	// доехать целыми.
	for _, s := range got {
		if s.ID == neighbourInA.ID && (s.UserID != neighbour || s.ZoneID != fanoutZoneA) {
			t.Errorf("ActiveIn() returned %+v, want user %s in zone %s", s, neighbour, fanoutZoneA)
		}
	}
}

func TestActiveByUser(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	owner := seedUser(t, pool, 1, "active", "male", "any")
	stranger := seedUser(t, pool, 2, "active", "male", "any")

	create := func(userID uuid.UUID, zone string) entity.Search {
		t.Helper()
		s, err := repo.Create(t.Context(), userID, testParams(zone))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		return s
	}

	inA := create(owner, fanoutZoneA)
	inB := create(owner, fanoutZoneB)
	strangers := create(stranger, fanoutZoneA)
	closed := create(owner, fanoutZoneC)
	if _, err := repo.Close(t.Context(), owner, closed.ID, entity.CloseFoundHere); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	got, err := repo.ActiveByUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ActiveByUser() failed: %v", err)
	}

	ids := make([]uuid.UUID, 0, len(got))
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []uuid.UUID{inA.ID, inB.ID}) {
		t.Errorf("ActiveByUser() = %v, want the two active searches %v and %v", ids, inA.ID, inB.ID)
	}
	for _, unwanted := range []entity.Search{strangers, closed} {
		if slices.Contains(ids, unwanted.ID) {
			t.Errorf("ActiveByUser() returned %s — someone else's search or a closed one", unwanted.ID)
		}
	}
	// Доанонс строит контекст из зоны, и она обязана доехать целой.
	if got[0].ZoneID != fanoutZoneA || got[0].UserID != owner {
		t.Errorf("ActiveByUser() returned %+v, want user %s in zone %s", got[0], owner, fanoutZoneA)
	}
}
