package likesapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
)

// TestIncomingCarriesNoContacts pins the privacy rule structurally (§3): an
// incoming like shows a name and a face, never a way to reach the person —
// that appears once the like is answered and a match exists.
func TestIncomingCarriesNoContacts(t *testing.T) {
	like := likesrepo.IncomingLike{
		From: entity.User{
			ID:         uuid.New(),
			TelegramID: 777000123,
			Name:       "Катя",
			Username:   "katya",
			Profile:    entity.Profile{Age: 27, Schedule: entity.ScheduleOwl},
		},
		ZoneID:    "d-khamovniki",
		CreatedAt: time.Now(),
	}

	raw, err := json.Marshal(toIncoming(like, "/media/photos/a1b2.jpg", nil, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"username", "tg_id", "katya", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("incoming JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}

// «На твою комнату» (§2.2): адресат intent_home-контекста и есть автор
// комнаты, поэтому подпись говорит с ним напрямую. Квартира с рынка — общий
// пул, и подпись у неё нейтральная.
func TestPointLabelsSpeakForTheContext(t *testing.T) {
	cases := []struct {
		kind    entity.PointType
		station string
		want    string
	}{
		{entity.PointIntentHome, "Сокол", "на твою комнату · м. Сокол"},
		{entity.PointFlat, "Сокол", "квартира у м. Сокол"},
		// Зона ушла из справочника — лайк это переживает (§2.3), строка короче.
		{entity.PointIntentHome, "", "на твою комнату"},
		{entity.PointFlat, "", "квартира"},
	}

	for _, tc := range cases {
		if got := pointLabel(tc.kind, tc.station); got != tc.want {
			t.Errorf("pointLabel(%s, %q) = %q, want %q", tc.kind, tc.station, got, tc.want)
		}
	}
}
