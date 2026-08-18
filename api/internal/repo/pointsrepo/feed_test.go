package pointsrepo

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
)

func newFeedRepo(t *testing.T) (*pgxpool.Pool, *Repo) {
	t.Helper()
	pool := pgtest.NewPool(t)
	cleanTestLinks(t, pool)
	cleanTestUsers(t, pool)
	t.Cleanup(func() { cleanTestLinks(t, pool) })
	return pool, New(pool)
}

func cleanTestLinks(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)
		    OR b_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
		`DELETE FROM likes_users WHERE from_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)
		    OR to_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
		`DELETE FROM blocks WHERE blocker_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)
		    OR blocked_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
	} {
		if _, err := pool.Exec(context.Background(), q, testTGBase); err != nil {
			t.Errorf("clean up test links: %v", err)
		}
	}
}

func seedPerson(t *testing.T, pool *pgxpool.Pool, n int64, gender, pref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, tg_username, status, gender, gender_pref,
			age, occupation, schedule, smoking, photo_key, photo_status)
		VALUES ($1, 'test', 'contact', 'active', $2, $3, 25, 'remote', 'owl', 'no',
			'users/a.jpg', 'ok')
		RETURNING id`, testTGBase-n, gender, pref).Scan(&id)
	if err != nil {
		t.Fatalf("seed person: %v", err)
	}
	return id
}

func seedBlock(t *testing.T, pool *pgxpool.Pool, blocker, blocked uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, blocker, blocked)
	if err != nil {
		t.Fatalf("seed block: %v", err)
	}
}

func seedMatch(t *testing.T, pool *pgxpool.Pool, a, b uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO matches (a_id, b_id)
		VALUES (least($1::uuid, $2::uuid), greatest($1::uuid, $2::uuid))`, a, b)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
}

func seedResponse(t *testing.T, pool *pgxpool.Pool, from, to, pointID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO likes_users (from_id, to_id, point_id, spent) VALUES ($1, $2, $3, true)`,
		from, to, pointID)
	if err != nil {
		t.Fatalf("seed response: %v", err)
	}
}

func seedSearch(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, status string, daysAgo int) uuid.UUID {
	t.Helper()
	var reason *string
	if status == "closed" {
		reason = new(string)
		*reason = "changed_mind"
	}
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO searches (user_id, zone_id, price_max, status, close_reason, created_at)
		VALUES ($1, $2, 45000, $3, $4, now() - make_interval(days => $5))
		RETURNING id`, userID, testZone, status, reason, daysAgo).Scan(&id)
	if err != nil {
		t.Fatalf("seed search: %v", err)
	}
	return id
}

func enterPool(t *testing.T, repo *Repo, userID, pointID uuid.UUID) {
	t.Helper()
	if _, err := repo.Enter(t.Context(), userID, pointID); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}
}

func agePoint(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, daysAgo int) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE points SET created_at = now() - make_interval(days => $2) WHERE id = $1`, id, daysAgo)
	if err != nil {
		t.Fatalf("age point: %v", err)
	}
}

func enteredAgo(t *testing.T, pool *pgxpool.Pool, userID, pointID uuid.UUID, minutesAgo int) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE likes_points SET created_at = now() - make_interval(mins => $3)
		WHERE user_id = $1 AND point_id = $2`, userID, pointID, minutesAgo)
	if err != nil {
		t.Fatalf("move pool entry: %v", err)
	}
}

func TestFeedCountsAndMembersArePersonal(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	mate := seedPerson(t, pool, 2, "male", "any")
	otherGender := seedPerson(t, pool, 3, "female", "same")
	blocked := seedPerson(t, pool, 4, "male", "any")
	matched := seedPerson(t, pool, 5, "male", "any")
	seedBlock(t, pool, blocked, viewer)
	seedMatch(t, pool, viewer, matched)

	flat, err := repo.Create(t.Context(), flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	for _, u := range []uuid.UUID{viewer, mate, otherGender, blocked, matched} {
		enterPool(t, repo, u, flat.ID)
	}

	feed, err := repo.Feed(t.Context(), viewer, []string{testZone})
	if err != nil {
		t.Fatalf("Feed() failed: %v", err)
	}
	if len(feed) != 1 {
		t.Fatalf("Feed() returned %d points, want 1", len(feed))
	}

	if feed[0].PoolCount != 1 {
		t.Errorf("PoolCount = %d, want 1", feed[0].PoolCount)
	}
	if !feed[0].InPool {
		t.Error("InPool = false, want true — смотрящий в пуле")
	}
	if feed[0].Responses != 0 {
		t.Errorf("Responses = %d, want 0 for a market flat", feed[0].Responses)
	}

	members, err := repo.PoolMembers(t.Context(), viewer, flat.ID)
	if err != nil {
		t.Fatalf("PoolMembers() failed: %v", err)
	}
	if len(members) != 1 || members[0].User.ID != mate {
		t.Fatalf("PoolMembers() = %+v, want only the visible mate", members)
	}

	if feed[0].PoolCount != len(members) {
		t.Errorf("PoolCount = %d, but the pool lists %d people", feed[0].PoolCount, len(members))
	}
	if members[0].User.Username != "" || members[0].User.TelegramID != 0 {
		t.Errorf("pool member carries contacts: %+v", members[0].User)
	}

	hers, err := repo.Feed(t.Context(), otherGender, []string{testZone})
	if err != nil {
		t.Fatalf("Feed() for the other gender failed: %v", err)
	}
	if len(hers) != 1 || hers[0].PoolCount != 0 {
		t.Errorf("Feed() for the other gender = %+v, want one point with an empty pool", hers)
	}
}

func TestFeedResponsesCountPointLikes(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	author := seedPerson(t, pool, 2, "male", "any")
	responder := seedPerson(t, pool, 3, "male", "any")
	hidden := seedPerson(t, pool, 4, "female", "same")

	home, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	flat, err := repo.Create(t.Context(), flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(flat) failed: %v", err)
	}
	seedResponse(t, pool, responder, author, home.ID)
	seedResponse(t, pool, hidden, author, home.ID)

	seedResponse(t, pool, hidden, responder, flat.ID)

	feed, err := repo.Feed(t.Context(), viewer, []string{testZone})
	if err != nil {
		t.Fatalf("Feed() failed: %v", err)
	}
	byID := map[uuid.UUID]FeedPoint{}
	for _, p := range feed {
		byID[p.Point.ID] = p
	}

	if got := byID[home.ID].Responses; got != 2 {
		t.Errorf("Responses of the room = %d, want 2", got)
	}
	if got := byID[flat.ID].Responses; got != 0 {
		t.Errorf("Responses of the market flat = %d, want 0", got)
	}
}

func TestFeedRanksFreshOverCrowded(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	var crowd []uuid.UUID
	for n := int64(2); n <= 6; n++ {
		crowd = append(crowd, seedPerson(t, pool, n, "male", "any"))
	}

	newPoint := func(url string) entity.Point {
		t.Helper()
		p, err := repo.Create(t.Context(), flatPoint(nil, entity.SourceManual, url, entity.PointActive))
		if err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
		return p
	}

	empty := newPoint("https://cian.ru/empty")
	crowded := newPoint("https://cian.ru/crowded")
	aging := newPoint("https://cian.ru/aging")
	agePoint(t, pool, crowded.ID, 1)
	agePoint(t, pool, aging.ID, 10)
	for _, u := range crowd {
		enterPool(t, repo, u, crowded.ID)
	}
	for _, u := range crowd[:2] {
		enterPool(t, repo, u, aging.ID)
	}

	feed, err := repo.Feed(t.Context(), viewer, []string{testZone})
	if err != nil {
		t.Fatalf("Feed() failed: %v", err)
	}
	want := []uuid.UUID{crowded.ID, empty.ID, aging.ID}
	if len(feed) != len(want) {
		t.Fatalf("Feed() returned %d points, want %d", len(feed), len(want))
	}
	got := make([]uuid.UUID, 0, len(feed))
	for _, p := range feed {
		got = append(got, p.Point.ID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Feed() order = %v, want crowded %s, empty %s, aging %s",
			got, crowded.ID, empty.ID, aging.ID)
	}

	for _, p := range feed {
		if p.InPool {
			t.Errorf("InPool = true for point %s the viewer never entered", p.Point.ID)
		}
	}
}

func TestFeedAuthorIsShowcaseOnly(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	author := seedPerson(t, pool, 2, "male", "any")
	hiddenAuthor := seedPerson(t, pool, 3, "female", "same")

	home, err := repo.Create(t.Context(), intentHome(author, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	hiddenHome, err := repo.Create(t.Context(), intentHome(hiddenAuthor, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(hidden intent_home) failed: %v", err)
	}
	flat, err := repo.Create(t.Context(), flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(flat) failed: %v", err)
	}

	feed, err := repo.Feed(t.Context(), viewer, []string{testZone})
	if err != nil {
		t.Fatalf("Feed() failed: %v", err)
	}
	byID := map[uuid.UUID]FeedPoint{}
	for _, p := range feed {
		byID[p.Point.ID] = p
	}

	if byID[flat.ID].Author != nil {
		t.Errorf("market flat carries an author: %+v", byID[flat.ID].Author)
	}
	if byID[flat.ID].InPool {
		t.Error("InPool = true for a pool the viewer never entered")
	}
	got := byID[home.ID].Author
	if got == nil {
		t.Fatalf("intent_home came without its author")
	}
	if got.ID != author || got.Name != "test" || got.Profile.Age != 25 ||
		got.Profile.Occupation != entity.OccupationRemote ||
		got.Profile.Schedule != entity.ScheduleOwl || got.PhotoKey != "users/a.jpg" ||
		got.PhotoStatus != entity.ModerationOK || got.Status != entity.StatusActive {
		t.Errorf("author lost the showcase fields: %+v", got)
	}

	if got.Username != "" || got.TelegramID != 0 {
		t.Errorf("author carries contacts: %+v", got)
	}

	if _, shown := byID[hiddenHome.ID]; shown {
		t.Error("feed shows the room of an author the viewer cannot see")
	}
}

func TestMineSplitsCreatedAndEntered(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	other := seedPerson(t, pool, 2, "male", "any")

	pending, err := repo.Create(t.Context(),
		flatPoint(&viewer, entity.SourceUserLink, "https://avito.ru/pending", entity.PointPending))
	if err != nil {
		t.Fatalf("Create(pending) failed: %v", err)
	}
	own, err := repo.Create(t.Context(),
		flatPoint(&viewer, entity.SourceUserLink, "https://avito.ru/own", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(own) failed: %v", err)
	}
	closed, err := repo.Create(t.Context(),
		flatPoint(&viewer, entity.SourceUserLink, "https://avito.ru/closed", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(closed) failed: %v", err)
	}
	closePoint(t, pool, closed.ID)
	home, err := repo.Create(t.Context(), intentHome(viewer, entity.StageLive))
	if err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	foreign, err := repo.Create(t.Context(),
		flatPoint(&other, entity.SourceUserLink, "https://avito.ru/foreign", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(foreign) failed: %v", err)
	}
	enterPool(t, repo, viewer, own.ID)
	enterPool(t, repo, viewer, foreign.ID)

	created, entered, err := repo.Mine(t.Context(), viewer)
	if err != nil {
		t.Fatalf("Mine() failed: %v", err)
	}

	ids := func(points []FeedPoint) map[uuid.UUID]FeedPoint {
		out := map[uuid.UUID]FeedPoint{}
		for _, p := range points {
			out[p.Point.ID] = p
		}
		return out
	}
	createdByID, enteredByID := ids(created), ids(entered)

	if len(created) != 3 {
		t.Errorf("Mine() created = %d points, want 3", len(created))
	}
	for _, want := range []uuid.UUID{pending.ID, own.ID, home.ID} {
		if _, ok := createdByID[want]; !ok {
			t.Errorf("created list misses point %s", want)
		}
	}
	if _, ok := createdByID[closed.ID]; ok {
		t.Error("created list shows a closed point")
	}
	if !createdByID[own.ID].InPool {
		t.Error("InPool = false for the creator's own pool")
	}

	if a := createdByID[own.ID].Author; a != nil {
		t.Errorf("user_link flat carries an author: %+v", a)
	}

	if a := createdByID[home.ID].Author; a == nil || a.ID != viewer {
		t.Errorf("own intent_home came without its author: %+v", a)
	}

	if len(entered) != 1 || entered[0].Point.ID != foreign.ID {
		t.Fatalf("Mine() entered = %+v, want only the foreign flat", entered)
	}
	if !enteredByID[foreign.ID].InPool {
		t.Error("InPool = false for an entered pool")
	}
}

func TestPoolMembersCarryFreshestSearch(t *testing.T) {
	pool, repo := newFeedRepo(t)

	viewer := seedPerson(t, pool, 1, "male", "any")
	searching := seedPerson(t, pool, 2, "male", "any")
	silent := seedPerson(t, pool, 3, "male", "any")
	matched := seedPerson(t, pool, 4, "male", "any")
	seedMatch(t, pool, viewer, matched)

	seedSearch(t, pool, searching, "active", 5)
	freshest := seedSearch(t, pool, searching, "active", 1)
	seedSearch(t, pool, searching, "closed", 0)

	seedSearch(t, pool, silent, "closed", 0)

	flat, err := repo.Create(t.Context(), flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive))
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	for _, u := range []uuid.UUID{viewer, searching, silent, matched} {
		enterPool(t, repo, u, flat.ID)
	}
	enteredAgo(t, pool, silent, flat.ID, 3)
	enteredAgo(t, pool, searching, flat.ID, 2)

	members, err := repo.PoolMembers(t.Context(), viewer, flat.ID)
	if err != nil {
		t.Fatalf("PoolMembers() failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("PoolMembers() returned %d people, want 2", len(members))
	}

	if members[0].User.ID != searching || members[1].User.ID != silent {
		t.Fatalf("PoolMembers() order = %s, %s, want searching, silent",
			members[0].User.ID, members[1].User.ID)
	}
	if members[0].Search == nil {
		t.Fatalf("member with searches came without one")
	}
	if members[0].Search.ID != freshest {
		t.Errorf("Search.ID = %s, want the freshest active one %s", members[0].Search.ID, freshest)
	}
	if members[0].Search.UserID != searching || members[0].Search.ZoneID != testZone ||
		members[0].Search.PriceMax != 45000 || members[0].Search.Status != entity.SearchActive {
		t.Errorf("member search lost its parameters: %+v", members[0].Search)
	}
	if members[1].Search != nil {
		t.Errorf("member without an active search carries one: %+v", members[1].Search)
	}
	if members[0].EnteredAt.IsZero() || !members[0].EnteredAt.After(members[1].EnteredAt) {
		t.Errorf("EnteredAt = %s and %s, want the first one later",
			members[0].EnteredAt, members[1].EnteredAt)
	}
	if time.Since(members[0].EnteredAt) > time.Hour {
		t.Errorf("EnteredAt = %s, want a fresh entry", members[0].EnteredAt)
	}
}

func TestCountActiveIn(t *testing.T) {
	pool, repo := newFeedRepo(t)

	author := seedPerson(t, pool, 1, "male", "any")
	if _, err := repo.Create(t.Context(),
		flatPoint(nil, entity.SourceManual, "https://cian.ru/1", entity.PointActive)); err != nil {
		t.Fatalf("Create(active flat) failed: %v", err)
	}
	if _, err := repo.Create(t.Context(), intentHome(author, entity.StageLive)); err != nil {
		t.Fatalf("Create(intent_home) failed: %v", err)
	}
	if _, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/pending", entity.PointPending)); err != nil {
		t.Fatalf("Create(pending flat) failed: %v", err)
	}
	closed, err := repo.Create(t.Context(),
		flatPoint(&author, entity.SourceUserLink, "https://avito.ru/closed", entity.PointActive))
	if err != nil {
		t.Fatalf("Create(closed flat) failed: %v", err)
	}
	closePoint(t, pool, closed.ID)

	n, err := repo.CountActiveIn(t.Context(), []string{testZone})
	if err != nil {
		t.Fatalf("CountActiveIn() failed: %v", err)
	}
	if n != 2 {
		t.Errorf("CountActiveIn() = %d, want 2 — только активные", n)
	}
	if n, err := repo.CountActiveIn(t.Context(), []string{"s-nowhere"}); err != nil || n != 0 {
		t.Errorf("CountActiveIn() of an empty zone = %d, %v, want 0, nil", n, err)
	}
}

func TestMineKeepsRejectedUntilHidden(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)

	creator := seedUser(t, pool, 1)
	rejected, err := repo.Create(t.Context(),
		flatPoint(&creator, entity.SourceUserLink, "https://cian.ru/rejected-1", entity.PointPending))
	if err != nil {
		t.Fatalf("create point: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		UPDATE points SET status = 'closed', close_reason = 'rejected' WHERE id = $1`,
		rejected.ID); err != nil {
		t.Fatalf("reject point: %v", err)
	}

	created, _, err := repo.Mine(t.Context(), creator)
	if err != nil {
		t.Fatalf("Mine() failed: %v", err)
	}
	if len(created) != 1 || created[0].Point.ID != rejected.ID {
		t.Fatalf("rejected point must stay listed, got %d cards", len(created))
	}
	if reason := created[0].Point.CloseReason; reason == nil || *reason != "rejected" {
		t.Errorf("close_reason = %v, want rejected", reason)
	}

	if err := repo.Close(t.Context(), creator, rejected.ID); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	created, _, err = repo.Mine(t.Context(), creator)
	if err != nil {
		t.Fatalf("Mine() after hide failed: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("hidden rejected point still listed: %d cards", len(created))
	}
}
