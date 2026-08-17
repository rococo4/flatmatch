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
	existing      *entity.Search // what ByID answers; nil → ErrNotFound
	created       *entity.SearchParams
	updated       *entity.SearchParams
	createErr     error
	reactivateErr error

	listCalls       int
	closeCalls      int
	reactivateCalls int
}

// The real repository must keep satisfying the interface this package declares:
// drift should break the build here, not at the Task 9 handler wiring.
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

// An empty zone is already entity's complaint; the directory lookup must not
// pile a second "zone_id" onto the same list.
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

func TestCreateNormalisesMoveInToMonthStart(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, testDir(t), nil)

	mid := time.Date(2026, 9, 17, 13, 45, 0, 0, time.UTC)
	_, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 40000, MoveIn: &mid})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if repo.created.MoveIn == nil || !repo.created.MoveIn.Equal(want) {
		t.Errorf("MoveIn = %v, want %v", repo.created.MoveIn, want)
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

// The zone is the search's identity (§3): re-zoning is a new search, not an
// edit. Everything else stays freely editable.
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

	mid := time.Date(2026, 9, 17, 13, 45, 0, 0, time.UTC)
	_, err := svc.Update(t.Context(), uuid.New(), uuid.New(),
		entity.SearchParams{ZoneID: "line-1", PriceMax: 40000, MoveIn: &mid})
	if err != nil {
		t.Fatalf("Update() failed: %v", err)
	}
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if repo.updated == nil {
		t.Fatal("update did not reach the repo")
	}
	if repo.updated.MoveIn == nil || !repo.updated.MoveIn.Equal(want) {
		t.Errorf("MoveIn = %v, want %v", repo.updated.MoveIn, want)
	}
}

// Every class of bad reason must be named as a field here, not surface as a raw
// CHECK violation from Postgres (§3: закрытие — только с причиной).
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
