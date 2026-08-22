package pools

import (
	"slices"
	"strconv"
	"time"

	"flatmatch/api/internal/entity"
)

type Verdict string

const (
	VerdictGreen Verdict = "green"
	VerdictEdge  Verdict = "edge"
	VerdictGrey  Verdict = "grey"
	VerdictNone  Verdict = "none"
)

type Param string

const (
	ParamPrice  Param = "price"
	ParamRooms  Param = "rooms"
	ParamMoveIn Param = "move_in"
	ParamTerm   Param = "term"
)

type SectionID string

const (
	SectionAll     SectionID = "all"
	SectionPartial SectionID = "partial"
	SectionNone    SectionID = "none"
)

const monthKeyLayout = "2006-01"

type Chip struct {
	Param   Param
	Verdict Verdict
}

var sectionRank = map[SectionID]int{SectionNone: 0, SectionPartial: 1, SectionAll: 2}

func Compare(mine, theirs entity.SearchParams) ([]Chip, SectionID) {
	chips := []Chip{
		{ParamPrice, priceVerdict(mine, theirs)},
		{ParamRooms, roomsVerdict(mine, theirs)},
		{ParamMoveIn, moveInVerdict(mine, theirs)},
		{ParamTerm, termVerdict(mine, theirs)},
	}

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
	return setVerdict(roomKeys(mine.Rooms), roomKeys(theirs.Rooms))
}

func moveInVerdict(mine, theirs entity.SearchParams) Verdict {
	return setVerdict(monthKeys(mine.MoveInMonths), monthKeys(theirs.MoveInMonths))
}

func termVerdict(mine, theirs entity.SearchParams) Verdict {
	return setVerdict(termKeys(mine.Terms), termKeys(theirs.Terms))
}

func setVerdict(mine, theirs []string) Verdict {
	if len(theirs) == 0 {
		return VerdictNone
	}
	if len(mine) == 0 {
		return VerdictGreen
	}
	for _, v := range mine {
		if slices.Contains(theirs, v) {
			return VerdictGreen
		}
	}
	return VerdictGrey
}

func roomKeys(rooms []int) []string {
	out := make([]string, 0, len(rooms))
	for _, n := range rooms {
		out = append(out, strconv.Itoa(n))
	}
	return out
}

func monthKeys(months []time.Time) []string {
	out := make([]string, 0, len(months))
	for _, m := range months {
		out = append(out, m.Format(monthKeyLayout))
	}
	return out
}

func termKeys(terms []entity.Term) []string {
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		out = append(out, string(t))
	}
	return out
}

func orZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
