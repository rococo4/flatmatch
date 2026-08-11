package likesrepo

import (
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func mustIncoming(t *testing.T, repo *Repo, userID uuid.UUID) []IncomingLike {
	t.Helper()
	incoming, err := repo.Incoming(t.Context(), userID)
	if err != nil {
		t.Fatalf("Incoming() failed: %v", err)
	}
	return incoming
}

func TestIncomingHidesAnsweredAndInvisible(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active", "male", "any")

	waited := make([]uuid.UUID, 2)
	for i := range waited {
		waited[i] = seedUser(t, pool, int64(10+i), "active", "male", "any")
		seedSearch(t, pool, waited[i], "active")
		if _, err := repo.Create(t.Context(), waited[i], me, testZone); err != nil {
			t.Fatalf("Create() of waiting like #%d failed: %v", i, err)
		}
	}

	answered := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, answered, "active")
	if _, err := repo.Create(t.Context(), answered, me, testZone); err != nil {
		t.Fatalf("Create() of the answered like failed: %v", err)
	}
	// Ответ закрыл мэтч — отвечать больше не на что, входящий уходит из списка.
	if _, err := repo.Create(t.Context(), me, answered, ""); err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}

	narrowed := seedUser(t, pool, 3, "active", "male", "any")
	seedSearch(t, pool, narrowed, "active")
	if _, err := repo.Create(t.Context(), narrowed, me, "line-1"); err != nil {
		t.Fatalf("Create() of the narrowed liker failed: %v", err)
	}
	// Лайк уже стоял, когда его автор сузил круг до своего пола: с этого момента
	// друг для друга они не существуют нигде, входящие — не исключение (§4).
	if _, err := pool.Exec(t.Context(),
		`UPDATE users SET gender = 'female', gender_pref = 'same' WHERE id = $1`, narrowed); err != nil {
		t.Fatalf("narrow the liker's preference: %v", err)
	}

	incoming := mustIncoming(t, repo, me)
	if len(incoming) != 2 {
		t.Fatalf("Incoming() returned %d likes, want 2: %+v", len(incoming), incoming)
	}
	// Свежие сверху.
	if incoming[0].From.ID != waited[1] || incoming[1].From.ID != waited[0] {
		t.Errorf("Incoming() order = %s, %s; want %s, %s",
			incoming[0].From.ID, incoming[1].From.ID, waited[1], waited[0])
	}
	if incoming[0].CreatedAt.Before(incoming[1].CreatedAt) {
		t.Errorf("timestamps are not descending: %v then %v", incoming[0].CreatedAt, incoming[1].CreatedAt)
	}

	got := incoming[0]
	if got.ZoneID != testZone {
		t.Errorf("ZoneID = %q, want %q", got.ZoneID, testZone)
	}
	if got.NotSearching {
		t.Error("NotSearching is set on a liker who still has an active search")
	}
	if got.From.Name != "test" || got.From.Status != entity.StatusActive {
		t.Errorf("liker card = %+v, want a named active person", got.From)
	}
	// Приватность: контакты живут только на экране мэтча (§2.3).
	if got.From.Username != "" || got.From.TelegramID != 0 {
		t.Errorf("Incoming() leaked contacts: username=%q tg_id=%d", got.From.Username, got.From.TelegramID)
	}

	// Мэтч виден обеим сторонам, но входящим не остаётся ни у кого.
	if answeredIncoming := mustIncoming(t, repo, answered); len(answeredIncoming) != 0 {
		t.Errorf("the answering side has %d incoming likes, want 0", len(answeredIncoming))
	}
}

func TestIncomingNotSearchingFlag(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	me := seedUser(t, pool, 1, "active", "male", "any")
	liker := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, liker, "active")
	if _, err := repo.Create(t.Context(), liker, me, testZone); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if incoming := mustIncoming(t, repo, me); len(incoming) != 1 || incoming[0].NotSearching {
		t.Fatalf("Incoming() = %+v, want one like without the mark", incoming)
	}

	// Лайк не сгорает ни от чего (§2.3) — закрытый поиск лишь добавляет
	// адресату серую пометку «сейчас не в поиске».
	if _, err := pool.Exec(t.Context(), `
		UPDATE searches SET status = 'closed', close_reason = 'found_here'
		WHERE user_id = $1`, liker); err != nil {
		t.Fatalf("close the liker's search: %v", err)
	}

	incoming := mustIncoming(t, repo, me)
	if len(incoming) != 1 {
		t.Fatalf("Incoming() returned %d likes, want 1", len(incoming))
	}
	if !incoming[0].NotSearching {
		t.Error("NotSearching is not set on a liker who closed every search")
	}
}

func TestBetweenReportsBothDirections(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	first := seedUser(t, pool, 1, "active", "male", "any")
	second := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, first, "active")

	assert := func(stage string, a, b uuid.UUID, wantAB, wantBA bool) {
		t.Helper()
		aLikedB, bLikedA, err := repo.Between(t.Context(), a, b)
		if err != nil {
			t.Fatalf("Between() at %s failed: %v", stage, err)
		}
		if aLikedB != wantAB || bLikedA != wantBA {
			t.Errorf("Between() at %s = %v, %v; want %v, %v", stage, aLikedB, bLikedA, wantAB, wantBA)
		}
	}

	assert("no likes", first, second, false, false)
	if _, err := repo.Create(t.Context(), first, second, testZone); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	assert("one-sided", first, second, true, false)
	// Порядок аргументов задаёт, что есть «мой лайк», а что — «меня лайкнули».
	assert("one-sided, mirrored", second, first, false, true)

	if _, err := repo.Create(t.Context(), second, first, ""); err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}
	assert("mutual", first, second, true, true)
	assert("a stranger", first, uuid.New(), false, false)
}
