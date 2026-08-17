package searches

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/notifications"
)

// Сервис уведомлений обязан оставаться тем, кого этот пакет просит: расхождение
// должно ломать сборку здесь, а не в проводке.
var _ Pushes = (*notifications.Service)(nil)

type fakePushes struct {
	zoneNew []entity.Search
}

func (f *fakePushes) ZoneNew(_ context.Context, search entity.Search) {
	f.zoneNew = append(f.zoneNew, search)
}

func TestCreateAnnouncesTheSearch(t *testing.T) {
	repo := &fakeRepo{}
	pushes := &fakePushes{}
	svc := New(repo, testDir(t), pushes)

	if _, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 40000}); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if len(pushes.zoneNew) != 1 {
		t.Fatalf("ZoneNew called %d times, want 1", len(pushes.zoneNew))
	}
	if got := pushes.zoneNew[0].ZoneID; got != "d-khamovniki" {
		t.Errorf("ZoneNew got the search of zone %q, want d-khamovniki", got)
	}
}

// Правка параметров пуша не заказывает: подписчики уже знают об этом человеке,
// а пул виртуален и никого не дёргает (§3).
func TestUpdateStaysSilent(t *testing.T) {
	repo := &fakeRepo{existing: &entity.Search{
		SearchParams: entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 45000},
		Status:       entity.SearchActive,
	}}
	pushes := &fakePushes{}
	svc := New(repo, testDir(t), pushes)

	if _, err := svc.Update(t.Context(), uuid.New(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 50000}); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	if len(pushes.zoneNew) != 0 {
		t.Errorf("ZoneNew called %d times on an edit, want 0", len(pushes.zoneNew))
	}
}

// Реактивация — тот же приход человека в пул, что и создание (§3).
func TestReactivateAnnouncesTheSearch(t *testing.T) {
	repo := &fakeRepo{existing: &entity.Search{
		SearchParams: entity.SearchParams{ZoneID: "line-1", PriceMax: 40000},
		Status:       entity.SearchClosed,
	}}
	pushes := &fakePushes{}
	svc := New(repo, testDir(t), pushes)

	revived, err := svc.Reactivate(t.Context(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("Reactivate() failed: %v", err)
	}

	if len(pushes.zoneNew) != 1 {
		t.Fatalf("ZoneNew called %d times, want 1", len(pushes.zoneNew))
	}
	// Фан-аут строит пул по зоне: анонсировать надо ровно тот поиск, что вернул
	// репозиторий, а не форму запроса.
	if got := pushes.zoneNew[0]; got.ZoneID != revived.ZoneID || got.ZoneID != "line-1" {
		t.Errorf("ZoneNew got the search of zone %q, want %q", got.ZoneID, revived.ZoneID)
	}
}

func TestFailedReactivateAnnouncesNothing(t *testing.T) {
	repo := &fakeRepo{reactivateErr: errors.New("no such search")}
	pushes := &fakePushes{}
	svc := New(repo, testDir(t), pushes)

	if _, err := svc.Reactivate(t.Context(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("Reactivate() succeeded, want the repo error")
	}

	if len(pushes.zoneNew) != 0 {
		t.Errorf("ZoneNew called %d times after a failed reactivation, want 0", len(pushes.zoneNew))
	}
}

func TestFailedCreateAnnouncesNothing(t *testing.T) {
	repo := &fakeRepo{createErr: errors.New("too many searches")}
	pushes := &fakePushes{}
	svc := New(repo, testDir(t), pushes)

	if _, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 40000}); err == nil {
		t.Fatal("Create() succeeded, want the repo error")
	}

	if len(pushes.zoneNew) != 0 {
		t.Errorf("ZoneNew called %d times after a failed create, want 0", len(pushes.zoneNew))
	}
}

// Стенд без лога уведомлений — рабочий стенд: пуши не часть операции.
func TestNilPushesAreSilent(t *testing.T) {
	svc := New(&fakeRepo{}, testDir(t), nil)

	if _, err := svc.Create(t.Context(), uuid.New(),
		entity.SearchParams{ZoneID: "d-khamovniki", PriceMax: 40000}); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
}
