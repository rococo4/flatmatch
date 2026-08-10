package poolsapi

import (
	"encoding/json"
	"strings"
	"testing"

	"flatmatch/api/internal/service/pools"
	"flatmatch/api/internal/service/zones"
)

// TestPoolResponseListsAreAlwaysArrays pins the one thing the front-end may
// assume about an empty pool: both keys are there and both are arrays. An
// absent key would make «пул пуст» look like a field the server forgot.
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
