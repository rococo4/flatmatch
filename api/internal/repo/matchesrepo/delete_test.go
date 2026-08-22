package matchesrepo

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/likesrepo"
)

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func pairRows(t *testing.T, pool *pgxpool.Pool, matchID, a, b uuid.UUID) (matches, likes, polls int) {
	t.Helper()
	return countRows(t, pool, `SELECT count(*) FROM matches WHERE id = $1`, matchID),
		countRows(t, pool, `SELECT count(*) FROM likes_users
			WHERE (from_id, to_id) IN (($1, $2), ($2, $1))`, a, b),
		countRows(t, pool, `SELECT count(*) FROM match_polls WHERE match_id = $1`, matchID)
}

func TestDeleteRemovesMatchLikesAndPolls(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	seedSearch(t, pool, me)
	matchID := matchPair(t, likes, me, peer, testZone)

	if _, err := repo.SavePollAnswer(t.Context(), matchID, peer, entity.PollTalking); err != nil {
		t.Fatalf("SavePollAnswer() failed: %v", err)
	}
	if matches, likeRows, polls := pairRows(t, pool, matchID, me, peer); matches != 1 || likeRows != 2 || polls != 1 {
		t.Fatalf("before the delete: %d match, %d likes, %d polls; want 1, 2, 1", matches, likeRows, polls)
	}

	pair, err := repo.Delete(t.Context(), peer, matchID)
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}
	if pair == nil {
		t.Fatal("Delete() by a member returned no pair, want one")
	}
	if got := []uuid.UUID{pair.AID, pair.BID}; !slices.Contains(got, me) || !slices.Contains(got, peer) {
		t.Errorf("Delete() returned %+v, want the pair of %s and %s", pair, me, peer)
	}

	if matches, likeRows, polls := pairRows(t, pool, matchID, me, peer); matches != 0 || likeRows != 0 || polls != 0 {
		t.Errorf("after the delete: %d match, %d likes, %d polls; want zeros", matches, likeRows, polls)
	}

	pair, err = repo.Delete(t.Context(), peer, matchID)
	if err != nil {
		t.Fatalf("the repeated Delete() failed: %v", err)
	}
	if pair != nil {
		t.Errorf("the repeated Delete() returned %+v, want nil", pair)
	}
}

func TestDeleteByOutsiderDoesNothing(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	stranger := seedUser(t, pool, 3, "stranger")
	seedSearch(t, pool, me)
	matchID := matchPair(t, likes, me, peer, testZone)

	pair, err := repo.Delete(t.Context(), stranger, matchID)
	if err != nil {
		t.Fatalf("Delete() by an outsider failed: %v", err)
	}
	if pair != nil {
		t.Errorf("Delete() by an outsider returned %+v, want nil", pair)
	}

	if matches, likeRows, _ := pairRows(t, pool, matchID, me, peer); matches != 1 || likeRows != 2 {
		t.Errorf("an outsider touched the pair: %d match, %d likes; want 1, 2", matches, likeRows)
	}

	if pair, err := repo.Delete(t.Context(), me, uuid.New()); err != nil || pair != nil {
		t.Errorf("Delete() of a missing match = %v, %v; want nil, nil", pair, err)
	}
}
