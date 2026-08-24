package likesapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/likesrepo"
	"flatmatch/api/internal/service/deck"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

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

	raw, err := json.Marshal(toIncoming(like, []string{"/media/photos/a1b2.jpg"}, nil, nil, pairDTO{}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"username", "tg_id", "katya", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("incoming JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}

func TestPairOfARoomLikeCarriesTheRoomZone(t *testing.T) {
	roomZone := &zoneDTO{ID: "st-sokol", Name: "Сокол"}
	point := &pointDTO{ID: uuid.New(), Kind: string(entity.PointIntentHome), Label: "на твою комнату"}
	pair := &deck.Pair{
		Zone:      zones.Zone{ID: "d-khamovniki", Name: "Хамовники"},
		ZoneMatch: true,
		Section:   pools.SectionAll,
		Chips:     []pools.Chip{{Param: pools.ParamPrice, Verdict: pools.VerdictGreen}},
	}

	got := toPairDTO(pair, point, roomZone)

	if !got.Room {
		t.Error("a like on my room must be marked as a room like")
	}
	if got.Zone == nil || got.Zone.ID != "st-sokol" {
		t.Errorf("zone = %+v, want the zone of the room", got.Zone)
	}
	if got.Section != "all" || len(got.Chips) != 1 || !got.ZoneMatch {
		t.Errorf("pair = %+v, want the full match with its chips", got)
	}
}

func TestPairWithoutSearchesIsEmpty(t *testing.T) {
	got := toPairDTO(nil, nil, nil)

	if got.Room || got.Zone != nil || got.Section != string(pools.SectionNone) || len(got.Chips) != 0 {
		t.Errorf("pair = %+v, want an empty one", got)
	}
}

func TestPointLabelsSpeakForTheContext(t *testing.T) {
	cases := []struct {
		kind    entity.PointType
		station string
		want    string
	}{
		{entity.PointIntentHome, "Сокол", "на твою комнату · м. Сокол"},
		{entity.PointFlat, "Сокол", "квартира у м. Сокол"},

		{entity.PointIntentHome, "", "на твою комнату"},
		{entity.PointFlat, "", "квартира"},
	}

	for _, tc := range cases {
		if got := pointLabel(tc.kind, tc.station); got != tc.want {
			t.Errorf("pointLabel(%s, %q) = %q, want %q", tc.kind, tc.station, got, tc.want)
		}
	}
}
