package matches

import (
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/ru"
	"flatmatch/api/internal/service/zones"
)

const (
	ParamPrice  = "price"
	ParamRooms  = "rooms"
	ParamMoveIn = "move_in"
	ParamTerm   = "term"
)

const monthLayout = "2006-01"

const priceFloor = 0

type Summary struct {
	Zone *zones.Zone

	Point         *PointContext
	ParamsOverlap []ParamOverlap
	SharedAnswers []entity.SharedAnswer
}

type PointContext struct {
	ID    uuid.UUID
	Kind  entity.PointType
	Label string
}

type ParamOverlap struct {
	Param    string
	Min, Max int
	Values   []string
}

func pointLabel(kind entity.PointType, author, station string) string {
	if kind == entity.PointIntentHome {
		room := "на комнату"
		if author != "" {
			room += " " + ru.GenitiveFirstName(author)
		}
		if station == "" {
			return room
		}
		return room + " · м. " + station
	}
	if station == "" {
		return "квартира"
	}
	return "квартира у м. " + station
}

func paramsOverlap(mine, theirs entity.SearchParams) []ParamOverlap {
	var out []ParamOverlap

	if lo, hi, ok := intersect(
		or(mine.PriceMin, priceFloor), mine.PriceMax,
		or(theirs.PriceMin, priceFloor), theirs.PriceMax); ok {
		out = append(out, ParamOverlap{Param: ParamPrice, Min: lo, Max: hi})
	}

	if values := sharedValues(roomKeys(mine.Rooms), roomKeys(theirs.Rooms)); len(values) > 0 {
		out = append(out, ParamOverlap{Param: ParamRooms, Values: values})
	}

	if values := sharedValues(monthKeys(mine.MoveInMonths), monthKeys(theirs.MoveInMonths)); len(values) > 0 {
		out = append(out, ParamOverlap{Param: ParamMoveIn, Values: values})
	}

	if values := sharedValues(termKeys(mine.Terms), termKeys(theirs.Terms)); len(values) > 0 {
		out = append(out, ParamOverlap{Param: ParamTerm, Values: values})
	}

	return out
}

func intersect(aMin, aMax, bMin, bMax int) (lo, hi int, ok bool) {
	lo, hi = max(aMin, bMin), min(aMax, bMax)
	return lo, hi, lo <= hi
}

func sharedValues(mine, theirs []string) []string {
	var out []string
	for _, v := range mine {
		if slices.Contains(theirs, v) {
			out = append(out, v)
		}
	}
	return out
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
		out = append(out, m.Format(monthLayout))
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

func or(v *int, unstated int) int {
	if v == nil {
		return unstated
	}
	return *v
}
