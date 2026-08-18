package matchesrepo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/likesrepo"
)

const testTGBase int64 = -995000000

const testZone = "d-khamovniki"

func cleanTestUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	clean := func() {
		for _, q := range []string{
			`DELETE FROM match_polls WHERE user_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM matches WHERE a_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
			`DELETE FROM likes_users WHERE from_id IN (SELECT id FROM users WHERE tg_id <= $1 AND tg_id > $1-1000)`,
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

func seedUser(t *testing.T, pool *pgxpool.Pool, n int64, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (tg_id, name, tg_username, status, gender, gender_pref)
		VALUES ($1, $2, $3, 'active', 'male', 'any') RETURNING id`,
		testTGBase-n, name, name+"_tg").Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedSearch(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO searches (user_id, zone_id, price_max, status)
		VALUES ($1, $2, 45000, 'active')`, userID, testZone)
	if err != nil {
		t.Fatalf("seed search: %v", err)
	}
}

func mustList(t *testing.T, repo *Repo, userID uuid.UUID) []MatchWithPeer {
	t.Helper()
	list, err := repo.ListByUser(t.Context(), userID)
	if err != nil {
		t.Fatalf("ListByUser() failed: %v", err)
	}
	return list
}

func matchPair(t *testing.T, likes *likesrepo.Repo, first, second uuid.UUID, zone string) uuid.UUID {
	t.Helper()
	if _, err := likes.Create(t.Context(), first, second,
		likesrepo.LikeContext{ZoneID: zone, ContextZones: []string{zone, testZone}}); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	res, err := likes.Create(t.Context(), second, first, likesrepo.LikeContext{})
	if err != nil {
		t.Fatalf("reply Create() failed: %v", err)
	}
	if res.Match == nil {
		t.Fatal("the mutual like produced no match")
	}
	return res.Match.ID
}

func TestBannedAndBlockedPeerKeepsTheMatch(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	seedSearch(t, pool, me)
	matchID := matchPair(t, likes, me, peer, testZone)

	both := func(stage string, wantStatus entity.Status, wantBlocked bool) {
		t.Helper()
		list := mustList(t, repo, me)
		if len(list) != 1 || list[0].Match.ID != matchID {
			t.Fatalf("ListByUser() at %s = %+v, want the match kept", stage, list)
		}
		detail, err := repo.ByID(t.Context(), me, matchID)
		if err != nil {
			t.Fatalf("ByID() at %s failed: %v", stage, err)
		}
		for _, got := range []MatchWithPeer{list[0], detail} {
			if got.Peer.Status != wantStatus {
				t.Errorf("peer status at %s = %q, want %q", stage, got.Peer.Status, wantStatus)
			}
			if got.Blocked != wantBlocked {
				t.Errorf("Blocked at %s = %v, want %v", stage, got.Blocked, wantBlocked)
			}
		}
	}

	both("a fresh match", entity.StatusActive, false)

	if _, err := pool.Exec(t.Context(),
		`UPDATE users SET status = 'banned' WHERE id = $1`, peer); err != nil {
		t.Fatalf("ban the peer: %v", err)
	}
	both("a banned peer", entity.StatusBanned, false)

	for _, pair := range [][2]uuid.UUID{{me, peer}, {peer, me}} {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)`, pair[0], pair[1]); err != nil {
			t.Fatalf("insert block %s -> %s: %v", pair[0], pair[1], err)
		}
		both("a block by "+pair[0].String(), entity.StatusBanned, true)
		if _, err := pool.Exec(t.Context(),
			`DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`, pair[0], pair[1]); err != nil {
			t.Fatalf("delete the block: %v", err)
		}
	}
	both("the block lifted", entity.StatusBanned, false)
}

func TestListAndByID(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	stranger := seedUser(t, pool, 3, "stranger")
	seedSearch(t, pool, me)

	matchID := matchPair(t, likes, me, peer, testZone)
	newer := matchPair(t, likes, me, seedUser(t, pool, 4, "later"), "line-1")

	mine := mustList(t, repo, me)
	if len(mine) != 2 {
		t.Fatalf("ListByUser() returned %d matches, want 2", len(mine))
	}

	if mine[0].Match.ID != newer || mine[1].Match.ID != matchID {
		t.Errorf("ListByUser() order = %s, %s; want %s, %s",
			mine[0].Match.ID, mine[1].Match.ID, newer, matchID)
	}
	got := mine[1]

	if got.Match.ContextZone != testZone {
		t.Errorf("context zone = %q, want %q", got.Match.ContextZone, testZone)
	}
	if got.Peer.ID != peer || got.Peer.Name != "peer" {
		t.Errorf("peer = %+v, want the other side of the pair", got.Peer)
	}
	if got.Peer.Status != entity.StatusActive {
		t.Errorf("peer status = %q, want %q", got.Peer.Status, entity.StatusActive)
	}

	if got.Peer.Username != "" || got.Peer.TelegramID != 0 {
		t.Errorf("ListByUser() leaked contacts: username=%q tg_id=%d", got.Peer.Username, got.Peer.TelegramID)
	}

	theirs := mustList(t, repo, peer)
	if len(theirs) != 1 || theirs[0].Match.ID != matchID || theirs[0].Peer.ID != me {
		t.Errorf("the other side sees %+v, want the same match with the opposite peer", theirs)
	}
	if list := mustList(t, repo, stranger); len(list) != 0 {
		t.Errorf("a stranger sees %d matches, want 0", len(list))
	}

	detail, err := repo.ByID(t.Context(), me, matchID)
	if err != nil {
		t.Fatalf("ByID() failed: %v", err)
	}
	if detail.Match.ID != matchID || detail.Peer.ID != peer {
		t.Errorf("ByID() returned %+v, want the match with the peer", detail)
	}

	if detail.Peer.Username != "peer_tg" {
		t.Errorf("peer username = %q, want %q", detail.Peer.Username, "peer_tg")
	}
	if detail.Peer.TelegramID != testTGBase-2 {
		t.Errorf("peer tg_id = %d, want %d", detail.Peer.TelegramID, testTGBase-2)
	}

	if _, err := repo.ByID(t.Context(), stranger, matchID); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ByID() of a stranger's match = %v, want ErrNotFound", err)
	}
	if _, err := repo.ByID(t.Context(), me, uuid.New()); !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("ByID() of a missing match = %v, want ErrNotFound", err)
	}
}
