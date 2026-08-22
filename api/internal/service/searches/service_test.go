package searches

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/service/zones"
)

type fakeRepo struct {
	existing      *entity.Search
	created       *entity.SearchParams
	updated       *entity.SearchParams
	createErr     error
	reactivateErr error

	listCalls       int
	closeCalls      int
	reactivateCalls int
}

var _ Repository = (*searchesrepo.Repo)(nil)

func (f *fakeRepo) Create(_ context.Context, _ uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	if f.createErr != nil {
		return entity.Search{}, f.createErr
	}
	f.created = &p
	return entity.Search{SearchParams: p, Status: entity.SearchActive}, nil
}
func (f *fakeRepo) ByID(context.Context, uuid.UUID, uuid.UUID) (entity.Search, error) {
	if f.existing == nil {
		return entity.Search{}, entity.ErrNotFound
	}
	return *f.existing, nil
}
func (f *fakeRepo) ListByUser(context.Context, uuid.UUID) ([]entity.Search, error) {
	f.listCalls++
	return nil, nil
}
func (f *fakeRepo) UpdateParams(_ context.Context, _, _ uuid.UUID, p entity.SearchParams) (entity.Search, error) {
	f.updated = &p
	return entity.Search{SearchParams: p}, nil
}
func (f *fakeRepo) Close(_ context.Context, _, _ uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	f.closeCalls++
	return entity.Search{Status: entity.SearchClosed, CloseReason: reason}, nil
}
func (f *fakeRepo) Reactivate(context.Context, uuid.UUID, uuid.UUID) (entity.Search, error) {
	f.reactivateCalls++
	if f.reactivateErr != nil {
		return entity.Search{}, f.reactivateErr
	}
	revived := entity.Search{Status: entity.SearchActive}
	if f.existing != nil {
		revived = *f.existing
		revived.Status = entity.SearchActive
	}
	return revived, nil
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

func TestCreateRejectsUnknownZone(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-atlantis", PriceMax: 40000})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Contains(fields.Fields, "zone_id") {
		t.Errorf("Create() = %v, want ErrInvalidParams on zone_id", err)
	}
	if repo.created != nil {
		t.Error("repo must not be called on invalid params")
	}
}

func TestCreateRejectsBadParams(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 0})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !slices.Contains(fields.Fields, "price_max") {
		t.Errorf("Create() = %v, want price_max complaint", err)
	}
}

func TestCreateEmptyZoneReportsOnce(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "", PriceMax: 40000})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) {
		t.Fatalf("Create() = %v, want FieldsError", err)
	}
	got := 0
	for _, f := range fields.Fields {
		if f == "zone_id" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("zone_id reported %d times in %v, want exactly 1", got, fields.Fields)
	}
}

func TestCreateRejectsAMidMonthMoveIn(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	mid := time.Date(2026, 9, 17, 13, 45, 0, 0, time.UTC)
	_, err := svc.Create(t.Context(), uuid.New(), entity.SearchParams{
		ZoneID: "d-khamovniki", PriceMax: 40000, MoveInMonths: []time.Time{mid}})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || len(fields.Fields) != 1 || fields.Fields[0] != "move_in" {
		t.Fatalf("Create() = %v, want invalid move_in", err)
	}
	if repo.created != nil {
		t.Error("a mid-month move-in reached the repo")
	}
}

func TestCreateNormalisesSetsBeforeTheRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.Create(t.Context(), uuid.New(), entity.SearchParams{
		ZoneID:       "d-khamovniki",
		PriceMax:     40000,
		Rooms:        []int{3, 2, 3},
		Terms:        []entity.Term{entity.TermYear, entity.TermYear},
		MoveInMonths: []time.Time{oct, sept},
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if repo.created == nil {
		t.Fatal("create did not reach the repo")
	}
	if !slices.Equal(repo.created.Rooms, []int{2, 3}) {
		t.Errorf("Rooms = %v, want [2 3]", repo.created.Rooms)
	}
	if !slices.Equal(repo.created.Terms, []entity.Term{entity.TermYear}) {
		t.Errorf("Terms = %v, want [year]", repo.created.Terms)
	}
	if len(repo.created.MoveInMonths) != 2 ||
		!repo.created.MoveInMonths[0].Equal(sept) || !repo.created.MoveInMonths[1].Equal(oct) {
		t.Errorf("MoveInMonths = %v, want [sept oct]", repo.created.MoveInMonths)
	}
}

func TestCreateValidPassesThrough(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	got, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "line-1", PriceMax: 40000})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if got.Status != entity.SearchActive || repo.created == nil {
		t.Errorf("create did not reach the repo: %+v", got)
	}
}

func TestUpdateValidatesLikeCreate(t *testing.T) {
	repo := &fakeRepo{existing: &entity.Search{
		SearchParams: entity.SearchParams{ZoneID: "d-atlantis", PriceMax: 40000},
		Status:       entity.SearchActive,
	}}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Update(t.Context(), uuid.New(), uuid.New(),
		entity.SearchParams{ZoneID: "d-atlantis", PriceMax: 40000})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Contains(fields.Fields, "zone_id") {
		t.Errorf("Update() = %v, want ErrInvalidParams on zone_id", err)
	}
	if repo.updated != nil {
		t.Error("repo must not be called on invalid params")
	}
}

func TestUpdateRejectsZoneChange(t *testing.T) {
	repo := &fakeRepo{existing: &entity.Search{
		SearchParams: entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 45000},
		Status:       entity.SearchActive,
	}}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Update(t.Context(), uuid.New(), uuid.New(),
		entity.SearchParams{ZoneID: "line-1", PriceMax: 45000})

	var fields *entity.FieldsError
	if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) ||
		!slices.Equal(fields.Fields, []string{"zone_id"}) {
		t.Errorf("Update() = %v, want ErrInvalidParams on zone_id only", err)
	}
	if repo.updated != nil {
		t.Error("repo must not be called on a zone change")
	}
}

func TestUpdateMissingSearchIsNotFound(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	_, err := svc.Update(t.Context(), uuid.New(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 45000})
	if !errors.Is(err, entity.ErrNotFound) {
		t.Errorf("Update() = %v, want ErrNotFound", err)
	}
}

func TestUpdateNormalisesAndPassesThrough(t *testing.T) {
	repo := &fakeRepo{existing: &entity.Search{
		SearchParams: entity.SearchParams{ZoneID: "line-1", PriceMax: 40000},
		Status:       entity.SearchActive,
	}}
	svc := New(repo, testDir(t), nil)

	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.Update(t.Context(), uuid.New(), uuid.New(), entity.SearchParams{
		ZoneID:       "line-1",
		PriceMax:     40000,
		Rooms:        []int{3, 2},
		MoveInMonths: []time.Time{oct, sept},
	})
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}
	if repo.updated == nil {
		t.Fatal("update did not reach the repo")
	}
	if !slices.Equal(repo.updated.Rooms, []int{2, 3}) {
		t.Errorf("Rooms = %v, want [2 3]", repo.updated.Rooms)
	}
	if len(repo.updated.MoveInMonths) != 2 ||
		!repo.updated.MoveInMonths[0].Equal(sept) || !repo.updated.MoveInMonths[1].Equal(oct) {
		t.Errorf("MoveInMonths = %v, want [sept oct]", repo.updated.MoveInMonths)
	}
}

func TestCloseRejectsBadReasons(t *testing.T) {
	bad := map[string]entity.CloseReason{
		"empty":    "",
		"sentinel": entity.CloseReasonUnknown,
		"garbage":  entity.CloseReason("banana"),
	}

	for name, reason := range bad {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := New(repo, testDir(t), nil)

			_, err := svc.Close(t.Context(), uuid.New(), uuid.New(), reason)

			var fields *entity.FieldsError
			if !errors.As(err, &fields) || !errors.Is(err, ErrInvalidParams) {
				t.Fatalf("Close(%q) = %v, want ErrInvalidParams", reason, err)
			}
			if !slices.Equal(fields.Fields, []string{"reason"}) {
				t.Errorf("Fields = %v, want [reason]", fields.Fields)
			}
			if repo.closeCalls != 0 {
				t.Errorf("closeCalls = %d, want 0 — repo must not see a bad reason", repo.closeCalls)
			}
		})
	}
}

func TestCloseValidReasonReachesRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	got, err := svc.Close(t.Context(), uuid.New(), uuid.New(), entity.CloseFoundHere)
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	if repo.closeCalls != 1 {
		t.Errorf("closeCalls = %d, want 1", repo.closeCalls)
	}
	if got.CloseReason != entity.CloseFoundHere {
		t.Errorf("CloseReason = %q, want %q", got.CloseReason, entity.CloseFoundHere)
	}
}

func TestListAndReactivateReachRepo(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	if _, err := svc.List(t.Context(), uuid.New()); err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if repo.listCalls != 1 {
		t.Errorf("listCalls = %d, want 1", repo.listCalls)
	}

	if _, err := svc.Reactivate(t.Context(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("Reactivate() failed: %v", err)
	}
	if repo.reactivateCalls != 1 {
		t.Errorf("reactivateCalls = %d, want 1", repo.reactivateCalls)
	}
}
