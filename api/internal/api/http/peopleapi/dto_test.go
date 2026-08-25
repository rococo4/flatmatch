package peopleapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

func TestProfileCarriesNoContacts(t *testing.T) {
	target := entity.User{
		ID:         uuid.New(),
		TelegramID: 777000123,
		Name:       "Катя",
		Username:   "katya",
		Profile: entity.Profile{
			Age:      27,
			Schedule: entity.ScheduleOwl,
			About:    "люблю тишину",
		},
		AboutStatus: entity.ModerationOK,
	}

	raw, err := json.Marshal(toProfile(entity.User{}, target,
		[]string{"/media/photos/a1b2.jpg"}, nil, nil, false, false, nil, entity.LikeBudget{Remaining: 5}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"username", "tg_id", "katya", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("profile JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}

func TestProfileCarriesGenderAndLastSeen(t *testing.T) {
	seen := time.Date(2026, 8, 24, 21, 15, 0, 0, time.UTC)
	target := entity.User{
		ID:       uuid.New(),
		Name:     "Катя",
		Profile:  entity.Profile{Age: 27, Gender: entity.GenderFemale},
		LastSeen: seen,
	}

	raw, err := json.Marshal(toProfile(entity.User{}, target,
		nil, nil, nil, false, false, nil, entity.LikeBudget{Remaining: 5}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, want := range []string{`"gender":"female"`, `"last_seen":"2026-08-24T21:15:00Z"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("profile JSON %s\ndoes not contain %s", raw, want)
		}
	}
}

func TestProfileLikeDirectionsAreNotSwapped(t *testing.T) {
	const myLike, likedMe = true, false

	raw, err := json.Marshal(toProfile(entity.User{}, entity.User{ID: uuid.New()},
		nil, nil, nil, myLike, likedMe, nil, entity.LikeBudget{Remaining: 4}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, want := range []string{`"my_like":true`, `"liked_me":false`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("profile JSON %s\ndoes not contain %s", raw, want)
		}
	}
}
