package likes

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/notifications"
)

var _ Pushes = (*notifications.Service)(nil)

type likedCall struct {
	to, from uuid.UUID
	name     string
	gender   entity.Gender
}

type fakePushes struct {
	liked   []likedCall
	matched []entity.Match
}

func (f *fakePushes) Liked(_ context.Context, toID, fromID uuid.UUID, fromName string, fromGender entity.Gender) {
	f.liked = append(f.liked, likedCall{to: toID, from: fromID, name: fromName, gender: fromGender})
}

func (f *fakePushes) Matched(_ context.Context, match entity.Match) {
	f.matched = append(f.matched, match)
}

func liker() entity.User {
	return entity.User{
		ID:      uuid.New(),
		Name:    "Катя",
		Profile: entity.Profile{Gender: entity.GenderFemale},
	}
}

func TestSpentLikeTellsTheAddressee(t *testing.T) {
	pushes := &fakePushes{}
	svc := New(&fakeRepo{}, testDir(t), pushes, nil)

	from, to := liker(), uuid.New()
	if _, err := svc.Like(t.Context(), from, to, "d-khamovniki", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}

	if len(pushes.liked) != 1 {
		t.Fatalf("Liked called %d times, want 1", len(pushes.liked))
	}
	got := pushes.liked[0]
	if got.to != to || got.from != from.ID || got.name != "Катя" || got.gender != entity.GenderFemale {
		t.Errorf("Liked(%+v), want (%s, %s, Катя, female)", got, to, from.ID)
	}
	if len(pushes.matched) != 0 {
		t.Errorf("Matched called %d times without a match, want 0", len(pushes.matched))
	}
}

func TestReplyPushesTheMatchOnly(t *testing.T) {
	match := entity.Match{ID: uuid.New(), AID: uuid.New(), BID: uuid.New()}
	pushes := &fakePushes{}
	svc := New(&fakeRepo{result: &entity.LikeResult{
		Like:  entity.Like{Spent: false},
		Match: &match,
	}}, testDir(t), pushes, nil)

	if _, err := svc.Like(t.Context(), liker(), uuid.New(), "", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}

	if len(pushes.liked) != 0 {
		t.Errorf("Liked called %d times on a reply, want 0", len(pushes.liked))
	}
	if len(pushes.matched) != 1 || pushes.matched[0].ID != match.ID {
		t.Errorf("Matched = %+v, want the match %s once", pushes.matched, match.ID)
	}
}

func TestRepeatedTapStaysSilent(t *testing.T) {
	pushes := &fakePushes{}
	svc := New(&fakeRepo{result: &entity.LikeResult{
		Like:    entity.Like{Spent: true},
		Already: true,
	}}, testDir(t), pushes, nil)

	if _, err := svc.Like(t.Context(), liker(), uuid.New(), "d-khamovniki", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}

	if len(pushes.liked) != 0 || len(pushes.matched) != 0 {
		t.Errorf("pushes = %+v, want nothing on a repeat tap", pushes)
	}
}

func TestFailedLikeStaysSilent(t *testing.T) {
	pushes := &fakePushes{}
	svc := New(&fakeRepo{err: entity.ErrLikeLimit}, testDir(t), pushes, nil)

	if _, err := svc.Like(t.Context(), liker(), uuid.New(), "d-khamovniki", nil); !errors.Is(err, entity.ErrLikeLimit) {
		t.Fatalf("Like() = %v, want ErrLikeLimit", err)
	}

	if len(pushes.liked) != 0 || len(pushes.matched) != 0 {
		t.Errorf("pushes = %+v, want nothing after a refused like", pushes)
	}
}

func TestLikeWithoutPushesDoesNotPanic(t *testing.T) {
	match := entity.Match{ID: uuid.New()}
	svc := New(&fakeRepo{result: &entity.LikeResult{
		Like:  entity.Like{Spent: true},
		Match: &match,
	}}, testDir(t), nil, nil)

	if _, err := svc.Like(t.Context(), liker(), uuid.New(), "d-khamovniki", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
}
