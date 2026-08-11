package matchesapi

import (
	"encoding/json"
	"strings"
	"testing"

	"flatmatch/api/internal/service/matches"
)

// TestParamOverlapBounds pins how a range reaches the screen: an unbounded top
// («от двух комнат и сколько угодно») has no max at all, so the front-end has
// nothing to print a zero from, while a degenerate range keeps both numbers.
func TestParamOverlapBounds(t *testing.T) {
	tests := []struct {
		name    string
		overlap matches.ParamOverlap
		want    string
	}{
		{
			name:    "rooms without an upper bound",
			overlap: matches.ParamOverlap{Param: matches.ParamRooms, Min: 2},
			want:    `{"param":"rooms","min":2}`,
		},
		{
			name:    "one price for both",
			overlap: matches.ParamOverlap{Param: matches.ParamPrice, Min: 45000, Max: 45000},
			want:    `{"param":"price","min":45000,"max":45000}`,
		},
		{
			name:    "month carries no bounds",
			overlap: matches.ParamOverlap{Param: matches.ParamMoveIn, Month: "2026-09"},
			want:    `{"param":"move_in","month":"2026-09"}`,
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
