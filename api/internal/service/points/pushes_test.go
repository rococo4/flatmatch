package points

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/notifications"
)

var _ Pushes = (*notifications.Service)(nil)

type poolNewCall struct {
	pointID uuid.UUID
	entered uuid.UUID
}

type pointGoneCall struct {
	point     entity.Point
	lastVoter uuid.UUID
}

type fakePushes struct {
	poolNew   []poolNewCall
	pointGone []pointGoneCall
}

func (f *fakePushes) PoolNew(_ context.Context, pointID, enteredUserID uuid.UUID) {
	f.poolNew = append(f.poolNew, poolNewCall{pointID: pointID, entered: enteredUserID})
}

func (f *fakePushes) PointGone(_ context.Context, point entity.Point, lastVoter uuid.UUID) {
	f.pointGone = append(f.pointGone, pointGoneCall{point: point, lastVoter: lastVoter})
}

func TestEnterTellsThePool(t *testing.T) {
	repo := &fakeRepo{entered: true}
	pushes := &fakePushes{}
	s := New(repo, &fakeSearches{}, &fakePhotos{}, mustDir(t), nil, pushes)

	user, point := uuid.New(), uuid.New()
	if err := s.Enter(t.Context(), user, point); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}

	if len(pushes.poolNew) != 1 {
		t.Fatalf("PoolNew called %d times, want 1", len(pushes.poolNew))
	}
	if got := pushes.poolNew[0]; got.pointID != point || got.entered != user {
		t.Errorf("PoolNew(%s, %s), want (%s, %s)", got.pointID, got.entered, point, user)
	}
}

func TestRepeatedEnterStaysSilent(t *testing.T) {
	pushes := &fakePushes{}
	s := New(&fakeRepo{entered: false}, &fakeSearches{}, &fakePhotos{}, mustDir(t), nil, pushes)

	if err := s.Enter(t.Context(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}

	if len(pushes.poolNew) != 0 {
		t.Errorf("PoolNew called %d times on a repeat tap, want 0", len(pushes.poolNew))
	}
}

func TestFailedEnterStaysSilent(t *testing.T) {
	pushes := &fakePushes{}
	s := New(&fakeRepo{entered: true, enterErr: errors.New("no such point")},
		&fakeSearches{}, &fakePhotos{}, mustDir(t), nil, pushes)

	if err := s.Enter(t.Context(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("Enter() succeeded, want the repo error")
	}

	if len(pushes.poolNew) != 0 {
		t.Errorf("PoolNew called %d times after a failed enter, want 0", len(pushes.poolNew))
	}
}

func TestEnterWithoutPushesDoesNotPanic(t *testing.T) {
	s := New(&fakeRepo{entered: true}, &fakeSearches{}, &fakePhotos{}, mustDir(t), nil, nil)

	if err := s.Enter(t.Context(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("Enter() failed: %v", err)
	}
}
