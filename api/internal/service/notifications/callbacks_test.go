package notifications

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

type fakeSearchAnswers struct {
	searches map[uuid.UUID]entity.Search
	cleared  []uuid.UUID
	closed   map[uuid.UUID]entity.CloseReason
	revived  []uuid.UUID
	noSlots  bool
}

func (f *fakeSearchAnswers) ByID(_ context.Context, userID, id uuid.UUID) (entity.Search, error) {
	search, ok := f.searches[id]
	if !ok || search.UserID != userID {
		return entity.Search{}, entity.ErrNotFound
	}
	return search, nil
}

func (f *fakeSearchAnswers) ClearTTLAsked(_ context.Context, id uuid.UUID) error {
	f.cleared = append(f.cleared, id)
	return nil
}

func (f *fakeSearchAnswers) Reactivate(_ context.Context, userID, id uuid.UUID) (entity.Search, error) {
	search, ok := f.searches[id]
	if !ok || search.UserID != userID {
		return entity.Search{}, entity.ErrNotFound
	}
	if f.noSlots {
		return entity.Search{}, entity.ErrSearchLimit
	}
	f.revived = append(f.revived, id)
	search.Status = entity.SearchActive
	f.searches[id] = search
	return search, nil
}

func (f *fakeSearchAnswers) Close(_ context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error) {
	search, ok := f.searches[id]
	if !ok || search.UserID != userID {
		return entity.Search{}, entity.ErrNotFound
	}
	if search.Status == entity.SearchClosed {
		return entity.Search{}, entity.ErrSearchClosed
	}
	if f.closed == nil {
		f.closed = map[uuid.UUID]entity.CloseReason{}
	}
	f.closed[id] = reason
	search.Status = entity.SearchClosed
	f.searches[id] = search
	return search, nil
}

type fakePollAnswers struct {
	members  map[uuid.UUID][]uuid.UUID
	answered map[uuid.UUID]entity.PollAnswer
}

func (f *fakePollAnswers) SavePollAnswer(_ context.Context, matchID, userID uuid.UUID, answer entity.PollAnswer) (bool, error) {
	sides, ok := f.members[matchID]
	if !ok || !slices.Contains(sides, userID) {
		return false, entity.ErrNotFound
	}
	if _, already := f.answered[userID]; already {
		return false, nil
	}
	if f.answered == nil {
		f.answered = map[uuid.UUID]entity.PollAnswer{}
	}
	f.answered[userID] = answer
	return true, nil
}

type fakeAccounts struct {
	byTelegram map[int64]uuid.UUID
	unblocked  []uuid.UUID
}

func (f *fakeAccounts) ByTelegramID(_ context.Context, tgID int64) (entity.User, error) {
	id, ok := f.byTelegram[tgID]
	if !ok {
		return entity.User{}, entity.ErrNotFound
	}
	return entity.User{ID: id, TelegramID: tgID}, nil
}

func (f *fakeAccounts) ClearPushBlocked(_ context.Context, id uuid.UUID) error {
	f.unblocked = append(f.unblocked, id)
	return nil
}

func TestSearchCallbacks(t *testing.T) {
	owner, stranger := uuid.New(), uuid.New()
	const ownerTG, strangerTG = int64(1001), int64(1002)

	active := entity.Search{ID: uuid.New(), UserID: owner, Status: entity.SearchActive}
	paused := entity.Search{ID: uuid.New(), UserID: owner, Status: entity.SearchPaused}
	closed := entity.Search{ID: uuid.New(), UserID: owner, Status: entity.SearchClosed}

	cases := []struct {
		name        string
		tgID        int64
		data        string
		ack         string
		wantCleared bool
		wantClosed  bool
		wantErr     bool
		wantRevived bool
		noSlots     bool
	}{
		{name: "«да, ищу» по своему", tgID: ownerTG, data: searchCallback(opStillSearching, active.ID),
			ack: ackSearching, wantCleared: true},
		{name: "«нашёл» по своему", tgID: ownerTG, data: searchCallback(opFound, active.ID),
			ack: ackClosed, wantClosed: true},
		{name: "«да, ищу» по чужому", tgID: strangerTG, data: searchCallback(opStillSearching, active.ID),
			ack: ackNotYourSearch},
		{name: "«нашёл» по чужому", tgID: strangerTG, data: searchCallback(opFound, active.ID),
			ack: ackNotYourSearch},
		{name: "тап из чата, которого нет в базе", tgID: 999, data: searchCallback(opFound, active.ID),
			ack: ackNotYourSearch},

		{name: "«да, ищу» по погашенному", tgID: ownerTG, data: searchCallback(opStillSearching, paused.ID),
			ack: ackRevived, wantRevived: true},
		{name: "«да, ищу» по погашенному без слотов", tgID: ownerTG, data: searchCallback(opStillSearching, paused.ID),
			ack: ackNoSlots, noSlots: true},
		{name: "повторное «нашёл»", tgID: ownerTG, data: searchCallback(opFound, closed.ID),
			ack: ackAnswered},
		{name: "мусор вместо ответа", tgID: ownerTG, data: "srch:maybe:" + active.ID.String(), wantErr: true},
		{name: "чужой префикс", tgID: ownerTG, data: "mod:u:ok:" + active.ID.String(), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			searches := &fakeSearchAnswers{searches: map[uuid.UUID]entity.Search{
				active.ID: active, paused.ID: paused, closed.ID: closed,
			}, noSlots: tc.noSlots}
			users := &fakeAccounts{byTelegram: map[int64]uuid.UUID{ownerTG: owner, strangerTG: stranger}}
			callbacks := NewCallbacks(searches, &fakePollAnswers{}, users)

			ack, err := callbacks.HandleSearchCallback(t.Context(), tc.tgID, tc.data)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("HandleSearchCallback() = %q, want an error", ack)
				}
				return
			}
			if err != nil {
				t.Fatalf("HandleSearchCallback() failed: %v", err)
			}
			if ack != tc.ack {
				t.Errorf("ack = %q, want %q", ack, tc.ack)
			}
			if got := len(searches.cleared) == 1; got != tc.wantCleared {
				t.Errorf("cleared %v, want cleared = %v", searches.cleared, tc.wantCleared)
			}
			if got := len(searches.revived) == 1; got != tc.wantRevived {
				t.Errorf("revived %v, want revived = %v", searches.revived, tc.wantRevived)
			}

			if got := searches.closed[active.ID] == entity.CloseFoundHere; got != tc.wantClosed {
				t.Errorf("closed %v, want the search closed = %v", searches.closed, tc.wantClosed)
			}
		})
	}
}

func TestPollCallbacks(t *testing.T) {
	user, stranger := uuid.New(), uuid.New()
	const userTG, strangerTG = int64(1001), int64(1002)
	matchID := uuid.New()

	newCallbacks := func() (*Callbacks, *fakePollAnswers) {
		polls := &fakePollAnswers{members: map[uuid.UUID][]uuid.UUID{matchID: {user}}}
		users := &fakeAccounts{byTelegram: map[int64]uuid.UUID{userTG: user, strangerTG: stranger}}
		return NewCallbacks(&fakeSearchAnswers{}, polls, users), polls
	}

	callbacks, polls := newCallbacks()
	ack, err := callbacks.HandlePollCallback(t.Context(), userTG, pollCallback(matchID, entity.PollMoving))
	if err != nil {
		t.Fatalf("HandlePollCallback() failed: %v", err)
	}
	if ack != "✓ Съезжаемся" {
		t.Errorf("ack = %q, want «✓ Съезжаемся»", ack)
	}
	if got := polls.answered[user]; got != entity.PollMoving {
		t.Errorf("saved %q, want moving", got)
	}

	ack, err = callbacks.HandlePollCallback(t.Context(), userTG, pollCallback(matchID, entity.PollFailed))
	if err != nil || ack != ackAnswered {
		t.Errorf("second answer = (%q, %v), want (%q, nil)", ack, err, ackAnswered)
	}
	if got := polls.answered[user]; got != entity.PollMoving {
		t.Errorf("the repeat overwrote the answer with %q", got)
	}

	callbacks, _ = newCallbacks()
	ack, err = callbacks.HandlePollCallback(t.Context(), strangerTG, pollCallback(matchID, entity.PollTalking))
	if err != nil || ack != ackNotYourMatch {
		t.Errorf("stranger's answer = (%q, %v), want (%q, nil)", ack, err, ackNotYourMatch)
	}

	callbacks, _ = newCallbacks()
	if _, err := callbacks.HandlePollCallback(t.Context(), userTG, "poll:"+matchID.String()+":dunno"); err == nil {
		t.Error("an unknown answer was accepted")
	}
}

func TestClearPushBlockedByTelegramID(t *testing.T) {
	user := uuid.New()
	users := &fakeAccounts{byTelegram: map[int64]uuid.UUID{1001: user}}
	callbacks := NewCallbacks(&fakeSearchAnswers{}, &fakePollAnswers{}, users)

	if err := callbacks.ClearPushBlocked(t.Context(), 1001); err != nil {
		t.Fatalf("ClearPushBlocked() failed: %v", err)
	}
	if len(users.unblocked) != 1 || users.unblocked[0] != user {
		t.Errorf("unblocked %v, want [%s]", users.unblocked, user)
	}

	if err := callbacks.ClearPushBlocked(t.Context(), 4242); err != nil {
		t.Errorf("ClearPushBlocked() of an unknown chat = %v, want nil", err)
	}
	if len(users.unblocked) != 1 {
		t.Errorf("unblocked %v after a message from a stranger", users.unblocked)
	}
}
