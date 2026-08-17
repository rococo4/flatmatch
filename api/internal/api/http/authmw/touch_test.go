package authmw

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/searchesrepo"
	"flatmatch/api/internal/tma"
)

// Кладовая поисков обязана оставаться той, кого просит middleware.
var _ Touches = (*searchesrepo.Repo)(nil)

type fakeValidator struct{}

type fakeAccounts struct {
	user entity.User
}

type fakeTouches struct {
	calls []uuid.UUID
	err   error
}

func (fakeValidator) Validate(string) (tma.InitData, error) {
	return tma.InitData{User: tma.User{ID: 1, FirstName: "Катя"}}, nil
}

func (f fakeAccounts) EnsureFromTelegram(context.Context, entity.TelegramIdentity) (entity.User, error) {
	return f.user, nil
}

func (f *fakeTouches) Touch(_ context.Context, userID uuid.UUID) error {
	f.calls = append(f.calls, userID)
	return f.err
}

func TestThrottleLetsOneTouchThroughPerWindow(t *testing.T) {
	now := time.Now()
	gate := &throttle{every: touchEvery}
	user, other := uuid.New(), uuid.New()

	if !gate.allow(user, now) {
		t.Fatal("first request did not touch")
	}
	if gate.allow(user, now.Add(touchEvery-time.Second)) {
		t.Error("a request inside the window touched the database")
	}
	// Соседний человек ждать чужого окна не обязан.
	if !gate.allow(other, now) {
		t.Error("another user did not touch")
	}
	if !gate.allow(user, now.Add(touchEvery+time.Second)) {
		t.Error("a request after the window did not touch")
	}
}

func TestMiddlewareTouchesOncePerWindow(t *testing.T) {
	user := entity.User{ID: uuid.New(), Status: entity.StatusActive}
	touches := &fakeTouches{}
	mw := New(fakeValidator{}, fakeAccounts{user: user}, touches)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for range 3 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
		req.Header.Set("Authorization", "tma stub")
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	}

	if len(touches.calls) != 1 || touches.calls[0] != user.ID {
		t.Errorf("Touch calls = %v, want exactly one for %s", touches.calls, user.ID)
	}
}

// Незаписанная активность стоит лишнего вопроса «ещё ищешь?» — не экрана.
func TestFailedTouchDoesNotBreakTheRequest(t *testing.T) {
	touches := &fakeTouches{err: errors.New("database is away")}
	mw := New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, touches)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	req.Header.Set("Authorization", "tma stub")
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareWithoutTouchesDoesNotPanic(t *testing.T) {
	mw := New(fakeValidator{}, fakeAccounts{
		user: entity.User{ID: uuid.New(), Status: entity.StatusActive},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
	req.Header.Set("Authorization", "tma stub")
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
