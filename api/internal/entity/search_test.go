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
		ZoneID:   "d-khamovniki",
		PriceMin: pointer.To(35000),
		PriceMax: 45000,
		RoomsMin: pointer.To(2),
		RoomsMax: pointer.To(3),
		MoveIn:   &move,
		Term:     pointer.To(TermYear),
		HasStuff: []StuffChip{StuffFurniture},
	}
}

func TestSearchParamsValidPassesValidation(t *testing.T) {
	if bad := validParams().InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestSearchParamsMinimalIsValid(t *testing.T) {
	// «до» обязательно, всё остальное «неважно» (§2.1).
	p := SearchParams{ZoneID: "line-1", PriceMax: 40000}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

// TestSearchParamsRoomsCapIsInclusive pins the boundary itself: RoomsCap is a
// valid answer, not the first rejected one.
func TestSearchParamsRoomsCapIsInclusive(t *testing.T) {
	p := validParams()
	p.RoomsMin, p.RoomsMax = pointer.To(RoomsCap), pointer.To(RoomsCap)
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
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
		{"rooms inverted", func(p *SearchParams) { *p.RoomsMin = 4 }, "rooms_min"},
		{"rooms zero", func(p *SearchParams) { *p.RoomsMax = 0 }, "rooms_max"},
		// Без верхней границы такое число проходит валидацию и падает уже в
		// int4-кодировщике pgx — 500 вместо 400.
		{"rooms_max above cap", func(p *SearchParams) { *p.RoomsMax = 3_000_000_000 }, "rooms_max"},
		// RoomsMax убран, чтобы сработал именно потолок, а не «min > max».
		{"rooms_min above cap", func(p *SearchParams) {
			p.RoomsMin, p.RoomsMax = pointer.To(RoomsCap+1), nil
		}, "rooms_min"},
		{"unknown term", func(p *SearchParams) { *p.Term = TermUnknown }, "term"},
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
