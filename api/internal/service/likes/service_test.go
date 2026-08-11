package likes

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/zones"
)

type fakeRepo struct {
	err error

	zoneID        string
	createCalls   int
	incomingCalls int
	budgetCalls   int
}

// The real repository must keep satisfying the interface this package declares:
// drift should break the build here, not at the Task 5 handler wiring.
var _ Repository = (*likesrepo.Repo)(nil)

func (f *fakeRepo) Create(_ context.Context, _, _ uuid.UUID, zoneID string) (entity.LikeResult, error) {
	f.createCalls++
	f.zoneID = zoneID
	if f.err != nil {
		return entity.LikeResult{}, f.err
	}
	return entity.LikeResult{Like: entity.Like{ZoneID: zoneID, Spent: true}}, nil
}

func (f *fakeRepo) Incoming(context.Context, uuid.UUID) ([]likesrepo.IncomingLike, error) {
	f.incomingCalls++
	return nil, nil
}

func (f *fakeRepo) Budget(context.Context, uuid.UUID) (entity.LikeBudget, error) {
	f.budgetCalls++
	return entity.LikeBudget{Remaining: entity.MaxDailyLikes}, nil
}

func testDir(t *testing.T) *zones.Directory {
	t.Helper()
	raw, err := os.ReadFile("../zones/testdata/mini.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	d, err := zones.New(raw)
	if err != nil {
		t.Fatalf("zones.New: %v", err)
	}
	return d
}

// likes_users.zone_id has no foreign key, so a zone nobody knows would be
// stored as it came and resurface as the context of the match.
func TestLikeRejectsUnknownZone(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	_, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "d-atlantis")

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Equal(fields.Fields, []string{"zone_id"}) {
		t.Errorf("Like() = %v, want ErrInvalidParams on zone_id", err)
	}
	if repo.createCalls != 0 {
		t.Error("repo must not be called with an unknown zone")
	}
}

// A reply carries no context (§2.3), and that is not a missing field.
func TestLikeWithoutZoneIsLegal(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	got, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "")
	if err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.createCalls != 1 || repo.zoneID != "" || got.Like.ZoneID != "" {
		t.Errorf("empty zone did not reach the repo as empty: calls=%d zone=%q",
			repo.createCalls, repo.zoneID)
	}
}

func TestLikeKnownZoneReachesRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	if _, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "d-khamovniki"); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.zoneID != "d-khamovniki" {
		t.Errorf("repo got zone %q, want d-khamovniki", repo.zoneID)
	}
}

// Every gate of §2.3 lives in the transaction; the service must not swallow
// or rename what it answers.
func TestLikePassesRepoErrorsThrough(t *testing.T) {
	for _, want := range []error{
		entity.ErrLikeLimit, entity.ErrNeedSearch, entity.ErrNotApproved, entity.ErrNotFound,
	} {
		t.Run(want.Error(), func(t *testing.T) {
			svc := New(&fakeRepo{err: want}, testDir(t))

			_, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "line-1")
			if !errors.Is(err, want) {
				t.Errorf("Like() = %v, want %v", err, want)
			}
		})
	}
}

func TestIncomingAndBudgetReachRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	if _, err := svc.Incoming(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Incoming() failed: %v", err)
	}
	if _, err := svc.Budget(t.Context(), uuid.New()); err != nil {
		t.Fatalf("Budget() failed: %v", err)
	}
	if repo.incomingCalls != 1 || repo.budgetCalls != 1 {
		t.Errorf("incoming=%d budget=%d, want 1 and 1", repo.incomingCalls, repo.budgetCalls)
	}
}
