package searchesapi

import (
	"errors"
	"slices"
	"testing"
	"time"

	"flatmatch/api/internal/entity"
)

func TestToParamsMissingSetsAreAny(t *testing.T) {
	b := paramsRequest{ZoneID: "d-khamovniki", PriceMax: 45000}

	p, err := b.toParams()
	if err != nil {
		t.Fatalf("toParams() = %v, want no error", err)
	}
	if len(p.Rooms) != 0 || len(p.Terms) != 0 || len(p.MoveInMonths) != 0 {
		t.Errorf("sets = %+v, want all empty", p)
	}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestToParamsSortsAndDedupesSets(t *testing.T) {
	b := paramsRequest{
		ZoneID:   "d-khamovniki",
		PriceMax: 45000,
		Rooms:    []int{3, 2, 3},
		Terms:    []string{"year", "year"},
		MoveIn:   []string{"2026-10", "2026-09"},
	}

	p, err := b.toParams()
	if err != nil {
		t.Fatalf("toParams() = %v, want no error", err)
	}
	if !slices.Equal(p.Rooms, []int{2, 3}) {
		t.Errorf("Rooms = %v, want [2 3]", p.Rooms)
	}
	if !slices.Equal(p.Terms, []entity.Term{entity.TermYear}) {
		t.Errorf("Terms = %v, want [year]", p.Terms)
	}
	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if len(p.MoveInMonths) != 2 ||
		!p.MoveInMonths[0].Equal(sept) || !p.MoveInMonths[1].Equal(oct) {
		t.Errorf("MoveInMonths = %v, want [sept oct]", p.MoveInMonths)
	}
	if bad := p.InvalidFields(); len(bad) != 0 {
		t.Errorf("InvalidFields() = %v, want none", bad)
	}
}

func TestToParamsMalformedMoveIn(t *testing.T) {
	for _, raw := range []string{"2026-13", "sept", "2026-09-01", ""} {
		t.Run(raw, func(t *testing.T) {
			b := paramsRequest{ZoneID: "d-khamovniki", PriceMax: 45000, MoveIn: []string{raw}}
			_, err := b.toParams()
			var fields *entity.FieldsError
			if !errors.As(err, &fields) {
				t.Fatalf("toParams() = %v, want a FieldsError", err)
			}
			if len(fields.Fields) != 1 || fields.Fields[0] != "move_in" {
				t.Errorf("Fields = %v, want [move_in]", fields.Fields)
			}
		})
	}
}

func TestToParamsUnknownTermFailsValidation(t *testing.T) {
	b := paramsRequest{ZoneID: "d-khamovniki", PriceMax: 45000, Terms: []string{"decade"}}

	p, err := b.toParams()
	if err != nil {
		t.Fatalf("toParams() = %v, want no error", err)
	}
	if bad := p.InvalidFields(); !slices.Contains(bad, "terms") {
		t.Errorf("InvalidFields() = %v, want to contain terms", bad)
	}
}
