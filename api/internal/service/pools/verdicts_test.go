package pools

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

func month(m time.Month) []time.Time {
	return []time.Time{time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)}
}

func monthOf(year int, m time.Month) []time.Time {
	return []time.Time{time.Date(year, m, 1, 0, 0, 0, 0, time.UTC)}
}

func months(in ...time.Time) []time.Time {
	return in
}

func terms(in ...entity.Term) []entity.Term {
	return in
}

func params(priceMin, priceMax int, moveIn []time.Time, term []entity.Term) entity.SearchParams {
	p := entity.SearchParams{PriceMax: priceMax, MoveInMonths: moveIn, Terms: term}
	if priceMin > 0 {
		p.PriceMin = pointer.To(priceMin)
	}
	return p
}

func withRooms(p entity.SearchParams, rooms ...int) entity.SearchParams {
	p.Rooms = rooms
	return p
}

func chipOf(chips []Chip, param Param) Verdict {
	for _, c := range chips {
		if c.Param == param {
			return c.Verdict
		}
	}
	return "missing"
}

func TestCompareSections(t *testing.T) {
	sept, dec := month(time.September), month(time.December)
	year := terms(entity.TermYear)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		want         SectionID
	}{
		{"overlap is a match", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), SectionAll},

		{"both minimal", params(0, 45000, nil, nil), params(0, 40000, nil, nil), SectionAll},

		{"their unspecified ok", params(35000, 45000, sept, year), params(40000, 55000, nil, nil), SectionAll},

		{"edge counts as match", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), SectionAll},

		{"price off move on", params(0, 40000, sept, nil), params(60000, 80000, sept, nil), SectionPartial},

		{"price on move off", params(35000, 45000, sept, nil), params(40000, 55000, dec, nil), SectionPartial},

		{
			"soft param breaks full",
			withRooms(params(0, 45000, nil, nil), 1),
			withRooms(params(0, 45000, nil, nil), 3, 4),
			SectionPartial,
		},

		{"both hard off", params(0, 40000, sept, nil), params(60000, 80000, dec, nil), SectionNone},

		{
			"identical rooms stay full",
			withRooms(params(0, 45000, nil, nil), 1),
			withRooms(params(0, 45000, nil, nil), 1),
			SectionAll,
		},

		{
			"their unset rooms stay full",
			withRooms(params(0, 45000, nil, nil), 1),
			params(0, 45000, nil, nil),
			SectionAll,
		},

		{
			"my unset rooms stay full",
			params(0, 45000, nil, nil),
			withRooms(params(0, 45000, nil, nil), 3, 4),
			SectionAll,
		},

		{
			"shared term stays full",
			params(35000, 45000, sept, terms(entity.TermHalfYear, entity.TermYear)),
			params(40000, 55000, sept, terms(entity.TermYear)),
			SectionAll,
		},

		{
			"disjoint term breaks full",
			params(35000, 45000, sept, terms(entity.TermHalfYear)),
			params(40000, 55000, sept, terms(entity.TermOpen)),
			SectionPartial,
		},

		{
			"price edge move off",
			params(35000, 45000, sept, nil),
			params(45000, 55000, dec, nil),
			SectionPartial,
		},

		{"neighbour month is a miss", params(35000, 45000, sept, nil), params(40000, 55000, month(time.October), nil), SectionPartial},

		{
			"shared month keeps full",
			params(35000, 45000, months(month(time.September)[0], month(time.October)[0]), nil),
			params(40000, 55000, months(month(time.October)[0], month(time.December)[0]), nil),
			SectionAll,
		},

		{
			"soft match cannot rescue hard miss",
			withRooms(params(0, 40000, sept, year), 1, 2),
			withRooms(params(60000, 80000, dec, year), 1, 2),
			SectionNone,
		},

		{"unset move_in is not a miss", params(0, 40000, nil, nil), params(60000, 80000, nil, nil), SectionPartial},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := Compare(tt.mine, tt.theirs); got != tt.want {
				t.Errorf("Compare() section = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetVerdicts(t *testing.T) {
	sept := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	withPrice := func(p entity.SearchParams) entity.SearchParams {
		p.PriceMax = 45000
		return p
	}
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		param        Param
		want         Verdict
	}{
		{"their rooms unset", entity.SearchParams{Rooms: []int{2}}, entity.SearchParams{}, ParamRooms, VerdictNone},
		{"my rooms unset", entity.SearchParams{}, entity.SearchParams{Rooms: []int{2}}, ParamRooms, VerdictGreen},
		{"rooms intersect", entity.SearchParams{Rooms: []int{2, 3}}, entity.SearchParams{Rooms: []int{3, 4}}, ParamRooms, VerdictGreen},
		{"rooms disjoint", entity.SearchParams{Rooms: []int{1}}, entity.SearchParams{Rooms: []int{3, 4}}, ParamRooms, VerdictGrey},
		{"rooms four plus meets four", entity.SearchParams{Rooms: []int{4}}, entity.SearchParams{Rooms: []int{2, 3, 4}}, ParamRooms, VerdictGreen},
		{"months neighbour is grey not edge", entity.SearchParams{MoveInMonths: []time.Time{sept}}, entity.SearchParams{MoveInMonths: []time.Time{oct}}, ParamMoveIn, VerdictGrey},
		{"months intersect", entity.SearchParams{MoveInMonths: []time.Time{sept, oct}}, entity.SearchParams{MoveInMonths: []time.Time{oct, dec}}, ParamMoveIn, VerdictGreen},
		{"months far apart", entity.SearchParams{MoveInMonths: []time.Time{sept}}, entity.SearchParams{MoveInMonths: []time.Time{dec}}, ParamMoveIn, VerdictGrey},
		{"their months unset", entity.SearchParams{MoveInMonths: []time.Time{sept}}, entity.SearchParams{}, ParamMoveIn, VerdictNone},
		{"my months unset", entity.SearchParams{}, entity.SearchParams{MoveInMonths: []time.Time{sept}}, ParamMoveIn, VerdictGreen},
		{"terms intersect", entity.SearchParams{Terms: []entity.Term{entity.TermYear, entity.TermOpen}}, entity.SearchParams{Terms: []entity.Term{entity.TermOpen}}, ParamTerm, VerdictGreen},
		{"terms disjoint is grey not edge", entity.SearchParams{Terms: []entity.Term{entity.TermHalfYear}}, entity.SearchParams{Terms: []entity.Term{entity.TermYear}}, ParamTerm, VerdictGrey},
		{"their terms unset", entity.SearchParams{Terms: []entity.Term{entity.TermYear}}, entity.SearchParams{}, ParamTerm, VerdictNone},
		{"my terms unset", entity.SearchParams{}, entity.SearchParams{Terms: []entity.Term{entity.TermYear}}, ParamTerm, VerdictGreen},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chips, _ := Compare(withPrice(tc.mine), withPrice(tc.theirs))
			if got := chipOf(chips, tc.param); got != tc.want {
				t.Errorf("chip %s = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

func TestComparePriceChips(t *testing.T) {
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		want         Verdict
	}{
		{"price overlap", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), VerdictGreen},
		{"price touch", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), VerdictEdge},
		{"price miss", params(0, 40000, nil, nil), params(60000, 80000, nil, nil), VerdictGrey},
		{"price nested range", params(0, 100000, nil, nil), params(40000, 55000, nil, nil), VerdictGreen},
		{"price touch reversed", params(45000, 55000, nil, nil), params(35000, 45000, nil, nil), VerdictEdge},
		{"price both from zero", params(0, 40000, nil, nil), params(0, 45000, nil, nil), VerdictGreen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chips, _ := Compare(tt.mine, tt.theirs)
			if got := chipOf(chips, ParamPrice); got != tt.want {
				t.Errorf("chip price = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCompareMonthChipsAcrossYears(t *testing.T) {
	chips, _ := Compare(
		params(0, 40000, monthOf(2026, time.December), nil),
		params(0, 40000, monthOf(2026, time.December), nil))
	if got := chipOf(chips, ParamMoveIn); got != VerdictGreen {
		t.Errorf("same month = %q, want green", got)
	}

	chips, _ = Compare(
		params(0, 40000, monthOf(2026, time.September), nil),
		params(0, 40000, monthOf(2027, time.September), nil))
	if got := chipOf(chips, ParamMoveIn); got != VerdictGrey {
		t.Errorf("year apart = %q, want grey", got)
	}
}

func TestCompareAlwaysReturnsFourChipsInOrder(t *testing.T) {
	chips, _ := Compare(params(0, 40000, nil, nil), params(0, 40000, nil, nil))
	want := []Param{ParamPrice, ParamRooms, ParamMoveIn, ParamTerm}
	if len(chips) != len(want) {
		t.Fatalf("Compare() returned %d chips, want %d", len(chips), len(want))
	}
	for i, p := range want {
		if chips[i].Param != p {
			t.Errorf("chip[%d].Param = %q, want %q", i, chips[i].Param, p)
		}
	}
}

func TestFullSectionNeverHasGreyChips(t *testing.T) {
	sept := month(time.September)
	cases := []entity.SearchParams{
		params(35000, 45000, sept, terms(entity.TermYear)),
		params(0, 40000, nil, nil),
		params(20000, 90000, sept, nil),
		withRooms(params(35000, 45000, month(time.October), terms(entity.TermOpen)), 1, 2),
		withRooms(params(45000, 55000, month(time.December), terms(entity.TermHalfYear)), 3, 4),
	}
	for _, mine := range cases {
		for _, theirs := range cases {
			chips, section := Compare(mine, theirs)
			if section != SectionAll {
				continue
			}
			for _, c := range chips {
				if c.Verdict == VerdictGrey {
					t.Errorf("grey chip %s in full section: mine=%+v theirs=%+v", c.Param, mine, theirs)
				}
			}
		}
	}
}

func TestBetterSection(t *testing.T) {
	tests := []struct {
		a, b SectionID
		want bool
	}{
		{SectionAll, SectionPartial, true},
		{SectionAll, SectionNone, true},
		{SectionPartial, SectionNone, true},
		{SectionPartial, SectionAll, false},
		{SectionNone, SectionAll, false},
		{SectionNone, SectionPartial, false},
		{SectionAll, SectionAll, false},
		{SectionPartial, SectionPartial, false},
		{SectionNone, SectionNone, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.a)+" over "+string(tt.b), func(t *testing.T) {
			if got := BetterSection(tt.a, tt.b); got != tt.want {
				t.Errorf("BetterSection(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
