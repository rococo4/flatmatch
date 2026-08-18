package matchesrepo

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/postgres/pgtest"
	"flatmatch/api/internal/repo/likesrepo"
)

func agePastMatch(t *testing.T, pool *pgxpool.Pool, matchID uuid.UUID, ago time.Duration) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`UPDATE matches SET created_at = now() - make_interval(secs => $2) WHERE id = $1`,
		matchID, ago.Seconds())
	if err != nil {
		t.Fatalf("age match %s: %v", matchID, err)
	}
}

func pollTargets(t *testing.T, repo *Repo, olderThan time.Duration, matchID uuid.UUID) []PollTarget {
	t.Helper()
	all, err := repo.MatchesForPostPoll(t.Context(), olderThan)
	if err != nil {
		t.Fatalf("MatchesForPostPoll() failed: %v", err)
	}
	var out []PollTarget
	for _, target := range all {
		if target.Match.ID == matchID {
			out = append(out, target)
		}
	}
	return out
}

func TestMatchesForPostPollAsksBothSidesUntilTheyAnswer(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	seedSearch(t, pool, me)
	matchID := matchPair(t, likes, me, peer, testZone)
	window := time.Duration(entity.PostMatchDays) * 24 * time.Hour

	if got := pollTargets(t, repo, window, matchID); len(got) != 0 {
		t.Fatalf("a fresh match is already polled: %+v", got)
	}

	agePastMatch(t, pool, matchID, window+time.Hour)
	got := pollTargets(t, repo, window, matchID)
	if len(got) != 2 {
		t.Fatalf("MatchesForPostPoll() returned %d targets, want 2", len(got))
	}
	for _, target := range got {
		if target.Match.ContextZone != testZone {
			t.Errorf("target %+v lost the match context", target)
		}

		wantPeer := "peer"
		if target.UserID == peer {
			wantPeer = "me"
		}
		if target.PeerName != wantPeer {
			t.Errorf("target of %s names %q, want %q", target.UserID, target.PeerName, wantPeer)
		}
	}
	if users := []uuid.UUID{got[0].UserID, got[1].UserID}; !slices.Contains(users, me) ||
		!slices.Contains(users, peer) {
		t.Errorf("targets = %v, want both sides of the pair", users)
	}

	changed, err := repo.SavePollAnswer(t.Context(), matchID, me, entity.PollMoving)
	if err != nil {
		t.Fatalf("SavePollAnswer() failed: %v", err)
	}
	if !changed {
		t.Error("SavePollAnswer() reported no change on the first answer")
	}
	left := pollTargets(t, repo, window, matchID)
	if len(left) != 1 || left[0].UserID != peer {
		t.Errorf("after one answer targets = %+v, want the silent side only", left)
	}
}

func TestSavePollAnswerIsIdempotent(t *testing.T) {
	pool := pgtest.NewPool(t)
	cleanTestUsers(t, pool)
	repo := New(pool)
	likes := likesrepo.New(pool)

	me := seedUser(t, pool, 1, "me")
	peer := seedUser(t, pool, 2, "peer")
	stranger := seedUser(t, pool, 3, "stranger")
	seedSearch(t, pool, me)
	matchID := matchPair(t, likes, me, peer, testZone)

	if changed, err := repo.SavePollAnswer(t.Context(), matchID, me, entity.PollTalking); err != nil || !changed {
		t.Fatalf("SavePollAnswer() = %v, %v, want true, nil", changed, err)
	}

	for _, answer := range []entity.PollAnswer{entity.PollTalking, entity.PollFailed} {
		changed, err := repo.SavePollAnswer(t.Context(), matchID, me, answer)
		if err != nil {
			t.Fatalf("second SavePollAnswer(%s) failed: %v", answer, err)
		}
		if changed {
			t.Errorf("SavePollAnswer(%s) overwrote an answer", answer)
		}
	}
	var stored string
	if err := pool.QueryRow(t.Context(),
		`SELECT answer FROM match_polls WHERE match_id = $1 AND user_id = $2`,
		matchID, me).Scan(&stored); err != nil {
		t.Fatalf("read the stored answer: %v", err)
	}
	if stored != string(entity.PollTalking) {
		t.Errorf("stored answer = %q, want %q", stored, entity.PollTalking)
	}

	for _, id := range []uuid.UUID{matchID, uuid.New()} {
		if _, err := repo.SavePollAnswer(t.Context(), id, stranger, entity.PollMoving); !errors.Is(err, entity.ErrNotFound) {
			t.Errorf("SavePollAnswer() by a stranger = %v, want ErrNotFound", err)
		}
	}
	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM match_polls WHERE match_id = $1`, matchID).Scan(&rows); err != nil {
		t.Fatalf("count answers: %v", err)
	}
	if rows != 1 {
		t.Errorf("match_polls holds %d rows, want 1", rows)
	}
}
