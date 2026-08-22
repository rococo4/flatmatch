package poolsapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

func TestParamsDTOSetsAreAlwaysArrays(t *testing.T) {
	raw, err := json.Marshal(toParamsDTO(entity.SearchParams{PriceMax: 45000}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"rooms":[]`, `"terms":[]`, `"move_in":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("params %s does not contain %s", raw, want)
		}
	}

	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	raw, err = json.Marshal(toParamsDTO(entity.SearchParams{
		PriceMax:     45000,
		Rooms:        []int{2, 3},
		Terms:        []entity.Term{entity.TermYear},
		MoveInMonths: []time.Time{sept},
	}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"rooms":[2,3]`, `"terms":["year"]`, `"move_in":["2026-09"]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("params %s does not contain %s", raw, want)
		}
	}
}

func TestPoolResponseListsAreAlwaysArrays(t *testing.T) {
	zone := zones.Zone{ID: "d-khamovniki", Kind: zones.KindDistrict, Name: "Хамовники"}

	tests := []struct {
		name string
		pool pools.Pool
		want []string
	}{
		{
			name: "unranked and empty",
			pool: pools.Pool{Zone: zone, Ranked: false},
			want: []string{`"people":[]`, `"sections":[]`},
		},
		{
			name: "ranked with three empty sections",
			pool: pools.Pool{Zone: zone, Ranked: true, Sections: []pools.Section{
				{ID: pools.SectionAll}, {ID: pools.SectionPartial}, {ID: pools.SectionNone},
			}},
			want: []string{
				`"sections":[{"id":"all","people":[]},{"id":"partial","people":[]},{"id":"none","people":[]}]`,
				`"people":[]`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(toPoolResponse(tt.pool))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(string(raw), want) {
					t.Errorf("response %s\ndoes not contain %s", raw, want)
				}
			}
		})
	}
}
