package peopleapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

// TestProfileCarriesNoContacts pins the privacy rule structurally (§3): the
// match screen is the only place a way to reach a person exists, so no field
// of this DTO may grow into a contact — not even by accident, when somebody
// serialises the whole entity.User the handler already has in hand.
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
		"/media/photos/a1b2.jpg", nil, false, false, entity.LikeBudget{Remaining: 5}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"username", "tg_id", "katya", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("profile JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}
