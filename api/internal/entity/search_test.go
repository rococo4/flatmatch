package entity

import (
	"slices"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
)

func validParams() SearchParams {
	move := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return SearchParams{
		ZoneID:       "d-khamovniki",
		PriceMin:     pointer.To(35000),
		PriceMax:     45000,
		Rooms:        []int{2, 3},
		Terms:        []Term{TermYear},
		MoveInMonths: []time.Time{move},
		HasStuff:     []StuffChip{StuffFurniture},
	}
}

func TestSearchParamsValidPassesValidation(t *testing.T) {
	if bad := validParams().InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsMinimalIsValid(t *testing.T) {
	p := SearchParams{ZoneID: "line-1", PriceMax: 40000}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsRoomsSetCapIsInclusive(t *testing.T) {
	p := validParams()
	p.Rooms = []int{RoomsSetCap}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsSetsValidation(t *testing.T) {
	sept := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		p    SearchParams
		want []string
	}{
		{"empty sets are fine", SearchParams{ZoneID: "z", PriceMax: 45000}, nil},
		{"full sets are fine", SearchParams{ZoneID: "z", PriceMax: 45000,
			Rooms:        []int{1, 2, 3, 4},
			Terms:        []Term{TermHalfYear, TermYear, TermOpen},
			MoveInMonths: []time.Time{sept, sept.AddDate(0, 1, 0)}}, nil},
		{"rooms above cap", SearchParams{ZoneID: "z", PriceMax: 45000, Rooms: []int{5}}, []string{"rooms"}},
		{"rooms below one", SearchParams{ZoneID: "z", PriceMax: 45000, Rooms: []int{0}}, []string{"rooms"}},
		{"unknown term", SearchParams{ZoneID: "z", PriceMax: 45000, Terms: []Term{TermUnknown}}, []string{"terms"}},
		{"garbage term", SearchParams{ZoneID: "z", PriceMax: 45000, Terms: []Term{"decade"}}, []string{"terms"}},
		{"month not first day", SearchParams{ZoneID: "z", PriceMax: 45000,
			MoveInMonths: []time.Time{sept.AddDate(0, 0, 3)}}, []string{"move_in"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.InvalidFields(); !slices.Equal(got, tc.want) {
				t.Errorf("InvalidFields() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSearchParamsNormalizedSortsAndDedupes(t *testing.T) {
	sept := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	p := SearchParams{
		Rooms:        []int{3, 2, 3},
		Terms:        []Term{TermYear, TermYear},
		MoveInMonths: []time.Time{oct, sept, oct},
	}
	got := p.Normalized()

	if !slices.Equal(got.Rooms, []int{2, 3}) {
		t.Errorf("Rooms = %v, want [2 3]", got.Rooms)
	}
	if !slices.Equal(got.Terms, []Term{TermYear}) {
		t.Errorf("Terms = %v, want [year]", got.Terms)
	}
	if len(got.MoveInMonths) != 2 || !got.MoveInMonths[0].Equal(sept) || !got.MoveInMonths[1].Equal(oct) {
		t.Errorf("MoveInMonths = %v, want [sept oct]", got.MoveInMonths)
	}
	if !slices.Equal(p.Rooms, []int{3, 2, 3}) {
		t.Errorf("Normalized() mutated the source: %v", p.Rooms)
	}
}

func TestSearchParamsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SearchParams)
		want   string
	}{
		{"no zone", func(p *SearchParams) { p.ZoneID = "" }, "zone_id"},
		{"no price_max", func(p *SearchParams) { p.PriceMax = 0 }, "price_max"},
		{"price_max above cap", func(p *SearchParams) { p.PriceMax = 2_000_000 }, "price_max"},
		{"min above max", func(p *SearchParams) { *p.PriceMin = 50000 }, "price_min"},
		{"negative min", func(p *SearchParams) { *p.PriceMin = -1 }, "price_min"},
		{"rooms zero", func(p *SearchParams) { p.Rooms = []int{0, 2} }, "rooms"},
		{"rooms above cap", func(p *SearchParams) { p.Rooms = []int{2, RoomsSetCap + 1} }, "rooms"},
		{"unknown term", func(p *SearchParams) { p.Terms = []Term{TermUnknown} }, "terms"},
		{"empty term", func(p *SearchParams) { p.Terms = []Term{""} }, "terms"},
		{"unknown chip", func(p *SearchParams) { p.HasStuff = []StuffChip{"jacuzzi"} }, "has_stuff"},
		{"unknown chip sentinel", func(p *SearchParams) {
			p.HasStuff = []StuffChip{StuffChipUnknown}
		}, "has_stuff"},
		{"empty chip", func(p *SearchParams) { p.HasStuff = []StuffChip{""} }, "has_stuff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validParams()
			tt.mutate(&p)
			if bad := p.InvalidFields(); !slices.Contains(bad, tt.want) {
				t.Errorf("InvalidFields() = %v, want to contain %q", bad, tt.want)
			}
		})
	}
}

func TestParseSearchEnums(t *testing.T) {
	if got := ParseTerm("year"); got != TermYear {
		t.Errorf("ParseTerm(year) = %q", got)
	}
	if got := ParseTerm("decade"); got != TermUnknown {
		t.Errorf("ParseTerm(decade) = %q, want unknown", got)
	}
	if got := ParseTerm(""); got != "" {
		t.Errorf("ParseTerm(\"\") = %q, want empty", got)
	}
	if got := ParseCloseReason("found_here"); got != CloseFoundHere {
		t.Errorf("ParseCloseReason(found_here) = %q", got)
	}
	if got := ParseSearchStatus("paused"); got != SearchPaused {
		t.Errorf("ParseSearchStatus(paused) = %q", got)
	}
}
