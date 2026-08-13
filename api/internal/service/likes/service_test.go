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

	ctx           likesrepo.LikeContext
	createCalls   int
	incomingCalls int
	budgetCalls   int
}

// The real repository must keep satisfying the interface this package declares:
// drift should break the build here, not at the Task 5 handler wiring.
var _ Repository = (*likesrepo.Repo)(nil)

func (f *fakeRepo) Create(_ context.Context, _, _ uuid.UUID, lc likesrepo.LikeContext) (entity.LikeResult, error) {
	f.createCalls++
	f.ctx = lc
	if f.err != nil {
		return entity.LikeResult{}, f.err
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

// likes_users.zone_id has no foreign key, so a zone nobody knows would be
// stored as it came and resurface as the context of the match.
func TestLikeRejectsUnknownZone(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))

	_, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "d-atlantis", nil)

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

	got, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "", nil)
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
	svc := New(repo, testDir(t))

	if _, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "d-khamovniki", nil); err != nil {
		t.Fatalf("Like() failed: %v", err)
	}
	if repo.ctx.ZoneID != "d-khamovniki" {
		t.Errorf("repo got zone %q, want d-khamovniki", repo.ctx.ZoneID)
	}
	// Гейт «мы увиделись» работает списком пересекающихся зон — сам контекст
	// обязан в него входить, иначе поиск ровно на эту зону лайк не оплатит.
	if !slices.Contains(repo.ctx.ContextZones, "d-khamovniki") {
		t.Errorf("context zones %v must contain the context zone itself", repo.ctx.ContextZones)
	}
}

// Точечный контекст сервис не проверяет — активность точки, её тип и отношение
// к ней читаются в транзакции, рядом с решением о трате слота.
func TestLikePointReachesRepoUntouched(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))
	point := uuid.New()

	got, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "", &point)
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

// Один лайк — один контекст: мэтч рассказывает, ГДЕ пересеклись, и делить этот
// рассказ надвое нельзя (§6).
func TestLikeRefusesTwoContexts(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t))
	point := uuid.New()

	_, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "d-khamovniki", &point)

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Equal(fields.Fields, []string{"zone_id", "point_id"}) {
		t.Errorf("Like() = %v, want ErrInvalidParams on both context fields", err)
	}
	if repo.createCalls != 0 {
		t.Error("repo must not be called with two contexts")
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

			_, err := svc.Like(t.Context(), uuid.New(), uuid.New(), "line-1", nil)
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
