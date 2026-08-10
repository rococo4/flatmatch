package pools

import (
	"testing"
	"time"

	"github.com/AlekSi/pointer"

	"flatmatch/api/internal/entity"
)

func month(m time.Month) *time.Time {
	t := time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)
	return &t
}

func monthOf(year int, m time.Month) *time.Time {
	t := time.Date(year, m, 1, 0, 0, 0, 0, time.UTC)
	return &t
}

func params(priceMin, priceMax int, moveIn *time.Time, term *entity.Term) entity.SearchParams {
	p := entity.SearchParams{PriceMax: priceMax, MoveIn: moveIn, Term: term}
	if priceMin > 0 {
		p.PriceMin = pointer.To(priceMin)
	}
	return p
}

func withRooms(p entity.SearchParams, min, max *int) entity.SearchParams {
	p.RoomsMin, p.RoomsMax = min, max
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
	year := pointer.To(entity.TermYear)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		want         SectionID
	}{
		// «35–45» и «40–55» совпали — канонический пример §2.1.
		{"overlap is a match", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), SectionAll},
		// NULL = «неважно»: минимальные параметры совпадают со всеми.
		{"both minimal", params(0, 45000, nil, nil), params(0, 40000, nil, nil), SectionAll},
		// Незаданные у него параметры не мешают полному совпадению.
		{"their unspecified ok", params(35000, 45000, sept, year), params(40000, 55000, nil, nil), SectionAll},
		// Касание границ — «краем», но для секций это пересечение.
		{"edge counts as match", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), SectionAll},
		// Цена мимо, заезд совпал → частично.
		{"price off move on", params(0, 40000, sept, nil), params(60000, 80000, sept, nil), SectionPartial},
		// Цена совпала, заезд мимо → частично.
		{"price on move off", params(35000, 45000, sept, nil), params(40000, 55000, dec, nil), SectionPartial},
		// Мягкий параметр (комнаты) ломает «полностью», но не «частично».
		{
			"soft param breaks full",
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(1)},
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(3), RoomsMax: pointer.To(4)},
			SectionPartial,
		},
		// Ни цена, ни заезд → не совпали.
		{"both hard off", params(0, 40000, sept, nil), params(60000, 80000, dec, nil), SectionNone},

		// --- Дополнительное покрытие семантических углов §2.1 ---

		// Одинаковая комнатность — не помеха «полностью» (чип зелёный, см. чипы).
		{
			"identical rooms stay full",
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(1)},
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(1)},
			SectionAll,
		},
		// «Не указана» у него по мягкому параметру — тоже не помеха.
		{
			"their unset rooms stay full",
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(1)},
			entity.SearchParams{PriceMax: 45000},
			SectionAll,
		},
		// Моё «неважно» по комнатам совпадает со всем.
		{
			"my unset rooms stay full",
			entity.SearchParams{PriceMax: 45000},
			entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(3), RoomsMax: pointer.To(4)},
			SectionAll,
		},
		// Срок «краем» (полгода+ vs год+) — пересечение, «полностью» держится.
		{
			"term neighbour stays full",
			params(35000, 45000, sept, pointer.To(entity.TermHalfYear)),
			params(40000, 55000, sept, pointer.To(entity.TermYear)),
			SectionAll,
		},
		// Срок мимо — мягкий параметр, ломает только «полностью».
		{
			"term far breaks full",
			params(35000, 45000, sept, pointer.To(entity.TermHalfYear)),
			params(40000, 55000, sept, pointer.To(entity.TermOpen)),
			SectionPartial,
		},
		// Цена касается краем, заезд мимо → частично (краем = пересеклись).
		{
			"price edge move off",
			params(35000, 45000, sept, nil),
			params(45000, 55000, dec, nil),
			SectionPartial,
		},
		// Заезд соседним месяцем — «краем», для секции пересечение.
		{"move edge counts", params(35000, 45000, sept, nil), params(40000, 55000, month(time.October), nil), SectionAll},
		// Мягкие параметры не спасают: обе жёсткие мимо → «не совпали».
		{
			"soft match cannot rescue hard miss",
			withRooms(params(0, 40000, sept, year), pointer.To(1), pointer.To(2)),
			withRooms(params(60000, 80000, dec, year), pointer.To(1), pointer.To(2)),
			SectionNone,
		},
		// «Не указана» — не «мимо»: незаданный заезд не может дать «не совпали»,
		// в «не совпали» падают только те, у кого ОБА жёстких параметра серые.
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

func TestCompareChips(t *testing.T) {
	sept, oct, dec := month(time.September), month(time.October), month(time.December)
	tests := []struct {
		name         string
		mine, theirs entity.SearchParams
		param        Param
		want         Verdict
	}{
		{"price overlap", params(35000, 45000, nil, nil), params(40000, 55000, nil, nil), ParamPrice, VerdictGreen},
		{"price touch", params(35000, 45000, nil, nil), params(45000, 55000, nil, nil), ParamPrice, VerdictEdge},
		{"price miss", params(0, 40000, nil, nil), params(60000, 80000, nil, nil), ParamPrice, VerdictGrey},
		{"same month", params(0, 40000, sept, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictGreen},
		{"next month", params(0, 40000, sept, nil), params(0, 40000, oct, nil), ParamMoveIn, VerdictEdge},
		{"far month", params(0, 40000, sept, nil), params(0, 40000, dec, nil), ParamMoveIn, VerdictGrey},
		{"their unset month", params(0, 40000, sept, nil), params(0, 40000, nil, nil), ParamMoveIn, VerdictNone},
		// Моё «неважно» совпадает со всем — его чип зелёный.
		{"my unset month", params(0, 40000, nil, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictGreen},
		{"term equal", params(0, 40000, nil, pointer.To(entity.TermYear)), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictGreen},
		{"term neighbour", params(0, 40000, nil, pointer.To(entity.TermHalfYear)), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictEdge},
		{"term far", params(0, 40000, nil, pointer.To(entity.TermHalfYear)), params(0, 40000, nil, pointer.To(entity.TermOpen)), ParamTerm, VerdictGrey},

		// --- Дополнительное покрытие ---

		// Цена участвует всегда: «до» обязательно, nil «от» = 0.
		{"price nested range", params(0, 100000, nil, nil), params(40000, 55000, nil, nil), ParamPrice, VerdictGreen},
		{"price touch reversed", params(45000, 55000, nil, nil), params(35000, 45000, nil, nil), ParamPrice, VerdictEdge},
		{"price both from zero", params(0, 40000, nil, nil), params(0, 45000, nil, nil), ParamPrice, VerdictGreen},

		// Комнатность: обе вилки — целые и включительные.
		{
			"rooms identical",
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(1)),
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(1)),
			ParamRooms, VerdictGreen,
		},
		{
			"rooms share one value",
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(2)),
			withRooms(params(0, 40000, nil, nil), pointer.To(2), pointer.To(3)),
			ParamRooms, VerdictGreen,
		},
		{
			"rooms miss",
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(1)),
			withRooms(params(0, 40000, nil, nil), pointer.To(3), pointer.To(4)),
			ParamRooms, VerdictGrey,
		},
		// Открытая сверху вилка: nil rooms_max = «сколько угодно».
		{
			"rooms open upper bound",
			withRooms(params(0, 40000, nil, nil), pointer.To(3), nil),
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(4)),
			ParamRooms, VerdictGreen,
		},
		// Открытая снизу вилка: nil rooms_min = 1.
		{
			"rooms open lower bound misses",
			withRooms(params(0, 40000, nil, nil), nil, pointer.To(2)),
			withRooms(params(0, 40000, nil, nil), pointer.To(3), pointer.To(4)),
			ParamRooms, VerdictGrey,
		},
		{
			"their unset rooms",
			withRooms(params(0, 40000, nil, nil), pointer.To(1), pointer.To(2)),
			params(0, 40000, nil, nil),
			ParamRooms, VerdictNone,
		},
		{
			"my unset rooms",
			params(0, 40000, nil, nil),
			withRooms(params(0, 40000, nil, nil), pointer.To(3), pointer.To(4)),
			ParamRooms, VerdictGreen,
		},

		// Заезд: соседний месяц через границу года — тоже «краем».
		{"month across year", params(0, 40000, monthOf(2026, time.December), nil), params(0, 40000, monthOf(2027, time.January), nil), ParamMoveIn, VerdictEdge},
		{"previous month", params(0, 40000, oct, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictEdge},
		{"far month backwards", params(0, 40000, dec, nil), params(0, 40000, sept, nil), ParamMoveIn, VerdictGrey},
		{"year apart", params(0, 40000, monthOf(2026, time.September), nil), params(0, 40000, monthOf(2027, time.September), nil), ParamMoveIn, VerdictGrey},

		// Срок: лестница симметрична, «не указан» у него — серый «не указана».
		{"term far reversed", params(0, 40000, nil, pointer.To(entity.TermOpen)), params(0, 40000, nil, pointer.To(entity.TermHalfYear)), ParamTerm, VerdictGrey},
		{"term neighbour reversed", params(0, 40000, nil, pointer.To(entity.TermOpen)), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictEdge},
		{"their unset term", params(0, 40000, nil, pointer.To(entity.TermYear)), params(0, 40000, nil, nil), ParamTerm, VerdictNone},
		{"my unset term", params(0, 40000, nil, nil), params(0, 40000, nil, pointer.To(entity.TermYear)), ParamTerm, VerdictGreen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chips, _ := Compare(tt.mine, tt.theirs)
			if got := chipOf(chips, tt.param); got != tt.want {
				t.Errorf("chip %s = %q, want %q", tt.param, got, tt.want)
			}
		})
	}
}

// Task 9 serialises the chips as-is, so the set and its order are contract.
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

// Инвариант спеки: в секции «Совпали по всем» не бывает серого чипа.
func TestFullSectionNeverHasGreyChips(t *testing.T) {
	sept := month(time.September)
	cases := []entity.SearchParams{
		params(35000, 45000, sept, pointer.To(entity.TermYear)),
		params(0, 40000, nil, nil),
		params(20000, 90000, sept, nil),
		withRooms(params(35000, 45000, month(time.October), pointer.To(entity.TermOpen)), pointer.To(1), pointer.To(2)),
		withRooms(params(45000, 55000, month(time.December), pointer.To(entity.TermHalfYear)), pointer.To(3), nil),
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
