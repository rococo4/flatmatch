package matchesapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/repo/matchesrepo"
	"flatmatch/api/internal/service/matches"
	"flatmatch/api/internal/service/zones"
)

func TestMatchListCarriesNoContacts(t *testing.T) {
	m := matchesrepo.MatchWithPeer{
		Match: entity.Match{ID: uuid.New(), CreatedAt: time.Now()},
		Peer: entity.User{
			ID:         uuid.New(),
			TelegramID: 777000123,
			Name:       "Катя",
			Username:   "katya",
			Profile:    entity.Profile{Age: 27},
		},
	}

	raw, err := json.Marshal(toMatchDTO(m, "/media/photos/a1b2.jpg", nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{"username", "tg_id", "katya", "777000123"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("match list JSON leaks %q:\n%s", forbidden, raw)
		}
	}
}

func TestParamOverlapBounds(t *testing.T) {
	tests := []struct {
		name    string
		overlap matches.ParamOverlap
		want    string
	}{
		{
			name:    "rooms carry values, not bounds",
			overlap: matches.ParamOverlap{Param: matches.ParamRooms, Values: []string{"2", "3"}},
			want:    `{"param":"rooms","values":["2","3"]}`,
		},
		{
			name:    "one price for both",
			overlap: matches.ParamOverlap{Param: matches.ParamPrice, Min: 45000, Max: 45000},
			want:    `{"param":"price","min":45000,"max":45000}`,
		},
		{
			name:    "months carry values",
			overlap: matches.ParamOverlap{Param: matches.ParamMoveIn, Values: []string{"2026-09"}},
			want:    `{"param":"move_in","values":["2026-09"]}`,
		},
		{
			name:    "term carries values",
			overlap: matches.ParamOverlap{Param: matches.ParamTerm, Values: []string{"year"}},
			want:    `{"param":"term","values":["year"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(toParamOverlap(tt.overlap))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(raw), tt.want) {
				t.Errorf("param JSON = %s, want %s", raw, tt.want)
			}
		})
	}
}

func TestMatchDetailTellsWhereFromOneContext(t *testing.T) {
	m := matchesrepo.MatchWithPeer{
		Match: entity.Match{ID: uuid.New(), CreatedAt: time.Now()},
		Peer:  entity.User{ID: uuid.New(), Name: "Катя"},
	}
	point := matches.PointContext{
		ID: uuid.New(), Kind: entity.PointIntentHome, Label: "на комнату Кати · м. Сокол",
	}

	raw, err := json.Marshal(toMatchDetail(m, matches.Summary{Point: &point}, ""))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"where_point"`, point.ID.String(), `"intent_home"`, point.Label} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("match detail %s does not carry %s", raw, want)
		}
	}
	if strings.Contains(string(raw), `"where"`) {
		t.Errorf("point context brought a zone along:\n%s", raw)
	}

	zoned, err := json.Marshal(toMatchDetail(m, matches.Summary{
		Zone: &zones.Zone{ID: "d-khamovniki", Name: "Хамовники"},
	}, ""))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(zoned), `"where"`) || strings.Contains(string(zoned), `"where_point"`) {
		t.Errorf("zone context = %s, want a zone and no point", zoned)
	}
}
