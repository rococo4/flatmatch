package likesrepo

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/repo/usersrepo"
)

const testTGBase int64 = -996000000

const testZone = "d-khamovniki"

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	clean := func() {
		for _, q := range []string{
			`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_users WHERE from_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM like_spends WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_points WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_points WHERE point_id IN (SELECT id FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000))`,
			`DELETE FROM points WHERE created_by IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
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

func seedSearch(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, status string) {
	t.Helper()
	var reason *string
	if status == "closed" {
		reason = pointer.To("changed_mind")
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO searches (user_id, zone_id, price_max, status, close_reason)
		VALUES ($1, $2, 45000, $3, $4)`, userID, testZone, status, reason)
	if err != nil {
		t.Fatalf("seed search: %v", err)
	}
}

func zoneCtx(zoneID string) LikeContext {
	return LikeContext{ZoneID: zoneID, ContextZones: []string{zoneID}}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func likeCreatedAt(t *testing.T, pool *pgxpool.Pool, fromID, toID uuid.UUID) time.Time {
	t.Helper()
	var at time.Time
	err := pool.QueryRow(context.Background(),
		`SELECT created_at FROM likes_users WHERE from_id = $1 AND to_id = $2`, fromID, toID).Scan(&at)
	if err != nil {
		t.Fatalf("read created_at of like %s -> %s: %v", fromID, toID, err)
	}
	return at
}

func TestLikeSpendsBudget(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	res, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if !res.Like.Spent || res.Already || res.Match != nil {
		t.Errorf("unexpected result: %+v", res)
	}
	if res.Like.FromID != liker || res.Like.ToID != target || res.Like.ZoneID != testZone {
		t.Errorf("like lost its fields: %+v", res.Like)
	}
	if res.Budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining = %d, want %d", res.Budget.Remaining, entity.MaxDailyLikes-1)
	}

	want := res.Like.CreatedAt.Add(entity.LikeWindow)
	if res.Budget.NextFreeAt == nil || !res.Budget.NextFreeAt.Equal(want) {
		t.Errorf("next_free_at = %v, want %v", res.Budget.NextFreeAt, want)
	}

	budget, err := repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("Budget() remaining = %d, want %d", budget.Remaining, entity.MaxDailyLikes-1)
	}

	targetBudget, err := repo.Budget(t.Context(), target)
	if err != nil {
		t.Fatalf("Budget() of the target failed: %v", err)
	}
	if targetBudget.Remaining != entity.MaxDailyLikes || targetBudget.NextFreeAt != nil {
		t.Errorf("target budget = %+v, want a full one", targetBudget)
	}

	if n := countRows(t, pool,
		`SELECT count(*) FROM likes_users WHERE from_id = $1 AND to_id = $2 AND spent`, liker, target); n != 1 {
		t.Errorf("database holds %d spent likes, want 1", n)
	}
}

func TestLikeRequiresActiveSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")

	if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); !errors.Is(err, entity.ErrNeedSearch) {
		t.Errorf("Create() without a search = %v, want ErrNeedSearch", err)
	}

	seedSearch(t, pool, liker, "closed")
	if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); !errors.Is(err, entity.ErrNeedSearch) {
		t.Errorf("Create() with a closed search = %v, want ErrNeedSearch", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 0 {
		t.Errorf("database holds %d likes after refusals, want 0", n)
	}

	seedSearch(t, pool, liker, "active")
	if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); err != nil {
		t.Fatalf("Create() with an active search failed: %v", err)
	}
}

func TestReplyIsFreeAndUnconditional(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	first := seedUser(t, pool, 1, "active", "male", "any")

	second := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, first, "active")

	if _, err := repo.Create(t.Context(), first, second, zoneCtx(testZone)); err != nil {
		t.Fatalf("first Create() failed: %v", err)
	}

	res, err := repo.Create(t.Context(), second, first, LikeContext{})
	if err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}
	if res.Like.Spent {
		t.Error("reply spent a slot")
	}
	if res.Like.ZoneID != "" {
		t.Errorf("reply zone = %q, want an empty one", res.Like.ZoneID)
	}
	if res.Match == nil {
		t.Fatal("reply did not produce a match")
	}
	if res.Match.AID.String() >= res.Match.BID.String() {
		t.Errorf("match pair is not ordered: %s, %s", res.Match.AID, res.Match.BID)
	}
	if res.Match.Peer(first) != second || res.Match.Peer(second) != first {
		t.Errorf("match holds the wrong pair: %+v", res.Match)
	}
	if res.Budget.Remaining != entity.MaxDailyLikes || res.Budget.NextFreeAt != nil {
		t.Errorf("replier budget = %+v, want an untouched one", res.Budget)
	}

	budget, err := repo.Budget(t.Context(), first)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("the first liker's remaining = %d, want %d", budget.Remaining, entity.MaxDailyLikes-1)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM matches WHERE a_id = $1 OR b_id = $1`, first); n != 1 {
		t.Errorf("database holds %d matches, want 1", n)
	}
}

func TestMatchContextIsFirstLike(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	first := seedUser(t, pool, 1, "active", "male", "any")
	second := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, first, "active")
	seedSearch(t, pool, second, "active")

	if _, err := repo.Create(t.Context(), first, second, zoneCtx(testZone)); err != nil {
		t.Fatalf("first Create() failed: %v", err)
	}
	res, err := repo.Create(t.Context(), second, first, zoneCtx("line-1"))
	if err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}
	if res.Match == nil {
		t.Fatal("reply did not produce a match")
	}

	if res.Match.ContextZone != testZone {
		t.Errorf("match context = %q, want %q", res.Match.ContextZone, testZone)
	}

	if res.Like.ZoneID != "line-1" {
		t.Errorf("reply zone = %q, want %q", res.Like.ZoneID, "line-1")
	}
}

func TestRepeatedLikeIsIdempotent(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	first, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	again, err := repo.Create(t.Context(), liker, target, zoneCtx("line-1"))
	if err != nil {
		t.Fatalf("repeated Create() failed: %v", err)
	}
	if !again.Already {
		t.Error("repeated like did not report Already")
	}
	if again.Match != nil {
		t.Errorf("repeated one-sided like produced a match: %+v", again.Match)
	}
	if again.Like.ZoneID != testZone || !again.Like.CreatedAt.Equal(first.Like.CreatedAt) {
		t.Errorf("repeated like overwrote the stored one: %+v", again.Like)
	}
	if again.Budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining = %d, want %d — the repeat must be free",
			again.Budget.Remaining, entity.MaxDailyLikes-1)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 1 {
		t.Errorf("database holds %d likes, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM matches WHERE a_id = $1 OR b_id = $1`, liker); n != 0 {
		t.Errorf("database holds %d matches, want 0", n)
	}
}

func TestRepeatIdempotentDespiteExhaustedBudget(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	targets := make([]uuid.UUID, entity.MaxDailyLikes)
	for i := range targets {
		targets[i] = seedUser(t, pool, int64(10+i), "active", "male", "any")
		if _, err := repo.Create(t.Context(), liker, targets[i], zoneCtx(testZone)); err != nil {
			t.Fatalf("Create() #%d failed: %v", i+1, err)
		}
	}

	res, err := repo.Create(t.Context(), liker, targets[entity.MaxDailyLikes-1], zoneCtx(testZone))
	if err != nil {
		t.Fatalf("repeated Create() with an exhausted budget failed: %v", err)
	}
	if !res.Already || res.Budget.Remaining != 0 {
		t.Errorf("result = %+v, want Already with an empty budget", res)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != entity.MaxDailyLikes {
		t.Errorf("database holds %d likes, want %d", n, entity.MaxDailyLikes)
	}
}

func TestRepeatIdempotentDespiteClosedSearch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE searches SET status = 'closed', close_reason = 'found_here'
		WHERE user_id = $1`, liker); err != nil {
		t.Fatalf("close the search: %v", err)
	}

	res, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone))
	if err != nil {
		t.Fatalf("repeated Create() without an active search failed: %v", err)
	}
	if !res.Already {
		t.Errorf("result = %+v, want Already", res)
	}
	if res.Budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining = %d, want %d — the repeat must be free",
			res.Budget.Remaining, entity.MaxDailyLikes-1)
	}
}

func TestLikeNeedsApprovedProfile(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	target := seedUser(t, pool, 1, "active", "male", "any")

	tests := []struct {
		name   string
		status string
	}{
		{"draft", "draft"},
		{"pending", "pending"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			liker := seedUser(t, pool, int64(10+i), tt.status, "male", "any")
			seedSearch(t, pool, liker, "active")
			if _, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone)); !errors.Is(err, entity.ErrNotApproved) {
				t.Errorf("Create() = %v, want ErrNotApproved", err)
			}
			if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 0 {
				t.Errorf("database holds %d likes, want 0", n)
			}
		})
	}
}

func TestLikeInvisiblePersonIs404(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	blocked := seedUser(t, pool, 2, "active", "male", "any")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, blocked, liker); err != nil {
		t.Fatalf("insert block: %v", err)
	}

	tests := []struct {
		name   string
		target uuid.UUID
	}{
		{"gender mismatch", seedUser(t, pool, 3, "active", "female", "same")},
		{"not approved yet", seedUser(t, pool, 4, "draft", "male", "any")},
		{"banned", seedUser(t, pool, 5, "banned", "male", "any")},
		{"blocked me", blocked},
		{"myself", liker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := repo.Create(t.Context(), liker, tt.target, zoneCtx(testZone)); !errors.Is(err, entity.ErrNotFound) {
				t.Errorf("Create() = %v, want ErrNotFound", err)
			}
		})
	}

	if n := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); n != 0 {
		t.Errorf("database holds %d likes, want 0", n)
	}
	budget, err := repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes {
		t.Errorf("remaining = %d, want %d", budget.Remaining, entity.MaxDailyLikes)
	}
}

func TestBudgetWindowSlides(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	targets := make([]uuid.UUID, entity.MaxDailyLikes+1)
	for i := range targets {
		targets[i] = seedUser(t, pool, int64(10+i), "active", "male", "any")
	}
	for i, target := range targets[:entity.MaxDailyLikes] {
		res, err := repo.Create(t.Context(), liker, target, zoneCtx(testZone))
		if err != nil {
			t.Fatalf("Create() #%d failed: %v", i+1, err)
		}
		if want := entity.MaxDailyLikes - i - 1; res.Budget.Remaining != want {
			t.Errorf("remaining after like #%d = %d, want %d", i+1, res.Budget.Remaining, want)
		}
	}
	if _, err := repo.Create(t.Context(), liker, targets[entity.MaxDailyLikes], zoneCtx(testZone)); !errors.Is(err, entity.ErrLikeLimit) {
		t.Fatalf("Create() over the limit = %v, want ErrLikeLimit", err)
	}

	budget, err := repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	oldest := likeCreatedAt(t, pool, liker, targets[0])
	if budget.NextFreeAt == nil || !budget.NextFreeAt.Equal(oldest.Add(entity.LikeWindow)) {
		t.Errorf("next_free_at = %v, want %v", budget.NextFreeAt, oldest.Add(entity.LikeWindow))
	}

	if _, err := pool.Exec(t.Context(), `
		UPDATE likes_users SET created_at = now() - interval '25 hours'
		WHERE from_id = $1 AND to_id = $2`, liker, targets[0]); err != nil {
		t.Fatalf("age the oldest like: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE like_spends SET created_at = now() - interval '25 hours'
		WHERE id = (SELECT id FROM like_spends WHERE user_id = $1
		            ORDER BY created_at, id LIMIT 1)`, liker); err != nil {
		t.Fatalf("age the oldest spend: %v", err)
	}

	budget, err = repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != 1 {
		t.Errorf("remaining = %d, want 1", budget.Remaining)
	}

	second := likeCreatedAt(t, pool, liker, targets[1])
	if budget.NextFreeAt == nil || !budget.NextFreeAt.Equal(second.Add(entity.LikeWindow)) {
		t.Errorf("next_free_at = %v, want %v", budget.NextFreeAt, second.Add(entity.LikeWindow))
	}

	res, err := repo.Create(t.Context(), liker, targets[entity.MaxDailyLikes], zoneCtx(testZone))
	if err != nil {
		t.Fatalf("Create() into the freed slot failed: %v", err)
	}
	if res.Budget.Remaining != 0 {
		t.Errorf("remaining = %d, want 0", res.Budget.Remaining)
	}
}

func TestLikeLimitUnderConcurrency(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	liker := seedUser(t, pool, 1, "active", "male", "any")
	seedSearch(t, pool, liker, "active")

	const attempts = entity.MaxDailyLikes + 2
	targets := make([]uuid.UUID, attempts)
	for i := range targets {
		targets[i] = seedUser(t, pool, int64(10+i), "active", "male", "any")
	}

	conns := make([]*pgxpool.Conn, attempts)
	for i := range conns {
		c, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatalf("warm up pool: %v", err)
		}
		conns[i] = c
	}
	for _, c := range conns {
		c.Release()
	}

	errs := make([]error, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = repo.Create(t.Context(), liker, targets[i], zoneCtx(testZone))
		}()
	}
	close(start)
	wg.Wait()

	var ok, limited int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, entity.ErrLikeLimit):
			limited++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != entity.MaxDailyLikes || limited != attempts-entity.MaxDailyLikes {
		t.Errorf("ok=%d limited=%d, want %d/%d", ok, limited, entity.MaxDailyLikes, attempts-entity.MaxDailyLikes)
	}

	spent := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1 AND spent`, liker)
	if spent != entity.MaxDailyLikes {
		t.Errorf("database holds %d spent likes, want %d", spent, entity.MaxDailyLikes)
	}
	if all := countRows(t, pool, `SELECT count(*) FROM likes_users WHERE from_id = $1`, liker); all != spent {
		t.Errorf("database holds %d likes, %d of them spent — a refused like left a row", all, spent)
	}
}

func TestMutualRaceMakesOneMatch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	first := seedUser(t, pool, 1, "active", "male", "any")
	second := seedUser(t, pool, 2, "active", "male", "any")

	seedSearch(t, pool, first, "active")
	seedSearch(t, pool, second, "active")

	gate, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("open the gate transaction: %v", err)
	}
	defer func() { _ = gate.Rollback(context.Background()) }()
	var gatePID int
	if err := gate.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&gatePID); err != nil {
		t.Fatalf("read the gate backend pid: %v", err)
	}
	if _, err := gate.Exec(t.Context(), `
		SELECT id FROM users WHERE id IN ($1::uuid, $2::uuid)
		ORDER BY id FOR NO KEY UPDATE`, first, second); err != nil {
		t.Fatalf("lock the pair from the gate: %v", err)
	}

	results := make([]entity.LikeResult, 2)
	errs := make([]error, 2)
	pairs := [2][2]uuid.UUID{{first, second}, {second, first}}
	var wg sync.WaitGroup
	for i, pair := range pairs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = repo.Create(t.Context(), pair[0], pair[1], zoneCtx(testZone))
		}()
	}
	waitBlockedBy(t, pool, gatePID, len(pairs))
	if err := gate.Commit(t.Context()); err != nil {
		t.Fatalf("release the gate: %v", err)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Create() #%d failed: %v", i, err)
		}
		if results[i].Match == nil {
			t.Errorf("side #%d got no match", i)
		}
	}
	if results[0].Match != nil && results[1].Match != nil && results[0].Match.ID != results[1].Match.ID {
		t.Errorf("sides got different matches: %s and %s", results[0].Match.ID, results[1].Match.ID)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM matches WHERE a_id = $1 OR b_id = $1`, first); n != 1 {
		t.Errorf("database holds %d matches, want 1", n)
	}

	spent := countRows(t, pool,
		`SELECT count(*) FROM likes_users WHERE from_id IN ($1, $2) AND spent`, first, second)
	if spent != 1 {
		t.Errorf("%d of the two likes are spent, want 1", spent)
	}
}

func waitBlockedBy(t *testing.T, pool *pgxpool.Pool, pid, n int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		blocked := countRows(t, pool, `
			SELECT count(*) FROM pg_stat_activity a
			WHERE $1 = ANY(pg_blocking_pids(a.pid))
			   OR EXISTS (
			       SELECT 1 FROM unnest(pg_blocking_pids(a.pid)) AS ahead (pid)
			       WHERE $1 = ANY(pg_blocking_pids(ahead.pid)))`, pid)
		if blocked >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d sessions are waiting for pid %d, want %d", blocked, pid, n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func inPoolOf(t *testing.T, repo *searchesrepo.Repo, viewer, target uuid.UUID) bool {
	t.Helper()
	list, err := repo.PoolCandidates(t.Context(), viewer, []string{testZone})
	if err != nil {
		t.Fatalf("PoolCandidates() failed: %v", err)
	}
	return slices.ContainsFunc(list, func(c entity.PoolCandidate) bool { return c.User.ID == target })
}

func inZonesOf(t *testing.T, repo *searchesrepo.Repo, viewer, target uuid.UUID) bool {
	t.Helper()
	rows, err := repo.ActiveZonesVisible(t.Context(), viewer)
	if err != nil {
		t.Fatalf("ActiveZonesVisible() failed: %v", err)
	}
	for _, row := range rows {
		if row.ZoneID == testZone && slices.Contains(row.UserIDs, target) {
			return true
		}
	}
	return false
}

func TestLikeAfterTheMatchIsDeletedSpendsAgain(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	matchRepo := matchesrepo.New(pool)
	searchRepo := searchesrepo.New(pool)

	me := seedUser(t, pool, 1, "active", "male", "any")
	peer := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, me, "active")
	seedSearch(t, pool, peer, "active")

	if _, err := repo.Create(t.Context(), me, peer, zoneCtx(testZone)); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	res, err := repo.Create(t.Context(), peer, me, zoneCtx(testZone))
	if err != nil {
		t.Fatalf("the reply Create() failed: %v", err)
	}
	if res.Match == nil {
		t.Fatal("the mutual like produced no match")
	}
	if inPoolOf(t, searchRepo, me, peer) {
		t.Error("a matched peer is still in the zone pool")
	}

	ok, err := matchRepo.Delete(t.Context(), me, res.Match.ID)
	if err != nil || !ok {
		t.Fatalf("Delete() = %v, %v; want true, nil", ok, err)
	}

	if !inPoolOf(t, searchRepo, me, peer) {
		t.Error("the peer did not come back to the zone pool after the delete")
	}
	if !inZonesOf(t, searchRepo, me, peer) {
		t.Error("the peer did not come back to the visible zones after the delete")
	}

	again, err := repo.Create(t.Context(), me, peer, zoneCtx(testZone))
	if err != nil {
		t.Fatalf("the like after the delete failed: %v", err)
	}
	if !again.Like.Spent {
		t.Error("the like after the delete was free, want a spent slot")
	}
	if again.Match != nil || again.Already {
		t.Errorf("the like after the delete = %+v, want a fresh one-sided like", again)
	}
}

func TestDeletedMatchDoesNotRefundTheSlot(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	matchRepo := matchesrepo.New(pool)

	me := seedUser(t, pool, 1, "active", "male", "any")
	peer := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, me, "active")
	seedSearch(t, pool, peer, "active")

	if _, err := repo.Create(t.Context(), me, peer, zoneCtx(testZone)); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	res, err := repo.Create(t.Context(), peer, me, zoneCtx(testZone))
	if err != nil || res.Match == nil {
		t.Fatalf("the reply Create() = %+v, %v; want a match", res, err)
	}

	if ok, err := matchRepo.Delete(t.Context(), me, res.Match.ID); err != nil || !ok {
		t.Fatalf("Delete() = %v, %v; want true, nil", ok, err)
	}

	budget, err := repo.Budget(t.Context(), me)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining after the delete = %d, want %d — the slot must not come back",
			budget.Remaining, entity.MaxDailyLikes-1)
	}

	for i := range entity.MaxDailyLikes - 1 {
		target := seedUser(t, pool, int64(10+i), "active", "male", "any")
		if _, err := repo.Create(t.Context(), me, target, zoneCtx(testZone)); err != nil {
			t.Fatalf("Create() #%d after the delete failed: %v", i+1, err)
		}
	}
	over := seedUser(t, pool, 20, "active", "male", "any")
	if _, err := repo.Create(t.Context(), me, over, zoneCtx(testZone)); !errors.Is(err, entity.ErrLikeLimit) {
		t.Errorf("Create() over the limit = %v, want ErrLikeLimit", err)
	}
}

func TestBlockDoesNotRefundTheSlot(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	users := usersrepo.New(pool)

	me := seedUser(t, pool, 1, "active", "male", "any")
	peer := seedUser(t, pool, 2, "active", "male", "any")
	seedSearch(t, pool, me, "active")

	if _, err := repo.Create(t.Context(), me, peer, zoneCtx(testZone)); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if err := users.Block(t.Context(), me, peer); err != nil {
		t.Fatalf("Block() failed: %v", err)
	}

	budget, err := repo.Budget(t.Context(), me)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining after the block = %d, want %d — the slot must not come back",
			budget.Remaining, entity.MaxDailyLikes-1)
	}
}

func TestBudgetCountsTheSpendLog(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	liker := seedUser(t, pool, 1, "active", "male", "any")
	target := seedUser(t, pool, 2, "active", "male", "any")
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO likes_users (from_id, to_id, zone_id, spent, created_at)
		VALUES ($1, $2, $3, true, now() - interval '2 hours')`, liker, target, testZone); err != nil {
		t.Fatalf("seed a like of the pre-migration world: %v", err)
	}

	budget, err := repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes {
		t.Errorf("remaining before the backfill = %d, want %d — the log is what counts",
			budget.Remaining, entity.MaxDailyLikes)
	}

	if _, err := pool.Exec(t.Context(), `
		INSERT INTO like_spends (user_id, created_at)
		SELECT from_id, created_at FROM likes_users WHERE spent AND from_id = $1`, liker); err != nil {
		t.Fatalf("backfill the spend log: %v", err)
	}

	budget, err = repo.Budget(t.Context(), liker)
	if err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if budget.Remaining != entity.MaxDailyLikes-1 {
		t.Errorf("remaining after the backfill = %d, want %d", budget.Remaining, entity.MaxDailyLikes-1)
	}
	want := likeCreatedAt(t, pool, liker, target).Add(entity.LikeWindow)
	if budget.NextFreeAt == nil || !budget.NextFreeAt.Equal(want) {
		t.Errorf("next_free_at = %v, want %v", budget.NextFreeAt, want)
	}
}
