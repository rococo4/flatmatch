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

	result *entity.LikeResult

	ctx           likesrepo.LikeContext
	createCalls   int
	incomingCalls int
	budgetCalls   int
}

var _ Repository = (*likesrepo.Repo)(nil)

func (f *fakeRepo) Create(_ context.Context, _, _ uuid.UUID, lc likesrepo.LikeContext) (entity.LikeResult, error) {
	f.createCalls++
	f.ctx = lc
	if f.err != nil {
		return entity.LikeResult{}, f.err
	}
	if f.result != nil {
		return *f.result, nil
	}
	return entity.LikeResult{Like: entity.Like{ZoneID: lc.ZoneID, PointID: lc.PointID, Spent: true}}, nil
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

func TestLikeRejectsUnknownZone(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "d-atlantis", nil)

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Equal(fields.Fields, []string{"zone_id"}) {
		t.Errorf("Like() = %v, want ErrInvalidParams on zone_id", err)
	}
	if repo.createCalls != 0 {
		t.Error("repo must not be called with an unknown zone")
	}
}

func TestLikeWithoutZoneIsLegal(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	got, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "", nil)
	if err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.createCalls != 1 || repo.ctx.ZoneID != "" || got.Like.ZoneID != "" {
		t.Errorf("empty zone did not reach the repo as empty: calls=%d zone=%q",
			repo.createCalls, repo.ctx.ZoneID)
	}
	if repo.ctx.ContextZones != nil {
		t.Errorf("empty zone must carry no context zones, got %v", repo.ctx.ContextZones)
	}
}

func TestLikeKnownZoneReachesRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	if _, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "d-khamovniki", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.ctx.ZoneID != "d-khamovniki" {
		t.Errorf("repo got zone %q, want d-khamovniki", repo.ctx.ZoneID)
	}

	if !slices.Contains(repo.ctx.ContextZones, "d-khamovniki") {
		t.Errorf("context zones %v must contain the context zone itself", repo.ctx.ContextZones)
	}
}

func TestLikePointReachesRepoUntouched(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)
	point := uuid.New()

	got, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "", &point)
	if err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.ctx.PointID == nil || *repo.ctx.PointID != point || got.Like.PointID == nil {
		t.Errorf("point context did not reach the repo: %+v", repo.ctx)
	}
	if repo.ctx.ZoneID != "" || repo.ctx.ContextZones != nil {
		t.Errorf("point like must carry no zone context, got %+v", repo.ctx)
	}
}

func TestLikeRefusesTwoContexts(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)
	point := uuid.New()

	_, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "d-khamovniki", &point)

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Equal(fields.Fields, []string{"zone_id", "point_id"}) {
		t.Errorf("Like() = %v, want ErrInvalidParams on both context fields", err)
	}
	if repo.createCalls != 0 {
		t.Error("repo must not be called with two contexts")
	}
}

func TestLikePassesRepoErrorsThrough(t *testing.T) {
	for _, want := range []error{
		entity.ErrLikeLimit, entity.ErrNeedSearch, entity.ErrNotApproved, entity.ErrNotFound,
	} {
		t.Run(want.Error(), func(t *testing.T) {
			svc := New(&fakeRepo{err: want}, testDir(t), nil)

			_, err := svc.Like(t.Context(), entity.User{ID: uuid.New()}, uuid.New(), "line-1", nil)
			if !errors.Is(err, want) {
				t.Errorf("Like() = %v, want %v", err, want)
			}
		})
	}
}

func TestIncomingAndBudgetReachRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

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
