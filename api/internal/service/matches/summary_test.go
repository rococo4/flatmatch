package matches

import (
	"slices"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
)

var (
	sept = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	oct  = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
)

func active(p entity.SearchParams) entity.Search {
	return entity.Search{SearchParams: p, Status: entity.SearchActive}
}

func closed(p entity.SearchParams) entity.Search {
	return entity.Search{SearchParams: p, Status: entity.SearchClosed, CloseReason: entity.CloseChangedMind}
}

func TestParamsOverlap(t *testing.T) {
	cases := map[string]struct {
		mine, theirs entity.SearchParams
		want         []ParamOverlap
	}{
		"price ranges intersect": {
			mine:   entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000},
			theirs: entity.SearchParams{PriceMin: pointer.To(40000), PriceMax: 55000},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 40000, Max: 45000}},
		},
		"price ranges apart": {
			mine:   entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000},
			theirs: entity.SearchParams{PriceMin: pointer.To(60000), PriceMax: 80000},
			want:   nil,
		},
		"price touching at the border": {
			mine:   entity.SearchParams{PriceMin: pointer.To(35000), PriceMax: 45000},
			theirs: entity.SearchParams{PriceMin: pointer.To(45000), PriceMax: 60000},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 45000, Max: 45000}},
		},
		"price without a lower bound starts at zero": {
			mine:   entity.SearchParams{PriceMax: 45000},
			theirs: entity.SearchParams{PriceMax: 55000},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
		"rooms intersect": {
			mine:   entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(2)},
			theirs: entity.SearchParams{PriceMax: 55000, RoomsMin: pointer.To(2), RoomsMax: pointer.To(3)},
			want: []ParamOverlap{
				{Param: ParamPrice, Min: 0, Max: 45000},
				{Param: ParamRooms, Min: 2, Max: 2},
			},
		},
		// «Неважно» о комнатах нечего рассказывать: в саммари попадает то, о чём
		// договорились обе стороны, а не то, что одна не указала.
		"rooms unstated on one side": {
			mine:   entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(1), RoomsMax: pointer.To(2)},
			theirs: entity.SearchParams{PriceMax: 55000},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
		"rooms with no upper bound on either side": {
			mine:   entity.SearchParams{PriceMax: 45000, RoomsMin: pointer.To(2)},
			theirs: entity.SearchParams{PriceMax: 55000, RoomsMin: pointer.To(3)},
			want: []ParamOverlap{
				{Param: ParamPrice, Min: 0, Max: 45000},
				{Param: ParamRooms, Min: 3, Max: 0},
			},
		},
		"rooms apart": {
			mine:   entity.SearchParams{PriceMax: 45000, RoomsMax: pointer.To(1)},
			theirs: entity.SearchParams{PriceMax: 55000, RoomsMin: pointer.To(3)},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
		"same move-in month": {
			mine:   entity.SearchParams{PriceMax: 45000, MoveIn: &sept},
			theirs: entity.SearchParams{PriceMax: 55000, MoveIn: &sept},
			want: []ParamOverlap{
				{Param: ParamPrice, Min: 0, Max: 45000},
				{Param: ParamMoveIn, Month: "2026-09"},
			},
		},
		"different move-in months": {
			mine:   entity.SearchParams{PriceMax: 45000, MoveIn: &sept},
			theirs: entity.SearchParams{PriceMax: 55000, MoveIn: &oct},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
		"move-in unstated on one side": {
			mine:   entity.SearchParams{PriceMax: 45000, MoveIn: &sept},
			theirs: entity.SearchParams{PriceMax: 55000},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
		"same term": {
			mine:   entity.SearchParams{PriceMax: 45000, Term: pointer.To(entity.TermYear)},
			theirs: entity.SearchParams{PriceMax: 55000, Term: pointer.To(entity.TermYear)},
			want: []ParamOverlap{
				{Param: ParamPrice, Min: 0, Max: 45000},
				{Param: ParamTerm, Term: "year"},
			},
		},
		"different terms": {
			mine:   entity.SearchParams{PriceMax: 45000, Term: pointer.To(entity.TermYear)},
			theirs: entity.SearchParams{PriceMax: 55000, Term: pointer.To(entity.TermOpen)},
			want:   []ParamOverlap{{Param: ParamPrice, Min: 0, Max: 45000}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := paramsOverlap(tc.mine, tc.theirs); !slices.Equal(got, tc.want) {
				t.Errorf("paramsOverlap() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSummaryReadsTheFreshestActiveSearchOfBothSides(t *testing.T) {
	f := newFixture(t)
	f.searches[f.viewer.ID] = []entity.Search{
		active(entity.SearchParams{ZoneID: "line-1", PriceMin: pointer.To(35000), PriceMax: 45000}),
		active(entity.SearchParams{ZoneID: "line-2", PriceMin: pointer.To(90000), PriceMax: 120000}),
	}
	// Закрытый поиск в списке идёт первым нарочно: свежесть считается среди
	// активных, а не по всему архиву.
	f.searches[f.peer.ID] = []entity.Search{
		closed(entity.SearchParams{ZoneID: "line-1", PriceMin: pointer.To(90000), PriceMax: 120000}),
		active(entity.SearchParams{ZoneID: "line-1", PriceMin: pointer.To(40000), PriceMax: 55000}),
	}

	_, summary := f.detail(t)

	want := []ParamOverlap{{Param: ParamPrice, Min: 40000, Max: 45000}}
	if !slices.Equal(summary.ParamsOverlap, want) {
		t.Errorf("ParamsOverlap = %v, want %v", summary.ParamsOverlap, want)
	}
}

// Лайк не сгорает и мэтч не умирает от закрытого поиска (§2.3): саммари просто
// молчит о параметрах, которых больше нет.
func TestSummaryWithoutActiveSearchOnOneSide(t *testing.T) {
	f := newFixture(t)
	f.searches[f.viewer.ID] = []entity.Search{
		active(entity.SearchParams{ZoneID: "line-1", PriceMin: pointer.To(35000), PriceMax: 45000}),
	}
	f.searches[f.peer.ID] = []entity.Search{
		closed(entity.SearchParams{ZoneID: "line-1", PriceMin: pointer.To(40000), PriceMax: 55000}),
	}

	_, summary := f.detail(t)

	if len(summary.ParamsOverlap) != 0 {
		t.Errorf("ParamsOverlap = %v, want none", summary.ParamsOverlap)
	}
	if summary.Zone == nil || len(summary.SharedAnswers) == 0 {
		t.Error("the rest of the summary must still be there")
	}
}

// Контекст-зона историческая: она из мэтча, а не из того, где человек ищет
// сейчас (§6 — «пересеклись в пуле Хамовников»).
func TestSummaryZoneComesFromTheMatch(t *testing.T) {
	f := newFixture(t)
	f.searches[f.viewer.ID] = []entity.Search{active(entity.SearchParams{ZoneID: "line-2", PriceMax: 45000})}
	f.searches[f.peer.ID] = []entity.Search{active(entity.SearchParams{ZoneID: "line-2", PriceMax: 55000})}

	_, summary := f.detail(t)

	if summary.Zone == nil || summary.Zone.ID != "d-khamovniki" {
		t.Errorf("Zone = %v, want the match context d-khamovniki", summary.Zone)
	}
}

func TestSummaryWithoutZoneContext(t *testing.T) {
	f := newFixture(t)
	f.repo.match.Match.ContextZone = ""

	_, summary := f.detail(t)

	if summary.Zone != nil {
		t.Errorf("Zone = %v, want nil for a context-less match", summary.Zone)
	}
}

// Блок ГДЕ у точечного мэтча — мини-карточка точки, и зона в нём молчит:
// контекст у лайка один (§2.3).
func TestSummaryPointContext(t *testing.T) {
	cases := map[string]struct {
		point entity.Point
		want  string
	}{
		"«к себе» несёт имя автора и метро": {
			point: entity.Point{
				Type:      entity.PointIntentHome,
				Stage:     pointer.To(entity.StageLive),
				Status:    entity.PointActive,
				ZoneID:    "st-sokol",
				CreatedBy: pointer.To(uuid.Nil), // подменяется на автора в тесте
			},
			want: "на комнату Катя · м. Сокол",
		},
		"квартира с рынка обезличена": {
			point: entity.Point{Type: entity.PointFlat, Status: entity.PointActive, ZoneID: "st-sokol"},
			want:  "квартира у м. Сокол",
		},
		// Точку закрыли, а мэтч остался: контекст исторический, саммари его помнит.
		"закрытая точка всё равно рассказывает, где пересеклись": {
			point: entity.Point{
				Type:        entity.PointFlat,
				Status:      entity.PointClosed,
				CloseReason: pointer.To("rejected"),
				ZoneID:      "st-sokol",
			},
			want: "квартира у м. Сокол",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if tc.point.CreatedBy != nil {
				tc.point.CreatedBy = pointer.To(f.peer.ID)
			}
			id := f.atPoint(tc.point)

			_, summary := f.detail(t)

			if summary.Zone != nil {
				t.Errorf("Zone = %v, want none for a point match", summary.Zone)
			}
			if summary.Point == nil {
				t.Fatal("Point context is missing")
			}
			if summary.Point.ID != id || summary.Point.Kind != tc.point.Type {
				t.Errorf("Point = %+v, want point %s of kind %s", summary.Point, id, tc.point.Type)
			}
			if summary.Point.Label != tc.want {
				t.Errorf("Label = %q, want %q", summary.Point.Label, tc.want)
			}
		})
	}
}

// Блок ГДЕ один: если в строке мэтча оказались оба контекста, рассказывает
// точка — она конкретнее зоны.
func TestSummaryPointWinsOverZone(t *testing.T) {
	f := newFixture(t)
	f.atPoint(entity.Point{Type: entity.PointFlat, Status: entity.PointActive, ZoneID: "st-sokol"})
	f.repo.match.Match.ContextZone = "d-khamovniki"

	_, summary := f.detail(t)

	if summary.Point == nil || summary.Zone != nil {
		t.Errorf("context = %+v / %+v, want the point alone", summary.Point, summary.Zone)
	}
}

// Точка могла исчезнуть из справочника зон или из базы — мэтч необратим, а
// саммари просто теряет блок ГДЕ.
func TestSummarySurvivesAMissingPoint(t *testing.T) {
	f := newFixture(t)
	gone := uuid.New()
	f.repo.match.Match.ContextZone = ""
	f.repo.match.Match.ContextPoint = &gone

	_, summary := f.detail(t)

	if summary.Point != nil || summary.Zone != nil {
		t.Errorf("context = %+v / %+v, want none", summary.Point, summary.Zone)
	}
	if len(summary.SharedAnswers) == 0 {
		t.Error("the rest of the summary must still be there")
	}
}

func TestSummarySharedAnswersComparesViewerWithPeer(t *testing.T) {
	f := newFixture(t)

	_, summary := f.detail(t)

	want := []entity.SharedAnswer{{Field: "schedule", Value: string(entity.ScheduleOwl)}}
	if !slices.Equal(summary.SharedAnswers, want) {
		t.Errorf("SharedAnswers = %v, want %v", summary.SharedAnswers, want)
	}
}
