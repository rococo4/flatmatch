package matches

import (
	"math"

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

const (
	priceFloor = 0
	roomsFloor = 1
	roomsWide  = math.MaxInt32
)

type Summary struct {
	Zone *zones.Zone // контекст мэтча, если зонный
	// Point — тот же блок ГДЕ, если пересеклись на точке. Контексты
	// взаимоисключимы: лайк несёт один (§2.3).
	Point         *PointContext
	ParamsOverlap []ParamOverlap
	SharedAnswers []entity.SharedAnswer
}

// PointContext is the mini-card of a point match: the label is ready to print,
// the kind decides the colour of the card's header (§6).
type PointContext struct {
	ID    uuid.UUID
	Kind  entity.PointType
	Label string
}

type ParamOverlap struct {
	Param    string
	Min, Max int    // вилки: Max == 0 у комнат — верхней границы не назвал никто
	Month    string // move_in, «2026-09»
	Term     string
}

// pointLabel: имя автора и станция могут не приехать — автор забанен, зона
// ушла из справочника, — и тогда строка просто короче.
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

	if roomsStated(mine) && roomsStated(theirs) {
		if lo, hi, ok := intersect(
			or(mine.RoomsMin, roomsFloor), or(mine.RoomsMax, roomsWide),
			or(theirs.RoomsMin, roomsFloor), or(theirs.RoomsMax, roomsWide)); ok {
			if hi == roomsWide {
				hi = 0
			}
			out = append(out, ParamOverlap{Param: ParamRooms, Min: lo, Max: hi})
		}
	}

	if mine.MoveIn != nil && theirs.MoveIn != nil {
		if month := mine.MoveIn.Format(monthLayout); month == theirs.MoveIn.Format(monthLayout) {
			out = append(out, ParamOverlap{Param: ParamMoveIn, Month: month})
		}
	}

	if mine.Term != nil && theirs.Term != nil && *mine.Term == *theirs.Term {
		out = append(out, ParamOverlap{Param: ParamTerm, Term: string(*mine.Term)})
	}

	return out
}

func intersect(aMin, aMax, bMin, bMax int) (lo, hi int, ok bool) {
	lo, hi = max(aMin, bMin), min(aMax, bMax)
	return lo, hi, lo <= hi
}

func roomsStated(p entity.SearchParams) bool {
	return p.RoomsMin != nil || p.RoomsMax != nil
}

func or(v *int, unstated int) int {
	if v == nil {
		return unstated
	}
	return *v
}
