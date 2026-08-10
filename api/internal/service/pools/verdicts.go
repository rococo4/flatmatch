package pools

import (
	"math"

	"flatmatch/api/internal/entity"
)

type Verdict string

const (
	VerdictGreen Verdict = "green" // пересеклись
	VerdictEdge  Verdict = "edge"  // «краем»: границы касаются / соседние ступени
	VerdictGrey  Verdict = "grey"  // не пересеклись
	VerdictNone  Verdict = "none"  // у него не указан — «не указана», не участвует
)

type Param string

const (
	ParamPrice  Param = "price"
	ParamRooms  Param = "rooms"
	ParamMoveIn Param = "move_in"
	ParamTerm   Param = "term"
)

// SectionID values match the vested frontend (ZonePoolScreen): «Совпали по
// всем параметрам» / «Пересекаются частично» / «Не совпали».
type SectionID string

const (
	SectionAll     SectionID = "all"
	SectionPartial SectionID = "partial"
	SectionNone    SectionID = "none"
)

// roomsWide stands for an open upper bound on rooms: «от 3 и больше».
const roomsWide = math.MaxInt32

type Chip struct {
	Param   Param
	Verdict Verdict
}

var sectionRank = map[SectionID]int{SectionNone: 0, SectionPartial: 1, SectionAll: 2}

var termRank = map[entity.Term]int{entity.TermHalfYear: 0, entity.TermYear: 1, entity.TermOpen: 2}

func Compare(mine, theirs entity.SearchParams) ([]Chip, SectionID) {
	chips := []Chip{
		{ParamPrice, priceVerdict(mine, theirs)},
		{ParamRooms, roomsVerdict(mine, theirs)},
		{ParamMoveIn, moveInVerdict(mine, theirs)},
		{ParamTerm, termVerdict(mine, theirs)},
	}

	// «Пересёкся» = зелёный, «краем» или не участвует. Жёсткие — цена и заезд.
	ok := func(p Param) bool {
		for _, c := range chips {
			if c.Param == p {
				return c.Verdict != VerdictGrey
			}
		}
		return true
	}
	priceOK, moveOK := ok(ParamPrice), ok(ParamMoveIn)

	switch {
	case priceOK && moveOK && ok(ParamRooms) && ok(ParamTerm):
		return chips, SectionAll
	case priceOK || moveOK:
		return chips, SectionPartial
	default:
		return chips, SectionNone
	}
}

func BetterSection(a, b SectionID) bool {
	return sectionRank[a] > sectionRank[b]
}

func priceVerdict(mine, theirs entity.SearchParams) Verdict {
	aMin, aMax := orZero(mine.PriceMin), mine.PriceMax
	bMin, bMax := orZero(theirs.PriceMin), theirs.PriceMax
	switch {
	case aMax > bMin && bMax > aMin:
		return VerdictGreen
	case aMax == bMin || bMax == aMin:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

func roomsVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.RoomsMin == nil && theirs.RoomsMax == nil {
		return VerdictNone
	}
	if mine.RoomsMin == nil && mine.RoomsMax == nil {
		return VerdictGreen // моё «неважно» совпадает со всем (§2.1)
	}
	aMin, aMax := orOne(mine.RoomsMin), orWide(mine.RoomsMax)
	bMin, bMax := orOne(theirs.RoomsMin), orWide(theirs.RoomsMax)
	if aMax >= bMin && bMax >= aMin {
		return VerdictGreen
	}
	return VerdictGrey
}

func moveInVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.MoveIn == nil {
		return VerdictNone
	}
	if mine.MoveIn == nil {
		return VerdictGreen
	}
	a, b := *mine.MoveIn, *theirs.MoveIn
	apart := (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
	switch abs(apart) {
	case 0:
		return VerdictGreen
	case 1:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

// termVerdict: the ladder полгода+ → год+ → бессрочно; neighbours are «краем».
func termVerdict(mine, theirs entity.SearchParams) Verdict {
	if theirs.Term == nil {
		return VerdictNone
	}
	if mine.Term == nil {
		return VerdictGreen
	}
	switch abs(termRank[*mine.Term] - termRank[*theirs.Term]) {
	case 0:
		return VerdictGreen
	case 1:
		return VerdictEdge
	default:
		return VerdictGrey
	}
}

func orZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func orOne(v *int) int {
	if v == nil {
		return 1
	}
	return *v
}

func orWide(v *int) int {
	if v == nil {
		return roomsWide
	}
	return *v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
